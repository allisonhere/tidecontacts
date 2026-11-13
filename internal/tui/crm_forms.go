package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
	"github.com/allisonhere/tidecontacts/internal/transfer"
	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// crmPrompt is a one-line overlay: quick capture, dates, names.
type crmPrompt struct {
	title, hint string
	input       textinput.Model
	submit      func(m *Model, value string) (tea.Cmd, error)
	err         string
}

// crmPicker is a filterable list overlay for choosing a record or an option.
type crmPicker struct {
	title, hint string
	input       textinput.Model
	source      string       // entity, project, or empty for the static options
	static      []domain.Ref // options when source is empty
	options     []domain.Ref
	selected    int
	seq         uint64
	choose      func(m *Model, ref domain.Ref) tea.Cmd
}

type crmConfirm struct {
	text string
	yes  func(m *Model) tea.Cmd
}

type pickerResult struct {
	seq     uint64
	options []domain.Ref
}

func newInput(prompt string) textinput.Model {
	in := textinput.New()
	in.Prompt = prompt
	in.CharLimit = 2000
	return in
}

func (m *Model) openPrompt(title, hint, value string, submit func(*Model, string) (tea.Cmd, error)) tea.Cmd {
	in := newInput("> ")
	in.SetValue(value)
	in.CursorEnd()
	m.crm.prompt = &crmPrompt{title: title, hint: hint, input: in, submit: submit}
	return m.crm.prompt.input.Focus()
}

func (m *Model) openPicker(title, hint, source string, static []domain.Ref, choose func(*Model, domain.Ref) tea.Cmd) tea.Cmd {
	p := &crmPicker{title: title, hint: hint, input: newInput("/ "), source: source, static: static, options: static, choose: choose}
	m.crm.picker = p
	return tea.Batch(p.input.Focus(), m.pickerSearch())
}

// pickerSearch refreshes picker options off the UI loop; stale answers are dropped by seq.
func (m *Model) pickerSearch() tea.Cmd {
	p := m.crm.picker
	p.seq++
	seq, text, source, repo, ctx := p.seq, strings.TrimSpace(p.input.Value()), p.source, m.crm.repo, m.contacts.ctx
	if source == "" {
		p.options = nil
		for _, o := range p.static {
			if text == "" || strings.Contains(strings.ToLower(o.Name), strings.ToLower(text)) {
				p.options = append(p.options, o)
			}
		}
		p.selected = min(p.selected, max(0, len(p.options)-1))
		return nil
	}
	return func() tea.Msg {
		q := search.Query{}
		if text != "" {
			q.Terms = strings.Fields(text)
		}
		out := []domain.Ref{}
		if source == "project" {
			list, _ := repo.Projects(ctx, "all", q)
			for _, p := range list {
				out = append(out, domain.Ref{Kind: "project", ID: p.ID, Name: p.Name, Role: p.Status})
			}
		} else {
			for _, kind := range []string{"person", "organization"} {
				rows, _ := repo.List(ctx, kind, q, "name")
				for i, row := range rows {
					if i >= 40 {
						break
					}
					out = append(out, domain.Ref{Kind: kind, ID: row.ID, Name: row.Name, Role: row.Organization})
				}
			}
		}
		return pickerResult{seq, out}
	}
}

