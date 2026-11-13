package tui

import (
	"context"
	"fmt"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
	"github.com/allisonhere/tidecontacts/internal/transfer"
	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"path/filepath"
	"sort"
	"strings"
)

type transferUI struct {
	events                         chan tea.Msg
	stage, action, message, format string
	path                           textinput.Model
	file                           transfer.File
	preview                        transfer.Preview
	selected                       int
	working                        bool
}
type importProgress struct{ done, total int }
type progressWriter interface {
	SaveContactsWithProgress(context.Context, []domain.Contact, string, func(int, int)) ([]domain.Contact, error)
}

type fileResult struct {
	file transfer.File
	err  error
}
type previewResult struct {
	preview transfer.Preview
	err     error
}
type exportResult struct {
	count int
	path  string
	err   error
}

func (m Model) startTransfer(action string) (Model, tea.Cmd, bool) {
	t := &transferUI{stage: "path", action: action, format: "vcard"}
	t.path = textinput.New()
	t.path.CharLimit = 4096
	t.path.Prompt = "Path: "
	m.contacts.transfer = t
	return m, t.path.Focus(), true
}
func (m Model) updateTransfer(msg tea.Msg) (Model, tea.Cmd, bool) {
	t := m.contacts.transfer
	switch v := msg.(type) {
	case importProgress:
		t.message = fmt.Sprintf("Writing %d/%d records (transaction not yet committed)...", v.done, v.total)
		return m, func() tea.Msg { return <-t.events }, true
	case tea.WindowSizeMsg:
		m.width, m.height = max(0, v.Width), max(0, v.Height)
		t.path.Width = max(1, m.width-12)
		return m, nil, true
	case fileResult:
		t.working = false
		if v.err != nil {
			t.message = v.err.Error()
			return m, nil, true
		}
		t.file = v.file
		t.message = ""
		if t.file.Format == "csv" {
			t.stage = "mapping"
			t.selected = 0
			return m, nil, true
		}
		return m, m.previewImport(), true
	case previewResult:
		t.working = false
		if v.err != nil {
			t.message = v.err.Error()
			return m, nil, true
		}
		t.preview = v.preview
		t.stage = "preview"
		t.selected = 0
		t.message = "Dry run: no records written. Duplicates are excluded by default."
		return m, nil, true
	case exportResult:
		t.working = false
		if v.err != nil {
			t.message = v.err.Error()
			return m, nil, true
		}
		t.stage = "result"
		t.message = fmt.Sprintf("Exported %d records to %s", v.count, v.path)
		return m, nil, true
	}
	if k, ok := msg.(tea.KeyMsg); ok && t.working && k.String() == "ctrl+c" {
		return m, tea.Quit, true
	}
	if t.working || m.contacts.busy {
		return m, nil, true
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		key := k.String()
		if key == "esc" {
			m.contacts.transfer = nil
			return m, nil, true
		}
		if key == "ctrl+c" {
			m.contacts.transfer = nil
			return m, tea.Quit, true
		}
		switch t.stage {
		case "path":
			if key == "enter" {
				path := t.path.Value()
				if strings.TrimSpace(path) == "" {
					t.message = "Enter a file path"
					return m, nil, true
				}
				if t.action == "import-contacts" {
					t.working = true
					t.message = "Reading import..."
					return m, func() tea.Msg { file, e := transfer.ReadFile(path); return fileResult{file, e} }, true
				}
				ext := strings.ToLower(filepath.Ext(path))
				switch ext {
				case ".csv":
					t.format = "csv"
				case ".vcf", ".vcard":
					t.format = "vcard"
				default:
					t.message = "Choose a .csv or .vcf destination"
					return m, nil, true
				}
				t.stage = "export-confirm"
				t.message = "Export includes private notes and extension fields. Destination must not exist."
				return m, nil, true
			}
			var cmd tea.Cmd
			t.path, cmd = t.path.Update(msg)
			return m, cmd, true
		case "mapping":
			switch key {
			case "up", "k":
				t.selected = max(0, t.selected-1)
			case "down", "j":
				t.selected = min(len(t.file.Mapping)-1, t.selected+1)
			case "h":
				t.file.HasHeader = !t.file.HasHeader
			case "left", "right", " ":
				i := 0
				for j, v := range transfer.Columns {
					if v == t.file.Mapping[t.selected] {
						i = j
					}
				}
				delta := 1
				if key == "left" {
					delta = -1
				}
				t.file.Mapping[t.selected] = transfer.Columns[(i+delta+len(transfer.Columns))%len(transfer.Columns)]
			case "enter":
				return m, m.previewImport(), true
			}
			return m, nil, true
		case "preview":
			switch key {
			case "up", "k":
				t.selected = max(0, t.selected-1)
			case "down", "j":
				t.selected = min(max(0, len(t.preview.Records)-1), t.selected+1)
			case " ":
				if len(t.preview.Records) > 0 {
					r := &t.preview.Records[t.selected]
					if r.Error == "" {
						r.Include = !r.Include
					}
				}
			case "b":
				if t.file.Format == "csv" {
					t.stage = "mapping"
					t.selected = 0
				}
			case "enter":
				if len(t.preview.Selected()) == 0 {
					t.message = "Select at least one valid record with Space"
				} else {
					t.stage = "import-confirm"
					t.message = fmt.Sprintf("Import %d selected records? y confirm / n return. No existing record will be overwritten.", len(t.preview.Selected()))
				}
			}
			return m, nil, true
		case "import-confirm":
			switch key {
			case "n":
				t.stage = "preview"
			case "y":
				contacts := t.preview.Selected()
				repo := m.contacts.repo
				ctx, cancel := context.WithCancel(m.contacts.ctx)
				m.contacts.cancelWrite = cancel
				m.contacts.busy = true
				t.message = fmt.Sprintf("Importing %d records in one transaction...", len(contacts))
				events := make(chan tea.Msg, 16)
				t.events = events
				return m, func() tea.Msg {
					go func() {
						var saved []domain.Contact
						var err error
						if writer, ok := repo.(progressWriter); ok {
							saved, err = writer.SaveContactsWithProgress(ctx, contacts, "import", func(done, total int) {
								select {
								case events <- importProgress{done, total}:
								default:
								}
							})
						} else {
							saved, err = repo.SaveContacts(ctx, contacts, "import")
						}
						result := saveResult{contacts: saved, err: err, action: "import"}
						for {
							select {
							case events <- result:
								return
							default:
								select {
								case <-events:
								default:
								}
							}
						}
					}()
					return <-events
				}, true
			}
			return m, nil, true
		case "export-confirm":
			if key == "y" {
				return m, m.exportContacts(), true
			}
			if key == "n" {
				t.stage = "path"
			}
			return m, nil, true
		case "result":
			if key == "enter" {
				m.contacts.transfer = nil
			}
			return m, nil, true
		}
	}
	return m, nil, true
}
func (m *Model) previewImport() tea.Cmd {
	t := m.contacts.transfer
	t.working = true
	t.message = "Validating records and checking duplicates..."
	file := t.file
	repo, ctx := m.contacts.repo, m.contacts.ctx
	return func() tea.Msg { preview, e := transfer.BuildPreview(ctx, repo, file); return previewResult{preview, e} }
}
func (m *Model) exportContacts() tea.Cmd {
	t := m.contacts.transfer
	t.working = true
	t.message = "Collecting records and writing export..."
	repo, ctx := m.contacts.repo, m.contacts.ctx
	path, format, action := t.path.Value(), t.format, t.action
	ids := []string{}
	switch action {
	case "export-selected":
		for id, marked := range m.contacts.marked {
			if marked {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		if len(ids) == 0 && m.contacts.contact.ID != "" {
			ids = append(ids, m.contacts.contact.ID)
		}
	case "export-filtered":
		for _, row := range m.contacts.rows {
			ids = append(ids, row.ID)
		}
	}
	return func() tea.Msg {
		if action == "export-all" {
			for _, kind := range []string{"person", "organization"} {
				for _, filter := range []string{"", "is:archived"} {
					q, _ := search.Parse(filter)
					rows, e := repo.List(ctx, kind, q, "name")
					if e != nil {
						return exportResult{err: e}
					}
					for _, row := range rows {
						ids = append(ids, row.ID)
					}
				}
			}
		}
		contacts := []domain.Contact{}
		for _, id := range ids {
			c, e := repo.Contact(ctx, id)
			if e != nil {
				return exportResult{err: e}
			}
			contacts = append(contacts, c)
		}
		if len(contacts) == 0 {
			return exportResult{err: fmt.Errorf("no contacts selected for export")}
		}
		if e := ctx.Err(); e != nil {
			return exportResult{err: e}
		}
		e := transfer.ExportFile(path, format, contacts)
		return exportResult{len(contacts), path, e}
	}
}
func (m Model) transferView() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	t := m.contacts.transfer
	r := tideui.NewRenderer(resolveTheme(m.cfg.Theme), tideui.StyleOptions{Density: tideui.Compact})
	lines := []string{r.Styles.DetailTitle.Render("TideContacts / " + t.action + " / " + t.stage), ""}
	w := max(1, m.width-4)
	switch t.stage {
	case "path":
		path := t.path
		path.Width = max(1, w-8)
		lines = append(lines, "Enter a local CSV or vCard file path.", path.View(), "", "Enter continue · Esc cancel")
	case "mapping":
		lines = append(lines, fmt.Sprintf("Header row: %t (h toggles)", t.file.HasHeader), "Up/Down column · Left/Right mapping · Enter preview", "")
		visible := max(1, m.height-9)
		start := max(0, t.selected-visible+1)
		for i := start; i < min(len(t.file.Headers), start+visible); i++ {
			prefix := "  "
			if i == t.selected {
				prefix = "> "
			}
			sample := ""
			row := 0
			if t.file.HasHeader {
				row = 1
			}
			if row < len(t.file.Rows) && i < len(t.file.Rows[row]) {
				sample = t.file.Rows[row][i]
			}
			lines = append(lines, clip(fmt.Sprintf("%s%d %s → %s  [%s]", prefix, i+1, domain.SafeText(t.file.Headers[i]), t.file.Mapping[i], domain.SafeText(sample)), w))
		}
	case "preview":
		lines = append(lines, fmt.Sprintf("%d records · %d selected", len(t.preview.Records), len(t.preview.Selected())), "Space include/exclude · b mapping · Enter confirm · Esc cancel", "")
		visible := max(1, (m.height-11)/2)
		start := max(0, t.selected-visible+1)
		for i := start; i < min(len(t.preview.Records), start+visible); i++ {
			row := t.preview.Records[i]
			prefix := "  "
			if i == t.selected {
				prefix = "> "
			}
			mark := "[ ]"
			if row.Include {
				mark = "[x]"
			}
			lines = append(lines, clip(fmt.Sprintf("%s%s row %d %s", prefix, mark, row.Row, domain.SafeText(row.Contact.DisplayName)), w))
			detail := row.Error
			if detail == "" {
				detail = row.Duplicate
			}
			if detail == "" {
				detail = fmt.Sprintf("%d methods · %d memberships · %s", len(row.Contact.Methods), len(row.Contact.Memberships), row.Contact.Kind)
			}
			lines = append(lines, clip("    "+domain.SafeText(detail), w))
		}
	case "export-confirm":
		lines = append(lines, "Destination: "+t.path.Value(), "Format: "+t.format, "Scope: "+t.action, "", "y export · n back · Esc cancel")
	case "import-confirm":
		lines = append(lines, "y import · n return to preview · Esc cancel")
	case "result":
		lines = append(lines, "Enter / Esc returns to contacts")
	}
	message := t.message
	if m.contacts.err != "" {
		message = "! " + m.contacts.err
	}
	lines = append(lines, "", domain.SafeText(message))
	return bounded(strings.Join(lines, "\n"), m.width, m.height)
}
