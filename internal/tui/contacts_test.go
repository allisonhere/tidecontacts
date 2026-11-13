package tui

import (
	"context"
	"fmt"
	"github.com/allisonhere/tidecontacts/internal/config"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func browser(t *testing.T) (Model, *sqlite.Store) {
	t.Helper()
	s, e := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = s.Close() })
	m := sized(110, 32)
	n, cmd := m.Update(Ready{Config: config.Defaults(), Repo: s, Context: context.Background()})
	m = n.(Model)
	m = deliver(m, cmd)
	return m, s
}
func deliver(m Model, cmd tea.Cmd) Model {
	for i := 0; cmd != nil && i < 10; i++ {
		// Storage commands answer in milliseconds; cursor-blink timers do not, and are skipped.
		done := make(chan tea.Msg, 1)
		go func(c tea.Cmd) { done <- c() }(cmd)
		var msg tea.Msg
		select {
		case msg = <-done:
		case <-time.After(350 * time.Millisecond):
			return m
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, cmd := range batch {
				m = deliver(m, cmd)
			}
			return m
		}
		switch msg.(type) {
		case listResult, detailResult, saveResult, searchTick, fileResult, previewResult, exportResult, importProgress,
			crmListMsg, crmDetailMsg, crmDoneMsg, dossierMsg, pickerResult, editMsg, crmSearchTick, dueMsg:
		default:
			return m
		}
		n, next := m.Update(msg)
		m = n.(Model)
		cmd = next
	}
	return m
}
func press(m Model, typ tea.KeyType) (Model, tea.Cmd) {
	n, c := m.Update(tea.KeyMsg{Type: typ})
	return n.(Model), c
}
func TestCreateEditNotesSearchUndo(t *testing.T) {
	m, s := browser(t)
	m, cmd := key(m, "n")
	_ = cmd
	if m.contacts.editor == nil {
		t.Fatal("editor absent")
	}
	m, _ = key(m, "Alex Rivera")
	m, cmd = press(m, tea.KeyF2)
	m = deliver(m, cmd)
	if len(m.contacts.rows) != 1 || m.contacts.contact.DisplayName != "Alex Rivera" {
		t.Fatal(m.contacts.err, m.contacts.rows)
	}
	m, _ = key(m, "e")
	for i := 0; i < 5; i++ {
		m, _ = press(m, tea.KeyF6)
	}
	if m.contacts.editor.section != 5 {
		t.Fatal("notes section")
	}
	m, _ = key(m, "first")
	m, _ = press(m, tea.KeyEnter)
	m, _ = key(m, "second")
	m, cmd = press(m, tea.KeyF2)
	m = deliver(m, cmd)
	if m.contacts.contact.Notes != "first\nsecond" {
		t.Fatal(m.contacts.contact.Notes, m.contacts.err)
	}
	m, cmd = key(m, "f")
	m = deliver(m, cmd)
	if !m.contacts.contact.Favorite {
		t.Fatal("favorite")
	}
	m, cmd = key(m, "u")
	m = deliver(m, cmd)
	if m.contacts.contact.Favorite {
		t.Fatal("undo")
	}
	m, _ = key(m, "/")
	m, cmd = key(m, "tag:")
	m = deliver(m, cmd)
	if !strings.Contains(m.contacts.err, "needs a value") {
		t.Fatal(m.contacts.err)
	}
	m, cmd = press(m, tea.KeyEsc)
	m = deliver(m, cmd)
	if len(m.contacts.rows) != 1 {
		t.Fatal("clear search")
	}
	m, _ = key(m, "D")
	m, cmd = key(m, "n")
	_ = cmd
	if m.contacts.confirm != "" {
		t.Fatal("cancel delete")
	}
	m, _ = key(m, "D")
	m, cmd = key(m, "y")
	m = deliver(m, cmd)
	if len(m.contacts.rows) != 0 {
		t.Fatal("delete")
	}
	m, cmd = key(m, "u")
	m = deliver(m, cmd)
	if len(m.contacts.rows) != 1 {
		t.Fatal("undo deletion")
	}
	got, e := s.Contact(context.Background(), m.contacts.rows[0].ID)
	if e != nil || got.Notes != "first\nsecond" {
		t.Fatal(got, e)
	}
}
func TestEditorCancelDynamicFieldsAndInputOwnership(t *testing.T) {
	m, _ := browser(t)
	m, _ = key(m, "n")
	m, _ = key(m, "q1?:")
	if m.contacts.editor.fields[0][0].input.Value() != "q1?:" || m.overlay != none {
		t.Fatal("editor key leak")
	}
	m, _ = press(m, tea.KeyEsc)
	if m.contacts.editor.confirm == "" {
		t.Fatal("no unsaved warning")
	}
	m, _ = key(m, "n")
	m, _ = press(m, tea.KeyF6)
	m, _ = press(m, tea.KeyCtrlN)
	if len(m.contacts.editor.draft.Methods) != 1 {
		t.Fatal("method not added")
	}
	m, _ = press(m, tea.KeyCtrlD)
	if len(m.contacts.editor.draft.Methods) != 0 {
		t.Fatal("method not removed")
	}
	m, _ = press(m, tea.KeyEsc)
	m, _ = key(m, "y")
	if m.contacts.editor != nil {
		t.Fatal("discard failed")
	}
}
func TestStaleResultsAndBrowserFrames(t *testing.T) {
	m, s := browser(t)
	c := domain.NewContact("person")
	c.DisplayName = "Renée Rivera"
	c.Notes = "Private notes\nSecond line"
	c.Methods = []domain.Method{{Kind: "email", Value: "renee@example.com"}}
	_, e := s.SaveContacts(context.Background(), []domain.Contact{c}, "seed")
	if e != nil {
		t.Fatal(e)
	}
	m = deliver(m, m.refreshContacts())
	n, _, _ := m.updateContacts(detailResult{seq: 0, contact: domain.Contact{DisplayName: "wrong"}})
	if n.contacts.contact.DisplayName == "wrong" {
		t.Fatal("stale detail applied")
	}
	m.contacts.contact.ID = "00000000-0000-0000-0000-000000000000"
	m.contacts.contact.UpdatedAt = "2000-01-01T00:00:00Z"
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(old)
	for _, size := range [][2]int{{160, 45}, {110, 32}, {80, 24}, {60, 20}, {20, 6}, {1, 1}} {
		m.width, m.height = size[0], size[1]
		for view := 0; view < 7; view++ {
			m.contacts.view = view
			out := m.View()
			checkBounds(t, out, size[0], size[1])
		}
		m.contacts.view = 0
		m.focus = 1
		name := fmt.Sprintf("contacts-%dx%d.txt", size[0], size[1])
		if size[0] >= 60 {
			out := ansi.Strip(m.View())

			path := filepath.Join("testdata", name)
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if e = os.WriteFile(path, []byte(out), 0644); e != nil {
					t.Fatal(e)
				}
			}
			want, e := os.ReadFile(path)
			if e != nil || string(want) != out {
				t.Fatalf("frame %s mismatch: %v", name, e)
			}
		}
		ed := newEditor(m.contacts.contact)
		m.contacts.editor = &ed
		for section := 0; section < 7; section++ {
			ed.section = section
			checkBounds(t, m.View(), size[0], size[1])
		}
		m.contacts.editor = nil
	}
}
func checkBounds(t *testing.T, out string, w, h int) {
	t.Helper()
	if len(strings.Split(out, "\n")) > h {
		t.Fatalf("height overflow %dx%d", w, h)
	}
	for _, line := range strings.Split(out, "\n") {
		if lipgloss.Width(line) > w {
			t.Fatalf("width overflow %dx%d", w, h)
		}
	}
}
func TestEditorPreservesMultilineMethodNotes(t *testing.T) {
	c := domain.NewContact("person")
	c.DisplayName = "Name"
	c.Methods = []domain.Method{{Kind: "phone", Value: "123", Notes: "one\ntwo"}}
	ed := newEditor(c)
	if ed.dirty() {
		t.Fatal("unmodified editor dirty")
	}
	if e := ed.flush(); e != nil {
		t.Fatal(e)
	}
	if ed.draft.Methods[0].Notes != "one\ntwo" {
		t.Fatal("multiline method note lost")
	}
}