// updateOverlay handles keys for the active prompt, picker or confirmation.
func (m Model) updateOverlay(k tea.KeyMsg) (Model, tea.Cmd) {
	c := &m.crm
	key := k.String()
	switch {
	case c.confirm != nil:
		switch key {
		case "y", "enter":
			yes := c.confirm.yes
			c.confirm = nil
			return m, yes(&m)
		case "n", "esc":
			c.confirm = nil
		}
		return m, nil
	case c.prompt != nil:
		p := c.prompt
		switch key {
		case "esc":
			c.prompt = nil
			return m, nil
		case "enter":
			cmd, err := p.submit(&m, strings.TrimSpace(p.input.Value()))
			if err != nil {
				p.err = err.Error()
				return m, nil
			}
			if c.prompt == p {
				c.prompt = nil
			}
			return m, cmd
		}
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(k)
		p.err = ""
		return m, cmd
	case c.picker != nil:
		p := c.picker
		switch key {
		case "esc":
			c.picker = nil
			return m, nil
		case "down", "ctrl+n":
			p.selected = min(p.selected+1, max(0, len(p.options)-1))
			return m, nil
		case "up", "ctrl+p":
			p.selected = max(0, p.selected-1)
			return m, nil
		case "enter":
			if len(p.options) == 0 {
				// Free text is allowed for option pickers such as relationship types.
				if p.source == "" && strings.TrimSpace(p.input.Value()) != "" && p.hint != "" && strings.Contains(p.hint, "type a new") {
					c.picker = nil
					return m, p.choose(&m, domain.Ref{Name: strings.TrimSpace(p.input.Value())})
				}
				return m, nil
			}
			choice := p.options[p.selected]
			c.picker = nil
			return m, p.choose(&m, choice)
		}
		before := p.input.Value()
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(k)
		if p.input.Value() != before {
			p.selected = 0
			return m, tea.Batch(cmd, m.pickerSearch())
		}
		return m, cmd
	}
	return m, nil
}

func (m Model) overlayPanel(r tideui.Renderer) *tideui.Overlay {
	c := m.crm
	width := max(1, min(72, m.width-4))
	inner := max(1, width-4)
	var title string
	lines := []string{}
	switch {
	case c.confirm != nil:
		title = "Confirm"
		lines = append(lines, r.RenderSoftBody(inner, domain.SafeText(c.confirm.text)), "", r.RenderSoftHints(inner, tideui.SoftHint{Key: "y", Label: "confirm"}, tideui.SoftHint{Key: "n/esc", Label: "cancel"}))
	case c.prompt != nil:
		p := c.prompt
		title = p.title
		in := p.input
		in.Width = max(1, inner-3)
		in.TextStyle = r.Styles.DetailBody
		lines = append(lines, in.View(), "")
		if p.err != "" {
			lines = append(lines, r.RenderSoftBody(inner, "! "+domain.SafeText(p.err)))
		} else {
			lines = append(lines, r.RenderSoftBody(inner, p.hint))
		}
	case c.picker != nil:
		p := c.picker
		title = p.title
		in := p.input
		in.Width = max(1, inner-3)
		in.TextStyle = r.Styles.DetailBody
		lines = append(lines, in.View(), "")
		visible := max(1, min(12, m.height-12))
		start, end := tideui.VisibleRange(len(p.options), p.selected, visible)
		for i := start; i < end; i++ {
			o := p.options[i]
			prefix := "  "
			if i == p.selected {
				prefix = "> "
			}
			lines = append(lines, r.RenderSoftRow(tideui.SoftRow{Prefix: prefix, Text: domain.SafeText(o.Name), Suffix: domain.SafeText(o.Role), Selected: i == p.selected}, inner))
		}
		if len(p.options) == 0 {
			lines = append(lines, r.RenderSoftBody(inner, "No matches"))
		}
		if p.hint != "" {
			lines = append(lines, "", r.RenderSoftBody(inner, p.hint))
		}
	default:
		return nil
	}
	o := r.SoftPanelOverlay(tideui.SoftPanel{Prefix: "tidecontacts", Title: title, Width: width, Content: strings.Join(lines, "\n")})
	return &o
}

// formField is one row of a full-screen record form. A field with options is a
// choice cycled with Left/Right; a multiline field edits through the Ripple adapter.
type formField struct {
	key, label, help string
	input            textinput.Model
	options          []string
	multiline        bool
	editor           multiline
	initial          string
}

func (f formField) value() string {
	if f.multiline {
		return f.editor.Value()
	}
	return f.input.Value()
}

