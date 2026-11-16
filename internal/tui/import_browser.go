package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/transfer"
	"github.com/allisonhere/tideui"
	tea "github.com/charmbracelet/bubbletea"
)

// Import sources own their supported file types; future importers extend this list.
type importSource struct {
	label      string
	extensions []string
	formats    string
	disabled   bool
	directory  bool
}

var importSources = activeImportSources([]importSource{
	{label: "CSV contacts", extensions: []string{".csv"}, formats: "CSV file"},
	{label: "vCard address book", extensions: []string{".vcf", ".vcard"}, formats: "vCard file"},
	{label: "TideContacts CRM archive", extensions: []string{".json"}, formats: "TideContacts JSON archive"},
	{label: "Mutt / NeoMutt aliases", formats: "Alias file or inline aliases in .muttrc"},
	{label: "Google Contacts", formats: "Google CSV, vCard", disabled: true},
	{label: "Microsoft Outlook / 365", formats: "Outlook CSV", disabled: true},
	{label: "Mozilla Thunderbird", formats: "CSV, LDIF, local address-book database", disabled: true},
	{label: "Apple Contacts / iCloud", formats: "vCard", disabled: true},
	{label: "Fastmail", formats: "vCard / CardDAV", disabled: true},
	{label: "Proton Contacts", formats: "vCard", disabled: true},
	{label: "Nextcloud Contacts", formats: "vCard / CardDAV", disabled: true},
	{label: "khard / vdirsyncer", extensions: []string{".vcf", ".vcard"}, formats: "Connect a vCard directory · refresh from the folder", directory: true},
	{label: "Android Contacts", formats: "vCard", disabled: true},
	{label: "Evolution / KDE Kontact", formats: "vCard, CSV, LDIF where available", disabled: true},
})

// Keep enabled sources first as new importers are implemented.
func activeImportSources(sources []importSource) []importSource {
	sort.SliceStable(sources, func(i, j int) bool { return !sources[i].disabled && sources[j].disabled })
	return sources
}

type browserEntry struct {
	name      string
	directory bool
}
type directoryResult struct {
	owner   *transferUI
	path    string
	entries []browserEntry
	err     error
}
type archiveReadResult struct {
	archive domain.Archive
	err     error
}
type archiveImportResult struct {
	report domain.ImportReport
	err    error
}

func (t *transferUI) accepts(path string) bool {
	if importSources[t.source].disabled {
		return false
	}
	if len(importSources[t.source].extensions) == 0 {
		return true
	}
	for _, ext := range importSources[t.source].extensions {
		if strings.EqualFold(filepath.Ext(path), ext) {
			return true
		}
	}
	return false
}
func (t *transferUI) readDirectory(path string) tea.Cmd {
	t.working = true
	t.message = "Reading directory…"
	source, showHidden := t.source, t.showHidden
	return func() tea.Msg {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return directoryResult{owner: t, err: err}
		}
		entries, err := os.ReadDir(absolute)
		if err != nil {
			return directoryResult{owner: t, err: err}
		}
		rows := []browserEntry{}
		if filepath.Dir(absolute) != absolute {
			rows = append(rows, browserEntry{"..", true})
		}
		for _, entry := range entries {
			if !showHidden && strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			directory := entry.IsDir()
			if entry.Type()&os.ModeSymlink != 0 {
				info, e := os.Stat(filepath.Join(absolute, entry.Name()))
				if e != nil {
					continue
				}
				directory = info.IsDir()
			}
			match := directory || len(importSources[source].extensions) == 0
			for _, ext := range importSources[source].extensions {
				match = match || strings.EqualFold(filepath.Ext(entry.Name()), ext)
			}
			if match {
				rows = append(rows, browserEntry{entry.Name(), directory})
			}
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].directory != rows[j].directory {
				return rows[i].directory
			}
			return strings.ToLower(rows[i].name) < strings.ToLower(rows[j].name)
		})
		return directoryResult{owner: t, path: absolute, entries: rows}
	}
}
func (m *Model) readImport(path string) tea.Cmd {
	t := m.contacts.transfer
	if importSources[t.source].disabled {
		t.message = "Coming soon — this importer is not available yet"
		return nil
	}
	if importSources[t.source].directory {
		return m.prepareDirectory(path)
	}
	t.path.SetValue(path)
	if !t.accepts(path) {
		t.message = fmt.Sprintf("Choose a %s file", strings.Join(importSources[t.source].extensions, " or "))
		return nil
	}
	t.working = true
	t.message = "Reading import…"
	if t.source == 2 {
		if m.crm.repo == nil {
			t.working = false
			t.message = "CRM storage is unavailable"
			return nil
		}
		return func() tea.Msg { archive, err := transfer.ReadArchiveFile(path); return archiveReadResult{archive, err} }
	}
	if t.source == 3 {
		return func() tea.Msg { file, err := transfer.ReadMuttFile(path); return fileResult{file, err} }
	}
	return func() tea.Msg { file, err := transfer.ReadFile(path); return fileResult{file, err} }
}

func (m Model) startImportModal() (tea.Model, tea.Cmd) {
	next, cmd, _ := m.startTransfer("import-contacts")
	return next, cmd
}

func (m Model) importSourcesPanel(r tideui.Renderer) *tideui.Overlay {
	u := newSoftUI(r)
	width, height := m.formSize(86)
	selected := m.contacts.transfer.source
	visible := max(1, (height-7)/2)
	start, end := tideui.VisibleRange(len(importSources), selected, visible)
	lines := []string{u.note(fmt.Sprintf("Choose an import source · %d/%d", selected+1, len(importSources)), width, false), u.blank(width)}
	for i := start; i < end; i++ {
		source := importSources[i]
		fg, bg := u.text, u.base
		focused := i == selected
		if source.disabled {
			fg = tideui.MixColors(u.base, u.muted, 0.4)
		} else if focused {
			fg, bg = u.accent, u.field
		}
		status := ""
		if source.disabled {
			status = "coming soon"
		}
		rail := r.SoftRail(focused, u.base)
		labelW := max(1, width-2-len(status)-2)
		label := u.cell(u.style(bg, fg).Bold(focused && !source.disabled).Render(source.label), labelW, bg)
		lines = append(lines, u.line(rail+label+u.style(bg, fg).Render("  "+status), width))
		if source.disabled {
			lines = append(lines, u.line(u.style(u.base, fg).Render("  "+source.formats), width))
		} else {
			lines = append(lines, u.note(source.formats, width, false))
		}
	}
	lines = append(lines, u.blank(width), u.note(m.contacts.transfer.message, width, false), u.hints(width, "↑/↓", "source", "enter", "choose", "esc", "cancel"))
	return u.panel("import / source", width, lines)
}
