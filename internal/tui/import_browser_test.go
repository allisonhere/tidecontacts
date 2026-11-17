package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/transfer"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestImportBrowserFiltersAndNavigates(t *testing.T) {
	m, _ := browser(t)
	root := t.TempDir()
	for _, name := range []string{"contacts.csv", "book.vcf", "archive.json", ".hidden.csv", ".hidden.vcf"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(""), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "folder"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".hidden-folder"), 0700); err != nil {
		t.Fatal(err)
	}
	m, _, _ = m.startTransfer("import-contacts")
	owner := m.contacts.transfer
	m = deliver(m, owner.readDirectory(root))
	if len(owner.entries) != 3 || owner.entries[0].name != ".." || owner.entries[1].name != "folder" || owner.entries[2].name != "contacts.csv" {
		t.Fatalf("wrong CSV entries: %+v", owner.entries)
	}
	m.contacts.transfer.stage = "browse"
	m, cmd := key(m, ".")
	m = deliver(m, cmd)
	if len(owner.entries) != 5 || !owner.showHidden {
		t.Fatalf("hidden CSV entries: %+v", owner.entries)
	}
	for _, entry := range owner.entries {
		if entry.name == ".hidden.vcf" {
			t.Fatal("hidden toggle bypassed format filter")
		}
	}
	m, cmd = key(m, ".")
	m = deliver(m, cmd)
	if len(owner.entries) != 3 || owner.showHidden {
		t.Fatal("hidden entries did not hide again")
	}
	m, _, _ = m.startTransfer("import-contacts")
	m, _, _ = m.updateTransfer(directoryResult{owner: owner, path: "stale"})
	if m.contacts.transfer.directory == "stale" {
		t.Fatal("accepted stale directory result")
	}
	m.contacts.transfer.source = 1
	m = deliver(m, m.contacts.transfer.readDirectory(root))
	if m.contacts.transfer.entries[2].name != "book.vcf" {
		t.Fatal("wrong vCard filter")
	}
	m.contacts.transfer.stage = "browse"
	m.contacts.transfer.browserSelected = 1
	m, cmd = press(m, tea.KeyEnter)
	m = deliver(m, cmd)
	if m.contacts.transfer.directory != filepath.Join(root, "folder") {
		t.Fatal("did not enter directory")
	}
	m, cmd = press(m, tea.KeyBackspace)
	m = deliver(m, cmd)
	if m.contacts.transfer.directory != root {
		t.Fatal("did not navigate to parent")
	}
}