type crmForm struct {
	title        string
	fields       []formField
	selected     int
	err, confirm string
	quit         bool
	save         func(m *Model, v map[string]string) (tea.Cmd, error)
}

func field(key, label, value, help string, options ...string) formField {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = 4096
	in.SetValue(value)
	return formField{key: key, label: label, help: help, input: in, options: options, initial: value}
}

func textArea(key, label, value string) formField {
	return formField{key: key, label: label, multiline: true, editor: newMultiline(value), initial: value, help: "Multiline: arrows, Shift+arrows select, Ctrl+Z undo, Ctrl+Y redo, paste. Tab leaves the editor."}
}

func (m *Model) openForm(title string, fields []formField, save func(*Model, map[string]string) (tea.Cmd, error)) tea.Cmd {
	f := &crmForm{title: title, fields: fields, save: save}
	m.crm.form = f
	return f.focus()
}

func (f *crmForm) focus() tea.Cmd {
	var cmd tea.Cmd
	for i := range f.fields {
		fd := &f.fields[i]
		fd.input.Blur()
		fd.editor.Blur()
		if i == f.selected {
			if fd.multiline {
				fd.editor.Focus()
			} else if len(fd.options) == 0 {
				cmd = fd.input.Focus()
			}
		}
	}
	return cmd
}

func (f *crmForm) dirty() bool {
	for _, fd := range f.fields {
		if fd.value() != fd.initial {
			return true
		}
	}
	return false
}

func (f *crmForm) values() map[string]string {
	v := map[string]string{}
	for _, fd := range f.fields {
		v[fd.key] = fd.value()
	}
	return v
}

func (m Model) updateForm(msg tea.Msg) (Model, tea.Cmd) {
	f := m.crm.form
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = max(0, size.Width), max(0, size.Height)
		return m, nil
	}
	if m.crm.busy {
		return m, nil
	}
	fd := &f.fields[f.selected]
	if k, ok := msg.(tea.KeyMsg); ok {
		key := k.String()
		if f.confirm != "" {
			switch key {
			case "y":
				m.crm.form = nil
				if f.quit {
					return m, tea.Quit
				}
			case "n", "esc":
				f.confirm, f.quit = "", false
			}
			return m, nil
		}
		move := func(delta int) (Model, tea.Cmd) {
			f.selected = (f.selected + delta + len(f.fields)) % len(f.fields)
			return m, f.focus()
		}
		switch key {
		case "ctrl+c", "esc":
			if f.dirty() {
				f.confirm, f.quit = "Discard unsaved changes? y discard / n continue", key == "ctrl+c"
				return m, nil
			}
			m.crm.form = nil
			if key == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		case "f2", "ctrl+s":
			cmd, err := f.save(&m, f.values())
			if err != nil {
				f.err = err.Error()
				return m, nil
			}
			f.err = ""
			return m, cmd
		case "tab":
			return move(1)
		case "shift+tab":
			return move(-1)
		case "down", "enter":
			if !fd.multiline {
				return move(1)
			}
		case "up":
			if !fd.multiline {
				return move(-1)
			}
		case "left", "right", " ":
			if n := len(fd.options); n > 0 {
				i := 0
				for j, o := range fd.options {
					if o == fd.input.Value() {
						i = j
					}
				}
				delta := 1
				if key == "left" {
					delta = -1
				}
				fd.input.SetValue(fd.options[(i+delta+n)%n])
				return m, nil
			}
		}
		if len(fd.options) > 0 {
			return m, nil
		}
	}
	var cmd tea.Cmd
	if fd.multiline {
		fd.editor, cmd = fd.editor.Update(msg)
	} else {
		fd.input, cmd = fd.input.Update(msg)
	}
	return m, cmd
}