func TestTransferWorkflowMappingDryRunAndMarkedExport(t *testing.T) {
	m, s := browser(t)
	path := filepath.Join(t.TempDir(), "input.csv")
	if e := os.WriteFile(path, []byte("Name,Email\nImported,imported@example.com\n"), 0600); e != nil {
		t.Fatal(e)
	}
	m, _, _ = m.startTransfer("import-contacts")
	m.contacts.transfer.path.SetValue(path)
	m, cmd := press(m, tea.KeyEnter)
	m = deliver(m, cmd)
	if m.contacts.transfer.stage != "mapping" {
		t.Fatal("missing CSV mapping")
	}
	m, cmd = press(m, tea.KeyEnter)
	m = deliver(m, cmd)
	if m.contacts.transfer.stage != "preview" {
		t.Fatal("missing preview")
	}
	rows, e := s.List(context.Background(), "person", search.Query{}, "name")
	if e != nil || len(rows) != 0 {
		t.Fatal("preview wrote records")
	}
	m, cmd = press(m, tea.KeyEnter)
	_ = cmd
	if m.contacts.transfer.stage != "import-confirm" {
		t.Fatal("missing confirmation")
	}
	m, cmd = key(m, "y")
	m = deliver(m, cmd)
	if m.contacts.transfer.stage != "result" || len(m.contacts.rows) != 1 {
		t.Fatal(m.contacts.err, m.contacts.transfer.stage)
	}
	m, _ = press(m, tea.KeyEsc)
	m, cmd = key(m, " ")
	_ = cmd
	if !m.contacts.marked[m.contacts.rows[0].ID] {
		t.Fatal("not marked")
	}
	destination := filepath.Join(t.TempDir(), "export.csv")
	m, _, _ = m.startTransfer("export-selected")
	m.contacts.transfer.path.SetValue(destination)
	m, _ = press(m, tea.KeyEnter)
	m, cmd = key(m, "y")
	m = deliver(m, cmd)
	data, e := os.ReadFile(destination)
	if e != nil || !strings.Contains(string(data), "Imported") {
		t.Fatal(string(data), e)
	}
}