func TestCRMArchiveUsesImportModalAndConfirmsWrite(t *testing.T) {
	m, store := browser(t)
	archive := domain.Archive{Format: domain.ArchiveFormat, Version: domain.ArchiveVersion, Tasks: []domain.Task{{ID: "imported-task", Title: "Imported task"}}}
	path := filepath.Join(t.TempDir(), "crm.json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = transfer.WriteArchive(f, archive)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	m, _, _ = m.startTransfer("import-contacts")
	m.contacts.transfer.source = 2
	m = deliver(m, m.readImport(path))
	if m.contacts.transfer.stage != "archive-preview" || m.crm.confirm != nil {
		t.Fatal("archive left import modal")
	}
	if _, err := store.Task(context.Background(), "imported-task"); err == nil {
		t.Fatal("preview wrote task")
	}
	m, _ = press(m, tea.KeyEnter)
	m, cmd := key(m, "y")
	m = deliver(m, cmd)
	if m.contacts.transfer.stage != "result" {
		t.Fatal("missing results")
	}
	if _, err := store.Task(context.Background(), "imported-task"); err != nil {
		t.Fatal(err)
	}
}

func TestImportModalBoundsAndFocus(t *testing.T) {
	m, _ := browser(t)
	m, _, _ = m.startTransfer("import-contacts")
	m, _ = press(m, tea.KeyCtrlG)
	if m.menuOpen {
		t.Fatal("menu stole modal focus")
	}
	for _, size := range [][2]int{{120, 32}, {80, 24}, {40, 12}, {10, 5}, {1, 1}} {
		m.width, m.height = size[0], size[1]
		view := ansi.Strip(m.View())
		lines := strings.Split(view, "\n")
		if len(lines) > m.height {
			t.Fatalf("height %v: %d", size, len(lines))
		}
		for _, line := range lines {
			if lipgloss.Width(line) > m.width {
				t.Fatalf("width %v: %d", size, lipgloss.Width(line))
			}
		}
	}
}

func TestMuttImportSourceUsesExtensionlessFiles(t *testing.T) {
	m, _ := browser(t)
	root := t.TempDir()
	path := filepath.Join(root, "aliases")
	if err := os.WriteFile(path, []byte("alias alex Alex Example <alex@example.org>\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m, _, _ = m.startTransfer("import-contacts")
	m.contacts.transfer.source = 3
	m = deliver(m, m.contacts.transfer.readDirectory(root))
	if len(m.contacts.transfer.entries) != 2 || m.contacts.transfer.entries[1].name != "aliases" {
		t.Fatalf("%+v", m.contacts.transfer.entries)
	}
	m.contacts.transfer.stage = "browse"
	m.contacts.transfer.browserSelected = 1
	m, cmd := press(m, tea.KeyEnter)
	m = deliver(m, cmd)
	if m.contacts.transfer.stage != "preview" || len(m.contacts.transfer.preview.Selected()) != 1 {
		t.Fatalf("%+v", m.contacts.transfer)
	}
	if len(m.contacts.rows) != 0 {
		t.Fatal("preview wrote contact")
	}
	m, _ = press(m, tea.KeyEnter)
	m, cmd = key(m, "y")
	m = deliver(m, cmd)
	if m.contacts.transfer.stage != "result" || len(m.contacts.rows) != 1 {
		t.Fatal("import failed", m.contacts.err)
	}
}

func TestPlannedImportSourcesAreDisabled(t *testing.T) {
	m, _ := browser(t)
	m, _, _ = m.startTransfer("import-contacts")
	for i := 4; i < len(importSources); i++ {
		if !importSources[i].disabled {
			continue
		}
		m.contacts.transfer.source = i
		m, cmd := press(m, tea.KeyEnter)
		if cmd != nil || m.contacts.transfer.stage != "source" || m.contacts.transfer.working {
			t.Fatalf("enabled planned source %s", importSources[i].label)
		}
		if cmd := m.readImport("anything.csv"); cmd != nil {
			t.Fatal("planned importer read a file")
		}
	}
	m.width, m.height = 80, 20
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Evolution / KDE Kontact") || !strings.Contains(view, "coming soon") {
		t.Fatal("selected planned source not visible")
	}
	if len(strings.Split(view, "\n")) > m.height {
		t.Fatal("source list overflowed")
	}
}

func TestDirectoryConnectionRefreshAndDisconnectInModal(t *testing.T) {
	m, store := browser(t)
	folder := t.TempDir()
	path := filepath.Join(folder, "alex.vcf")
	card := "BEGIN:VCARD\nVERSION:3.0\nUID:alex\nFN:Alex Directory\nEMAIL:alex@example.org\nEND:VCARD\n"
	if err := os.WriteFile(path, []byte(card), 0600); err != nil {
		t.Fatal(err)
	}
	m, _, _ = m.startTransfer("import-contacts")
	m.contacts.transfer.source = 4
	m, cmd := press(m, tea.KeyEnter)
	m = deliver(m, cmd)
	if m.contacts.transfer.stage != "directory-books" {
		t.Fatal("missing directory manager")
	}
	m, cmd = key(m, "n")
	m = deliver(m, cmd)
	m = deliver(m, m.contacts.transfer.readDirectory(folder))
	m, cmd = key(m, "s")
	m = deliver(m, cmd)
	if m.contacts.transfer.stage != "directory-preview" {
		t.Fatal(m.contacts.transfer.stage, m.contacts.transfer.message)
	}
	if len(m.contacts.rows) != 0 {
		t.Fatal("preview wrote contacts")
	}
	m, _ = press(m, tea.KeyEnter)
	m, cmd = key(m, "y")
	m = deliver(m, cmd)
	if m.contacts.transfer.stage != "result" || len(m.contacts.rows) != 1 {
		t.Fatal(m.contacts.transfer.message)
	}
	id := m.contacts.rows[0].ID
	m, _ = press(m, tea.KeyEsc)
	m, cmd = key(m, "e")
	m = deliver(m, cmd)
	if m.contacts.editor != nil || !strings.Contains(m.contacts.err, "read-only") {
		t.Fatal("directory edit allowed")
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(card, "Alex Directory", "Changed Directory")), 0600); err != nil {
		t.Fatal(err)
	}
	model, cmd := m.dispatch("refresh-address-books")
	m = deliver(model.(Model), cmd)
	contact, err := store.Contact(context.Background(), id)
	if err != nil || contact.DisplayName != "Changed Directory" {
		t.Fatal(contact, err)
	}
	m, _, _ = m.startTransfer("import-contacts")
	m.contacts.transfer.source = 4
	m, cmd = press(m, tea.KeyEnter)
	m = deliver(m, cmd)
	m, _ = key(m, "x")
	m, cmd = key(m, "y")
	m = deliver(m, cmd)
	if len(m.contacts.rows) != 0 {
		t.Fatal("disconnected contacts still active")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("source file changed", err)
	}
}