func (m Model) formView() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	f := m.crm.form
	r := tideui.NewRenderer(resolveTheme(m.cfg.Theme), tideui.StyleOptions{Density: tideui.Compact})
	width := max(1, m.width-4)
	fd := f.fields[f.selected]
	lines := []string{r.Styles.DetailTitle.Render(clip(f.title, width)), clip("Tab/↑↓ fields · ←/→ change a choice · F2 save · Esc cancel", width), clip(fd.help, width), ""}
	if fd.multiline {
		lines = append(lines, r.Styles.DetailMeta.Render(clip("> "+fd.label, width)), fd.editor.Render(r, m.width-6, m.height-8))
	} else {
		visible := max(1, (m.height-7)/2)
		start, end := tideui.VisibleRange(len(f.fields), f.selected, visible)
		for i := start; i < end; i++ {
			x := f.fields[i]
			prefix := "  "
			if i == f.selected {
				prefix = "> "
			}
			lines = append(lines, r.Styles.DetailMeta.Render(clip(prefix+x.label, width)))
			switch {
			case x.multiline:
				first, _, more := strings.Cut(x.editor.Value(), "\n")
				if more {
					first += " …"
				}
				lines = append(lines, r.Styles.DetailBody.Render(clip("  "+domain.SafeText(first), width)))
			case len(x.options) > 0:
				lines = append(lines, r.Styles.DetailBody.Render(clip("  ‹ "+x.input.Value()+" ›", width)))
			default:
				x.input.Width = max(1, width-4)
				x.input.TextStyle = r.Styles.DetailBody
				lines = append(lines, "  "+x.input.View())
			}
		}
	}
	footer := fmt.Sprintf("F2 / Ctrl+S save   Esc cancel   field %d/%d", f.selected+1, len(f.fields))
	if f.err != "" {
		footer = "! " + domain.SafeText(f.err)
	}
	if f.confirm != "" {
		footer = f.confirm
	}
	if m.crm.busy {
		footer = "Saving…"
	}
	body := bounded(strings.Join(lines, "\n"), m.width, max(0, m.height-1))
	if count := len(strings.Split(body, "\n")); count < m.height-1 {
		body += strings.Repeat("\n", m.height-1-count)
	}
	return bounded(body+"\n"+r.Styles.DetailTitle.Render(clip(footer, width)), m.width, m.height)
}

// refsFrom parses a comma-separated list of names ("Name" or "Name (role)"). Names
// that were already linked keep their IDs, so an unchanged field never becomes ambiguous.
func refsFrom(value string, original []domain.Ref) []domain.Ref {
	out := []domain.Ref{}
	if strings.TrimSpace(value) == "" {
		return out
	}
	for _, name := range transfer.SplitTags(value) {
		ref := domain.Ref{Name: strings.TrimSpace(name)}
		if i := strings.LastIndex(ref.Name, " ("); i > 0 && strings.HasSuffix(ref.Name, ")") {
			ref.Name, ref.Role = ref.Name[:i], ref.Name[i+2:len(ref.Name)-1]
		}
		for _, o := range original {
			if strings.EqualFold(o.Name, ref.Name) {
				ref.ID = o.ID
			}
		}
		if ref.Name != "" {
			out = append(out, ref)
		}
	}
	return out
}

