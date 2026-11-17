package tui

import (
	"encoding/json"
	"fmt"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/transfer"
	"github.com/allisonhere/tideui"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"reflect"
	"strconv"
	"strings"
)

type systemClipboard struct{}

func (systemClipboard) Read() (string, error) { return clipboard.ReadAll() }
func (systemClipboard) Write(s string) error  { return clipboard.WriteAll(s) }

var editorSections = []string{"Identity", "Methods", "Organizations", "Tags & fields", "Relationships", "Notes", "Biography"}

type editField struct {
	path, label, initial string
	group                int
	input                textinput.Model
}
type contactEditor struct {
	original, draft   domain.Contact
	fields            [5][]editField
	section, selected int
	notes, biography  multiline
	err, confirm      string
	quit              bool
}

func newEditor(c domain.Contact) contactEditor {
	ed := contactEditor{original: cloneContact(c), draft: cloneContact(c), notes: newMultiline(c.Notes), biography: newMultiline(c.Biography)}
	ed.rebuild()
	return ed
}
func (e *contactEditor) add(section, group int, path, label string) {
	v, err := fieldValue(&e.draft, path)
	if err != nil {
		return
	}
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 4096
	value := ""
	switch v.Kind() {
	case reflect.Bool:
		value = strconv.FormatBool(v.Bool())
	case reflect.Slice:
		parts := []string{}
		for i := 0; i < v.Len(); i++ {
			parts = append(parts, v.Index(i).String())
		}
		value = strings.Join(parts, ", ")
	default:
		value = v.String()
	}
	input.SetValue(value)
	e.fields[section] = append(e.fields[section], editField{path: path, label: label, initial: input.Value(), group: group, input: input})
}
func (e *contactEditor) rebuild() {
	e.fields = [5][]editField{}
	identity := []string{"DisplayName", "Aliases", "Source", "Avatar", "Favorite", "Archived"}
	if e.draft.Kind == "person" {
		identity = append(identity, "Given", "Middle", "Family", "Prefix", "Suffix", "Nickname", "Phonetic", "Birthday", "Anniversary")
	} else {
		identity = append(identity, "Website", "Industry")
	}
	for _, name := range identity {
		label := map[string]string{"DisplayName": "Display name", "Given": "Given name", "Middle": "Middle name", "Family": "Family name", "Phonetic": "Pronunciation"}[name]
		if label == "" {
			label = name
		}
		e.add(0, -1, name, label)
	}
	for i, m := range e.draft.Methods {
		keys := []string{"Kind", "Label", "Value", "Preferred", "Notes", "Source", "Verification"}
		if m.Kind == "address" {
			keys = append(keys, "Address.POBox", "Address.Extended", "Address.Street", "Address.City", "Address.Region", "Address.PostalCode", "Address.Country")
		}
		for _, name := range keys {
			e.add(1, i, fmt.Sprintf("Methods.%d.%s", i, name), entryLabel(i, name))
		}
	}
	for i := range e.draft.Memberships {
		for _, name := range []string{"Organization", "Title", "Department", "Start", "End", "Current"} {
			e.add(2, i, fmt.Sprintf("Memberships.%d.%s", i, name), entryLabel(i, name))
		}
	}
	e.add(3, -1, "Tags", "Tags (comma-separated)")
	for i := range e.draft.Fields {
		for _, name := range []string{"Name", "Type", "Value", "Options"} {
			e.add(3, i, fmt.Sprintf("Fields.%d.%s", i, name), entryLabel(i, name))
		}
	}
	for i := range e.draft.Relationships {
		for _, name := range []string{"TargetName", "Type", "Notes"} {
			e.add(4, i, fmt.Sprintf("Relationships.%d.%s", i, name), entryLabel(i, name))
		}
	}
	if e.section < 5 {
		e.selected = min(e.selected, max(0, len(e.fields[e.section])-1))
	}
}
func fieldValue(c *domain.Contact, path string) (reflect.Value, error) {
	v := reflect.ValueOf(c).Elem()
	for _, name := range strings.Split(path, ".") {
		if v.Kind() == reflect.Slice {
			i, err := strconv.Atoi(name)
			if err != nil || i < 0 || i >= v.Len() {
				return v, fmt.Errorf("invalid field path")
			}
			v = v.Index(i)
		} else {
			v = v.FieldByName(name)
			if !v.IsValid() {
				return v, fmt.Errorf("invalid field %s", name)
			}
		}
	}
	return v, nil
}
func (e *contactEditor) flush() error {
	for _, fields := range e.fields {
		for _, f := range fields {
			if f.input.Value() == f.initial {
				continue
			}
			v, err := fieldValue(&e.draft, f.path)
			if err != nil {
				return err
			}
			s := f.input.Value()
			switch v.Kind() {
			case reflect.Bool:
				b, err := strconv.ParseBool(s)
				if err != nil {
					return fmt.Errorf("%s must be true or false", f.label)
				}
				v.SetBool(b)
			case reflect.Slice:
				values := []string{}
				if strings.TrimSpace(s) != "" {
					values = transfer.SplitTags(s)
				}
				v.Set(reflect.ValueOf(values))
			default:
				v.SetString(s)
			}
		}
	}
	e.draft.Notes = e.notes.Value()
	e.draft.Biography = e.biography.Value()
	// A changed organization/target name is resolved by the repository, not by an old ID.
	for i := range e.draft.Memberships {
		m := &e.draft.Memberships[i]
		for _, old := range e.original.Memberships {
			if old.ID == m.ID && old.Organization != m.Organization {
				m.OrganizationID = ""
			}
		}
	}
	for i := range e.draft.Relationships {
		r := &e.draft.Relationships[i]
		for _, old := range e.original.Relationships {
			if old.ID == r.ID && old.TargetName != r.TargetName {
				r.TargetID = ""
			}
		}
	}
	return nil
}
func (e *contactEditor) dirty() bool {
	copy := *e
	copy.draft = cloneContact(e.draft)
	if copy.flush() != nil {
		return true
	}
	a, _ := json.Marshal(copy.draft)
	b, _ := json.Marshal(e.original)
	return string(a) != string(b)
}
func (e *contactEditor) focusCmd() tea.Cmd {
	e.notes.Blur()
	e.biography.Blur()
	for s := range e.fields {
		for i := range e.fields[s] {
			e.fields[s][i].input.Blur()
		}
	}
	if e.section == 5 {
		e.notes.Focus()
		return nil
	}
	if e.section == 6 {
		e.biography.Focus()
		return nil
	}
	if len(e.fields[e.section]) > 0 {
		return e.fields[e.section][e.selected].input.Focus()
	}
	return nil
}
func (m Model) updateEditor(msg tea.Msg) (Model, tea.Cmd, bool) {
	e := m.contacts.editor
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = max(0, size.Width), max(0, size.Height)
		e.notes.SetSize(max(1, m.width-6), max(1, m.height-8))
		e.biography.SetSize(max(1, m.width-6), max(1, m.height-8))
		return m, nil, true
	}
	if m.contacts.busy {
		return m, nil, true
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		if e.confirm != "" {
			switch k.String() {
			case "y":
				quit := e.quit
				m.contacts.editor = nil
				if quit {
					return m, tea.Quit, true
				}
				return m, nil, true
			case "n", "esc":
				e.confirm = ""
				e.quit = false
			}
			return m, nil, true
		}
		switch k.String() {
		case "ctrl+c", "esc":
			if e.dirty() {
				e.confirm = "Discard unsaved changes?"
				e.quit = k.String() == "ctrl+c"
				return m, nil, true
			}
			m.contacts.editor = nil
			if k.String() == "ctrl+c" {
				return m, tea.Quit, true
			}
			return m, nil, true
		case "enter":
			if e.section >= 5 {
				break // a newline in notes and biography
			}
			fallthrough
		case "f2", "ctrl+s":
			if err := e.flush(); err != nil {
				e.err = err.Error()
				return m, nil, true
			}
			draft := cloneContact(e.draft)
			draft.Normalize()
			if err := draft.Validate(); err != nil {
				e.err = err.Error()
				return m, nil, true
			}
			var previous *domain.Contact
			if draft.Revision > 0 {
				old := cloneContact(e.original)
				previous = &old
			}
			return m, m.saveContact(draft, previous, "edit"), true
		case "f6", "f5":
			if err := e.flush(); err != nil {
				e.err = err.Error()
				return m, nil, true
			}
			delta := 1
			if k.String() == "f5" {
				delta = -1
			}
			e.section = (e.section + delta + len(editorSections)) % len(editorSections)
			e.selected = 0
			e.rebuild()
			return m, e.focusCmd(), true
		case "tab", "shift+tab", "up", "down":
			if e.section >= 5 {
				if k.String() == "up" || k.String() == "down" {
					break
				}
				if e.section == 5 {
					e.section = 6
				} else {
					e.section = 5
				}
				return m, e.focusCmd(), true
			}
			n := len(e.fields[e.section])
			if n > 0 {
				delta := 1
				if k.String() == "shift+tab" || k.String() == "up" {
					delta = -1
				}
				e.selected = (e.selected + delta + n) % n
			}
			return m, e.focusCmd(), true
		case "ctrl+n", "ctrl+d":
			if e.section >= 5 {
				break
			}
			if err := e.flush(); err != nil {
				e.err = err.Error()
				return m, nil, true
			}
			index := -1
			if len(e.fields[e.section]) > 0 {
				index = e.fields[e.section][e.selected].group
			}
			add := k.String() == "ctrl+n"
			switch e.section {
			case 1:
				if add {
					e.draft.Methods = append(e.draft.Methods, domain.Method{Kind: "email", Verification: "unknown", Source: "manual"})
					index = len(e.draft.Methods) - 1
				} else if index >= 0 {
					e.draft.Methods = append(e.draft.Methods[:index], e.draft.Methods[index+1:]...)
				}
			case 2:
				if e.draft.Kind == "organization" {
					e.err = "Organizations cannot have person memberships; use Relationships"
					return m, nil, true
				}
				if add {
					e.draft.Memberships = append(e.draft.Memberships, domain.Membership{Current: true})
					index = len(e.draft.Memberships) - 1
				} else if index >= 0 {
					e.draft.Memberships = append(e.draft.Memberships[:index], e.draft.Memberships[index+1:]...)
				}
			case 3:
				if add {
					e.draft.Fields = append(e.draft.Fields, domain.CustomField{Type: "text"})
					index = len(e.draft.Fields) - 1
				} else if index >= 0 {
					e.draft.Fields = append(e.draft.Fields[:index], e.draft.Fields[index+1:]...)
				}
			case 4:
				if add {
					e.draft.Relationships = append(e.draft.Relationships, domain.Relationship{Type: "friend"})
					index = len(e.draft.Relationships) - 1
				} else if index >= 0 {
					e.draft.Relationships = append(e.draft.Relationships[:index], e.draft.Relationships[index+1:]...)
				}
			}
			e.rebuild()
			if add {
				for i, f := range e.fields[e.section] {
					if f.group == index {
						e.selected = i
						break
					}
				}
			}
			return m, e.focusCmd(), true
		}
	}
	var cmd tea.Cmd
	if e.section == 5 {
		e.notes.SetSize(max(1, m.width-6), max(1, m.height-8))
		e.notes, cmd = e.notes.Update(msg)
	} else if e.section == 6 {
		e.biography.SetSize(max(1, m.width-6), max(1, m.height-8))
		e.biography, cmd = e.biography.Update(msg)
	} else if len(e.fields[e.section]) > 0 {
		field := &e.fields[e.section][e.selected]
		before := field.input.Value()
		field.input, cmd = field.input.Update(msg)
		if strings.HasSuffix(field.path, ".Kind") && field.input.Value() != before {
			if err := e.flush(); err == nil {
				e.rebuild()
				cmd = e.focusCmd()
			}
		}
	}
	return m, cmd, true
}
func clip(s string, width int) string { return ansi.Truncate(s, max(0, width), "") }
func bounded(s string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = clip(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

// editorPanel renders the contact editor as a modal: a section strip, the section's
// fields as label/field rows, one status line and two quiet hint lines.
func (m Model) editorPanel(u softUI) *tideui.Overlay {
	e := m.contacts.editor
	title := "new " + e.draft.Kind
	if e.original.Revision > 0 {
		title = "edit " + e.draft.Kind + " · " + domain.SafeText(e.original.DisplayName)
	}
	helpText := "Names are optional. Aliases: comma-separated. Dates: YYYY-MM-DD or --MM-DD."
	switch e.section {
	case 1:
		helpText = "Kinds: email, phone, website, social, messaging, address. Preferred: true/false."
	case 2:
		helpText = "Organization: an existing name or UUID, or a new name to create on save."
	case 3:
		helpText = "Types: text, number, date, boolean, url, enum. Enum options: comma-separated."
	case 4:
		helpText = "Target: an existing unique contact name or UUID. Types may be custom."
	case 5, 6:
		helpText = "Arrows move, Shift+arrows select, Ctrl+Z undo, Ctrl+Y redo; paste works."
	}
	labelW := 8
	if e.section < 5 {
		for _, f := range e.fields[e.section] {
			labelW = max(labelW, lipgloss.Width(f.label)+2)
		}
	}
	width, height := m.recordFormSize(84)
	labelW = min(labelW, max(8, width*2/5))
	cw := u.controlWidth(width, labelW)
	body := []string{u.tabs(editorSections, e.section, width), u.blank(width)}
	budget := max(1, height-7)
	if e.section >= 5 {
		budget = max(1, height-8)
	}
	if e.section >= 5 {
		ed := e.notes
		if e.section == 6 {
			ed = e.biography
		}
		for _, line := range strings.Split(ed.Render(u.r, max(1, width-4), max(1, min(14, budget))), "\n") {
			body = append(body, u.line(u.style(u.base, u.text).Render("  ")+line, width))
		}
	} else {
		fields := e.fields[e.section]
		rows := make([]string, len(fields))
		for i, f := range fields {
			rows[i] = u.row(f.label, i == e.selected, u.input(f.input, cw, i == e.selected), width, labelW)
		}
		if len(rows) == 0 {
			rows = []string{u.note("No entries yet — ctrl+n adds one.", width, false)}
		}
		body = append(body, u.rows(rows, e.selected, width, budget)...)
	}
	footer := []string{u.blank(width)}
	switch {
	case m.contacts.busy:
		footer = append(footer, u.note("Saving…", width, false))
	case e.confirm != "":
		footer = append(footer, u.note(e.confirm, width, true))
	case e.err != "":
		footer = append(footer, u.note(domain.SafeText(e.err), width, true))
	default:
		footer = append(footer, u.note(helpText, width, false))
	}
	switch {
	case e.confirm != "":
		footer = append(footer, u.hints(width, "y", "discard", "n", "keep editing"))
	case e.section >= 5:
		footer = append(footer, u.hints(width, "ctrl+s", "save", "tab", "notes/biography"), u.hints(width, "f5", "previous section", "f6", "next section", "esc", "cancel"))
	default:
		footer = append(footer, u.hints(width, "enter", "save", "↑/↓", "field", "ctrl+n", "add entry", "ctrl+d", "remove entry"), u.hints(width, "f5", "previous section", "f6", "next section", "esc", "cancel"))
	}
	return u.fixedFormPanel(title, width, height, body, footer)
}

// entryLabel names a field of a repeated entry: "1 kind", "2 postal code".
func entryLabel(index int, field string) string {
	field = strings.TrimPrefix(field, "Address.")
	words := map[string]string{"POBox": "PO box", "PostalCode": "Postal code", "TargetName": "Target", "Extended": "Extended address"}
	if w, ok := words[field]; ok {
		field = w
	}
	return fmt.Sprintf("%d %s", index+1, strings.ToLower(field[:1])+field[1:])
}