func refsText(refs []domain.Ref, kinds ...string) string {
	names := []string{}
	for _, r := range refs {
		ok := len(kinds) == 0
		for _, k := range kinds {
			ok = ok || r.Kind == k
		}
		if !ok {
			continue
		}
		name := r.Name
		if r.Role != "" {
			name += " (" + r.Role + ")"
		}
		if strings.ContainsAny(name, ",\"") {
			name = `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
		}
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

// whenFrom reads "2026-11-13 14:30", "today 09:00", "fri" or an empty value (now).
func whenFrom(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return now, nil
	}
	day, clock := s, ""
	if i := strings.LastIndex(s, " "); i > 0 && domain.ClockValid(s[i+1:]) {
		day, clock = s[:i], s[i+1:]
	} else if domain.ClockValid(s) {
		day, clock = "today", s
	}
	resolved, e := domain.ParseWhen(day, domain.Day(now))
	if e != nil || resolved == "" {
		return now, fmt.Errorf("use a date such as today, fri or 2026-11-13, optionally followed by HH:MM")
	}
	if clock == "" {
		clock = now.Format("15:04")
		if resolved != domain.Day(now) {
			clock = "12:00"
		}
	}
	t, _ := domain.At(resolved, clock, now.Location())
	return t, nil
}

// capture is a parsed quick-capture line.
type capture struct {
	title, typ, status, due, clock, start, on, stage string
	priority                                         int
	recurrence                                       domain.Recurrence
	entities, projects                               []domain.Ref
	tags                                             []string
}

// parseCapture reads "Send proposal with:"Alex Rivera" due:fri pri:high project:Harbor".
// Bare words form the title; key:value pairs use the same vocabulary as search filters.
func parseCapture(line, today string) (capture, error) {
	var c capture
	title, filters, e := search.ParseCapture(line)
	if e != nil {
		return c, e
	}
	c.title = title
	for _, f := range filters {
		switch f.Key {
		case "with", "org":
			c.entities = append(c.entities, domain.Ref{Name: f.Value})
		case "project":
			c.projects = append(c.projects, domain.Ref{Name: f.Value})
		case "tag":
			c.tags = append(c.tags, f.Value)
		case "type":
			c.typ = strings.ToLower(f.Value)
		case "status":
			c.status = strings.ToLower(f.Value)
		case "stage":
			c.stage = f.Value
		case "pri":
			c.priority, e = domain.ParsePriority(f.Value)
		case "due":
			c.due, e = domain.ParseWhen(f.Value, today)
		case "start":
			c.start, e = domain.ParseWhen(f.Value, today)
		case "on":
			c.on, e = domain.ParseWhen(f.Value, today)
		case "at":
			if !domain.ClockValid(f.Value) {
				e = fmt.Errorf("at: needs a 24-hour HH:MM time")
			}
			c.clock = f.Value
		case "every":
			c.recurrence, e = domain.ParseRecurrence(f.Value)
		}
		if e != nil {
			return c, e
		}
	}
	return c, nil
}

func (c capture) task() domain.Task {
	t := domain.Task{Title: c.title, Status: c.status, Priority: c.priority, Due: c.due, DueTime: c.clock, Start: c.start, Recurrence: c.recurrence,
		Entities: c.entities, Projects: c.projects, Tags: c.tags}
	if t.Due == "" && c.on != "" {
		t.Due = c.on
	}
	if t.Status == "" && (t.Due != "" || t.Start != "") {
		t.Status = "scheduled"
	}
	return t
}

func (c capture) activity(now time.Time, defaultType string) domain.Activity {
	a := domain.Activity{Title: c.title, Type: c.typ, Entities: c.entities, Projects: c.projects, Tags: c.tags, OccurredAt: now}
	if a.Type == "" {
		a.Type = defaultType
	}
	day := c.on
	if day == "" {
		day = c.due
	}
	if day != "" || c.clock != "" {
		if day == "" {
			day = domain.Day(now)
		}
		clock := c.clock
		if clock == "" {
			clock = "12:00"
		}
		a.OccurredAt, _ = domain.At(day, clock, now.Location())
	}
	return a
}

// crmDo runs one storage operation off the UI loop and reports back.
func (m *Model) crmDo(note string, fn func(ctx context.Context) (string, error)) tea.Cmd {
	c := &m.crm
	if c.busy {
		return nil
	}
	c.busy, c.err = true, ""
	ctx := m.contacts.ctx
	return func() tea.Msg {
		want, e := fn(ctx)
		return crmDoneMsg{note: note, want: want, err: e}
	}
}
