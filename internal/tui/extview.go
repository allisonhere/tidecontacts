package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/ext"
	"github.com/allisonhere/tideui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Extension workspaces: a full-screen view described by an extension and drawn
// by the host. The extension supplies data (tabs, tiles, a board or list,
// sections, notices, actions); the host owns layout, focus, theme, keys and
// every pixel of styling. Nothing here executes extension code in Update/View:
// all calls are tea.Cmds.

const extNarrowFrom = 100

type extFrame struct {
	workspace, title string // set on a back-stack frame that crosses workspaces
	tab, item, query string
	state            map[string]string
	col, row         int
	selID            string
	panel            string         // focused panel
	psel             map[string]int // remembered selection per panel; replaced, never mutated
}

type extView struct {
	ext, ws string
	title   string
	stack   []extFrame
	cur     extFrame
	doc     ext.View
	busy    bool
	err     string
	notice  string
	seq     uint64
	help    bool
	cancel  context.CancelFunc // of the render in flight
}

type (
	extViewMsg struct {
		owner *extView
		seq   uint64
		doc   ext.View
		err   error
	}
	extActionMsg struct {
		owner *extView
		res   ext.ActionResult
		err   error
	}
	extSubmitMsg struct {
		owner *extView
		ext   string
		res   ext.ActionResult
		err   error
	}
)

func (m Model) extWorkspaceEntries() []app.Command {
	if m.extensions.mgr == nil {
		return nil
	}
	var out []app.Command
	for _, w := range m.extensions.mgr.Workspaces() {
		out = append(out, app.Command{ID: "ext-ws:" + w.Extension + ":" + w.ID, Label: ext.Clean(w.Title, 24, false)})
	}
	return out
}

// openExtWorkspace shows an extension's workspace, resuming where the user left it.
func (m Model) openExtWorkspace(extID, ws string) (Model, tea.Cmd) {
	m.closeExtView()
	title := ws
	for _, w := range m.extensions.mgr.Workspaces() {
		if w.Extension == extID && w.ID == ws {
			title = w.Title
		}
	}
	v := &extView{ext: extID, ws: ws, title: title, busy: true}
	if last, ok := m.extensions.resume[extID+"/"+ws]; ok {
		v.stack, v.cur = append([]extFrame(nil), last.stack...), last.cur
	}
	m.extensions.view = v
	m.menuOpen = false
	return m, m.extRender(v)
}

// extRender asks the extension for the current view, off the UI loop.
func (m *Model) extRender(v *extView) tea.Cmd {
	v.seq++
	v.busy = true
	seq, mgr, ctx := v.seq, m.extensions.mgr, m.contacts.ctx
	req := ext.ViewRequest{Workspace: v.ws, Tab: v.cur.tab, Item: v.cur.item, Query: v.cur.query, State: v.cur.state, Width: m.width, Narrow: m.width < extNarrowFrom}
	id := v.ext
	// A newer request replaces an older one: stop the extension working on it.
	if v.cancel != nil {
		v.cancel()
	}
	c, cancel := extTimeout(ctx)
	v.cancel = cancel
	return func() tea.Msg {
		defer cancel()
		doc, err := mgr.RenderView(c, id, req)
		return extViewMsg{owner: v, seq: seq, doc: doc, err: err}
	}
}

// selection returns the selectable items of the current document.
type extItem struct {
	id     string
	target *ext.Target
}

func (v *extView) items() []extItem {
	var out []extItem
	if len(v.doc.Panels) > 0 {
		return panelItems(v.doc.Panels[v.focusedPanel()])
	}
	switch {
	case v.doc.Board != nil && len(v.doc.Board.Columns) > 0:
		col := v.doc.Board.Columns[min(v.cur.col, len(v.doc.Board.Columns)-1)]
		for _, c := range col.Cards {
			out = append(out, extItem{id: c.ID})
		}
	case v.doc.List != nil:
		for _, r := range v.doc.List.Rows {
			out = append(out, extItem{id: r.ID})
		}
	}
	if len(out) == 0 || (v.doc.Board == nil && v.doc.List == nil) {
		for _, s := range v.doc.Sections {
			for _, l := range s.Lines {
				if l.Target != nil {
					out = append(out, extItem{id: firstNonEmptyStr(l.ID, l.Target.ID), target: l.Target})
				}
			}
		}
	}
	return out
}

func (v *extView) selected() (extItem, bool) {
	items := v.items()
	if len(items) == 0 {
		return extItem{}, false
	}
	return items[min(max(0, v.cur.row), len(items)-1)], true
}

// settle clamps the cursor after a new document arrives and keeps the same
// item selected when it still exists (so moving a card does not lose your place).
func (v *extView) settle() {
	if len(v.doc.Panels) > 0 {
		v.settlePanels()
		return
	}
	if v.doc.Board != nil && len(v.doc.Board.Columns) > 0 {
		v.cur.col = min(max(0, v.cur.col), len(v.doc.Board.Columns)-1)
		if v.cur.selID == "" && len(v.doc.Board.Columns[v.cur.col].Cards) == 0 {
			// Open on the first column that has something to select.
			for i, c := range v.doc.Board.Columns {
				if len(c.Cards) > 0 {
					v.cur.col, v.cur.row = i, 0
					break
				}
			}
		}
		if v.cur.selID != "" {
			for ci, c := range v.doc.Board.Columns {
				for ri, cd := range c.Cards {
					if cd.ID == v.cur.selID {
						v.cur.col, v.cur.row = ci, ri
						return
					}
				}
			}
		}
	} else if v.cur.selID != "" {
		for i, it := range v.items() {
			if it.id == v.cur.selID {
				v.cur.row = i
				return
			}
		}
	}
	v.cur.row = min(max(0, v.cur.row), max(0, len(v.items())-1))
}

func (m *Model) closeExtView() {
	v := m.extensions.view
	if v == nil {
		return
	}
	if v.cancel != nil {
		v.cancel()
	}
	if m.extensions.resume == nil {
		m.extensions.resume = map[string]*extView{}
	}
	m.extensions.resume[v.ext+"/"+v.ws] = &extView{ext: v.ext, ws: v.ws, title: v.title, stack: v.stack, cur: v.cur}
	m.extensions.view = nil
}

// extViewHandles reports whether the extension view should receive keys now.
func (m Model) extViewActive() bool {
	c := m.crm
	return m.extensions.view != nil && !m.settings.open && m.overlay == none && m.extensions.modal == nil && c.form == nil && c.prompt == nil && c.picker == nil && c.confirm == nil && m.contacts.editor == nil && m.contacts.transfer == nil
}

func (m Model) updateExtView(msg tea.Msg) (Model, tea.Cmd, bool) {
	v := m.extensions.view
	switch r := msg.(type) {
	case extViewMsg:
		if v == nil || r.owner != v || r.seq != v.seq {
			return m, nil, true
		}
		v.busy = false
		if r.err != nil {
			v.err = extSafe(r.err.Error(), 200)
			return m, nil, true
		}
		v.err, v.doc = "", r.doc
		if r.doc.Tab != "" {
			v.cur.tab = r.doc.Tab
		}
		v.settle()
		return m, nil, true
	case extActionMsg:
		if v == nil || r.owner != v {
			return m, nil, true
		}
		v.busy = false
		if r.err != nil {
			v.err = extSafe(r.err.Error(), 200)
			return m, nil, true
		}
		next, cmd := m.applyExtResult(v, r.res)
		return next, cmd, true
	case extSubmitMsg:
		m.crm.busy = false
		if r.err != nil {
			if m.crm.form != nil {
				m.crm.form.err = extSafe(r.err.Error(), 200)
			}
			return m, nil, true
		}
		if len(r.res.Errors) > 0 {
			if m.crm.form != nil {
				m.crm.form.err = formErrorText(m.crm.form, r.res.Errors)
			}
			return m, nil, true
		}
		m.crm.form = nil
		if v != nil && r.owner == v {
			next, cmd := m.applyExtResult(v, r.res)
			return next, cmd, true
		}
		next, cmd := m.applyExtCommandResult(r.ext, r.res)
		return next, cmd, true
	case tea.KeyMsg:
		if !m.extViewActive() {
			return m, nil, false
		}
		next, cmd := m.extViewKey(r)
		return next, cmd, true
	}
	return m, nil, false
}

func formErrorText(f *crmForm, errs map[string]string) string {
	for i, fd := range f.fields {
		if e, ok := errs[fd.key]; ok {
			f.selected = i
			return fd.label + ": " + e
		}
	}
	for _, e := range errs {
		return e
	}
	return ""
}

func (m Model) extViewKey(k tea.KeyMsg) (Model, tea.Cmd) {
	v := m.extensions.view
	key := k.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if key == "ctrl+z" {
		return m, tea.Suspend
	}
	back := func() (Model, tea.Cmd) {
		if n := len(v.stack); n > 0 {
			v.cur, v.stack = v.stack[n-1], v.stack[:n-1]
			if v.cur.workspace != "" {
				v.ws, v.title = v.cur.workspace, v.cur.title
				v.cur.workspace, v.cur.title = "", ""
			}
			v.err, v.notice = "", ""
			return m, m.extRender(v)
		}
		m.closeExtView()
		return m, nil
	}
	if v.help { // any key closes the help
		v.help = false
		return m, nil
	}
	switch key {
	case "esc", "q":
		return back()
	case "?":
		v.help = true
		return m, nil
	}
	if m.registry.Key(key) == "palette" {
		next, cmd := m.dispatch("palette")
		return next.(Model), cmd
	}
	if v.busy && v.doc.Tabs == nil {
		return m, nil
	}
	items := v.items()
	move := func(d int) {
		if len(items) > 0 {
			v.cur.row = min(max(0, v.cur.row+d), len(items)-1)
			v.cur.selID = items[v.cur.row].id
		}
	}
	switch key {
	case "j", "down", "ctrl+n":
		move(1)
	case "k", "up", "ctrl+p":
		move(-1)
	case "pgdown":
		move(8)
	case "pgup":
		move(-8)
	case "home":
		move(-1 << 20)
	case "end":
		move(1 << 20)
	case "h", "left", "l", "right":
		if len(v.doc.Panels) > 0 {
			d := 1
			if key == "h" || key == "left" {
				d = -1
			}
			v.focusPanel(d)
		} else if v.doc.Board != nil && len(v.doc.Board.Columns) > 0 {
			d := 1
			if key == "h" || key == "left" {
				d = -1
			}
			v.cur.col = min(max(0, v.cur.col+d), len(v.doc.Board.Columns)-1)
			v.cur.row = 0
			v.cur.selID = ""
			if it := v.items(); len(it) > 0 {
				v.cur.selID = it[0].id
			}
		}
	case "[", "]", "tab", "shift+tab":
		if len(v.doc.Panels) > 0 && (key == "tab" || key == "shift+tab") {
			d := 1
			if key == "shift+tab" {
				d = -1
			}
			v.focusPanel(d)
		} else if len(v.doc.Tabs) > 1 {
			i := 0
			for j, t := range v.doc.Tabs {
				if t.ID == v.cur.tab {
					i = j
				}
			}
			d := 1
			if key == "[" || key == "shift+tab" {
				d = -1
			}
			i = (i + d + len(v.doc.Tabs)) % len(v.doc.Tabs)
			v.cur.tab, v.cur.row, v.cur.col, v.cur.selID = v.doc.Tabs[i].ID, 0, 0, ""
			v.cur.panel, v.cur.psel = "", nil
			return m, m.extRender(v)
		}
	case "/":
		if v.doc.Searchable {
			return m, m.openPrompt("Search", "Filters this view. Empty clears.", v.cur.query, func(m *Model, value string) (tea.Cmd, error) {
				v.cur.query, v.cur.row, v.cur.selID = value, 0, ""
				return m.extRender(v), nil
			})
		}
	case "enter":
		sel, ok := v.selected()
		if !ok {
			if len(v.doc.Panels) > 0 {
				if t := v.doc.Panels[v.focusedPanel()].Open; t != nil {
					return m.followExtTarget(v, *t)
				}
			}
			return m, nil
		}
		if sel.target != nil {
			return m.followExtTarget(v, *sel.target)
		}
		if v.doc.Open != "" {
			return m, m.extRunAction(v, v.doc.Open, sel.id, nil)
		}
	default:
		if len(key) == 1 && !v.busy {
			for _, a := range v.doc.Actions {
				if a.Key != key {
					continue
				}
				target := ""
				if a.Scope == "item" {
					sel, ok := v.selected()
					if !ok {
						v.notice = "Select an item first"
						return m, nil
					}
					target = sel.id
				}
				if a.Confirm != "" {
					id, tgt := a.ID, target
					m.crm.confirm = &crmConfirm{text: a.Confirm, yes: func(m *Model) tea.Cmd { return m.extRunAction(v, id, tgt, nil) }}
					return m, nil
				}
				return m, m.extRunAction(v, a.ID, target, nil)
			}
		}
	}
	return m, nil
}

// extRunAction asks the extension to perform an action.
func (m *Model) extRunAction(v *extView, action, target string, args map[string]string) tea.Cmd {
	v.busy, v.notice, v.err = true, "", ""
	req := ext.ActionRequest{Workspace: v.ws, Action: action, Tab: v.cur.tab, Item: v.cur.item, Target: target, State: v.cur.state, Args: args}
	mgr, ctx, id := m.extensions.mgr, m.contacts.ctx, v.ext
	return func() tea.Msg {
		c, cancel := extTimeout(ctx)
		defer cancel()
		res, err := mgr.RunAction(c, id, req)
		return extActionMsg{owner: v, res: res, err: err}
	}
}

// followExtTarget opens another view of the extension, or a core record.
func (m Model) followExtTarget(v *extView, t ext.Target) (Model, tea.Cmd) {
	return m.followExtTargetState(v, t, nil)
}

func (m Model) followExtTargetState(v *extView, t ext.Target, state map[string]string) (Model, tea.Cmd) {
	switch t.Kind {
	case "ext":
		if t.Action != "" {
			return m, m.extRunAction(v, t.Action, t.ID, nil)
		}
		if t.Workspace != "" && t.Workspace != v.ws {
			workspaceTitle := ""
			for _, w := range m.extensions.mgr.Workspaces() {
				if w.Extension == v.ext && w.ID == t.Workspace {
					workspaceTitle = w.Title
					break
				}
			}
			if workspaceTitle == "" {
				return m, nil
			}
			v.cur.workspace, v.cur.title = v.ws, v.title
			v.ws, v.title = t.Workspace, workspaceTitle
		}
		v.cur.remember()
		v.stack = append(v.stack, v.cur)
		v.cur = extFrame{tab: t.Tab, item: t.ID, state: mergeExtState(v.cur.state, state)}
		return m, m.extRender(v)
	}
	kind := map[string]string{"contact": "person", "organization": "organization", "task": "task", "activity": "activity", "project": "project"}[t.Kind]
	if kind == "" || t.ID == "" || m.crm.repo == nil {
		return m, nil
	}
	// Attach the extension position to the same back stack as core records.
	next, cmd := m.follow(domain.Ref{Kind: kind, ID: t.ID})
	if active := next.extensions.view; active != nil {
		next.closeExtView()
		p := &next.crm.nav[len(next.crm.nav)-1]
		p.extension = next.extensions.resume[active.ext+"/"+active.ws]
		p.name = active.title
	}
	return next, cmd
}

// applyExtResult carries out an action result in the documented order.
func (m Model) applyExtResult(v *extView, res ext.ActionResult) (Model, tea.Cmd) {
	switch {
	case res.Form != nil:
		return m, m.openExtForm(v, *res.Form)
	case res.Confirm != nil:
		c := res.Confirm
		m.crm.confirm = &crmConfirm{text: c.Text, yes: func(m *Model) tea.Cmd { return m.extRunAction(v, c.Action, c.Target, c.Args) }}
		return m, nil
	case res.Open != nil:
		if res.Notice != "" {
			v.notice = res.Notice
		}
		return m.followExtTargetState(v, *res.Open, res.State)
	}
	if len(res.State) > 0 {
		v.cur.state = mergeExtState(v.cur.state, res.State)
	}
	v.notice = res.Notice
	if res.Close {
		m.closeExtView()
		return m, nil
	}
	return m, m.extRender(v)
}

// openExtForm builds a host modal form from an extension's declaration, using
// the same field components as every other TideContacts form.
func (m *Model) openExtForm(v *extView, f ext.Form) tea.Cmd {
	var fields []formField
	refKeys := map[string]bool{}
	for _, fd := range f.Fields {
		switch fd.Type {
		case "multiline":
			fields = append(fields, textArea(fd.Key, fd.Label, fd.Value))
		case "select":
			cur := fd.Value
			opts := fd.Options
			if len(opts) == 0 {
				opts = []string{""}
			}
			if cur == "" {
				cur = opts[0]
			}
			fields = append(fields, field(fd.Key, fd.Label, cur, fd.Help, opts...))
		case "contact", "contacts":
			var refs []domain.Ref
			for _, r := range fd.Refs {
				refs = append(refs, domain.Ref{Kind: r.Kind, ID: r.ID, Name: r.Name})
			}
			fields = append(fields, referenceField(fd.Key, fd.Label, "entity", refs))
			refKeys[fd.Key] = true
		default:
			fields = append(fields, field(fd.Key, fd.Label, fd.Value, fd.Help))
		}
	}
	required := map[string]string{}
	for _, fd := range f.Fields {
		if fd.Required {
			required[fd.Key] = fd.Label
		}
	}
	mgr, ctx, id, formID, target, args, ws := m.extensions.mgr, m.contacts.ctx, v.ext, f.ID, f.Target, f.Args, v.ws
	state := v.cur.state
	return m.openForm(f.Title, fields, func(m *Model, values map[string]string) (tea.Cmd, error) {
		refs := map[string][]ext.RefItem{}
		for key := range refKeys {
			for _, r := range m.formReferences(key) {
				refs[key] = append(refs[key], ext.RefItem{ID: r.ID, Name: r.Name, Kind: r.Kind})
			}
			values[key] = ""
		}
		for key, label := range required {
			if strings.TrimSpace(values[key]) == "" && len(refs[key]) == 0 {
				return nil, fmt.Errorf("%s is required", label)
			}
		}
		m.crm.busy = true
		return func() tea.Msg {
			c, cancel := extTimeout(ctx)
			defer cancel()
			res, err := mgr.SubmitForm(c, id, ext.FormSubmit{Workspace: ws, Form: formID, Target: target, Args: args, State: state, Values: values, Refs: refs})
			return extSubmitMsg{owner: v, ext: id, res: res, err: err}
		}, nil
	})
}

// ---- rendering ----

func (u softUI) tone(t string) lipgloss.Color {
	th := u.r.Styles.Theme
	switch t {
	case "ok":
		return th.SuccessColor()
	case "warn":
		return th.WarningColor()
	case "danger":
		return th.ErrorColor()
	case "info":
		return th.LinkColor()
	case "accent":
		return u.accent
	case "muted":
		return u.muted
	}
	return u.text
}

func (u softUI) bar(progress, width int, tone string) string {
	if progress < 0 || width < 4 {
		return ""
	}
	filled := min(width, progress*width/100)
	on, off := "━", "─"
	if u.r.Styles.PlainUI {
		on, off = "#", "-"
	}
	return u.style(u.base, u.tone(tone)).Render(strings.Repeat(on, filled)) + u.style(u.base, u.muted).Render(strings.Repeat(off, width-filled))
}

func (m Model) extViewPanel(u softUI) *tideui.Overlay {
	v := m.extensions.view
	d := v.doc
	width, height := m.formSize(150)
	panels := len(d.Panels) > 0
	if panels { // dashboards use the whole terminal
		width, height = max(1, min(220, m.width-4)), max(1, m.height-4)
	}
	compact := panels && (width < twoColumnsFrom || height < 28)
	var head, body, foot []string
	// header: badge, tabs
	if d.Badge != "" || v.title != "" {
		badge := u.style(u.base, u.muted).Render(d.Badge)
		left := u.style(u.base, u.accent).Bold(true).Render(extSafe(firstNonEmptyStr(d.Title, v.title), 40))
		gap := max(1, width-2-lipgloss.Width(left)-lipgloss.Width(badge))
		head = append(head, u.line("  "+left+strings.Repeat(" ", gap)+badge, width))
	}
	if len(d.Tabs) > 0 {
		names := make([]string, len(d.Tabs))
		cur := 0
		for i, t := range d.Tabs {
			names[i] = t.Title
			if t.ID == v.cur.tab {
				cur = i
			}
		}
		if panels && compact {
			head = append(head, u.line("  "+u.strip(names, cur, width-4), width))
		} else {
			for _, l := range u.tabs(names, cur, width-2, true) {
				head = append(head, u.line("  "+l, width))
			}
		}
	}
	if !compact {
		head = append(head, u.blank(width))
	}
	// tiles
	if len(d.Tiles) > 0 {
		if compact {
			body = append(body, m.compactTiles(u, d.Tiles, width)...)
		} else {
			body = append(body, m.extTiles(u, d.Tiles, width)...)
			body = append(body, u.blank(width))
		}
	}
	if d.Heading != "" || d.Subheading != "" {
		l := u.style(u.base, u.text).Bold(true).Render(d.Heading)
		r := u.style(u.base, u.muted).Render(d.Subheading)
		gap := max(1, width-4-lipgloss.Width(l)-lipgloss.Width(r))
		body = append(body, u.line("  "+l+strings.Repeat(" ", gap)+r, width))
	}
	// footer: notices, status, hints
	for i, n := range d.Notices {
		if compact && i > 0 {
			break
		}
		mark := "• "
		if n.Tone == "warn" || n.Tone == "danger" {
			mark = "! "
		}
		foot = append(foot, u.line(u.style(u.base, u.tone(n.Tone)).Render("  "+mark+n.Text), width))
	}
	switch {
	case v.err != "":
		foot = append(foot, u.wrapNote(v.err, width, 2, true)...)
	case v.busy && d.Tabs == nil && d.Board == nil && d.List == nil && len(d.Sections) == 0 && !panels:
		foot = append(foot, u.note("Loading…", width, false))
	case v.notice != "":
		foot = append(foot, u.note(extSafe(v.notice, 160), width, false))
	}
	pairs := []string{}
	if panels {
		pairs = append(pairs, "tab", "panel", "j/k", "item", "enter", "open")
		if len(d.Tabs) > 1 {
			pairs = append(pairs, "[ ]", "view")
		}
		pairs = append(pairs, "?", "keys")
	}
	if d.Board != nil {
		pairs = append(pairs, "h/l", "stage")
	}
	if d.Board != nil || d.List != nil || len(d.Sections) > 0 {
		pairs = append(pairs, "j/k", "item")
	}
	for _, a := range d.Actions {
		pairs = append(pairs, a.Key, strings.ToLower(a.Label))
	}
	if !panels && (d.Open != "" || len(d.Sections) > 0) {
		pairs = append(pairs, "enter", "open")
	}
	if !panels && len(d.Tabs) > 1 {
		pairs = append(pairs, "[ ]", "tab")
	}
	if d.Searchable {
		pairs = append(pairs, "/", "search")
	}
	closeLabel := "close"
	if len(v.stack) > 0 {
		closeLabel = "back"
	}
	pairs = append(pairs, "esc", closeLabel)
	foot = append(foot, u.hints(width, pairs...))
	avail := max(3, height-len(head)-len(body)-len(foot)-1)
	switch {
	case v.help:
		body = m.extHelp(u, v, width)
	case panels:
		body = append(body, m.extPanels(u, v, width, avail)...)
	case d.Board != nil && len(d.Board.Columns) > 0:
		body = append(body, m.extBoard(u, v, width, avail)...)
	case d.List != nil:
		body = append(body, m.extList(u, v, width, avail)...)
	}
	if len(d.Sections) > 0 && !panels && !v.help {
		body = append(body, m.extSections(u, v, width, avail)...)
	}
	if !panels && d.Board == nil && d.List == nil && len(d.Sections) == 0 && !v.busy && v.err == "" {
		body = append(body, u.note(firstNonEmptyStr(d.Empty, "Nothing to show"), width, false))
	}
	lines := append(append([]string{}, head...), body...)
	room := max(0, height-len(foot))
	if len(lines) > room {
		lines = lines[:room]
	}
	for len(lines) < room {
		lines = append(lines, u.blank(width))
	}
	lines = append(lines, foot...)
	return u.panel(strings.ToLower(extSafe(v.title, 30)), width, lines)
}

func firstNonEmptyStr(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func (m Model) extTiles(u softUI, tiles []ext.Tile, width int) []string {
	n := len(tiles)
	inner := width - 4
	if inner < n*18 { // too narrow for tiles side by side: one compact line each
		var out []string
		for _, t := range tiles {
			txt := u.style(u.base, u.muted).Render("  "+t.Label+"  ") + u.style(u.base, u.tone(t.Tone)).Bold(true).Render(t.Value)
			if t.Note != "" {
				txt += u.style(u.base, u.muted).Render("  " + t.Note)
			}
			out = append(out, u.line(txt, width))
		}
		return out
	}
	gap := 1
	w := (inner - gap*(n-1)) / n
	rows := [3][]string{}
	for _, t := range tiles {
		cells := [3]string{
			u.style(u.surface, u.muted).Render(" " + strings.ToUpper(t.Label)),
			u.style(u.surface, u.tone(t.Tone)).Bold(true).Render(" " + t.Value),
			u.style(u.surface, u.muted).Render(" " + t.Note),
		}
		for i := range rows {
			rows[i] = append(rows[i], u.cell(cells[i], w, u.surface))
		}
	}
	var out []string
	for _, r := range rows {
		out = append(out, u.line("  "+strings.Join(r, strings.Repeat(" ", gap)), width))
	}
	return out
}

func (m Model) extBoard(u softUI, v *extView, width, avail int) []string {
	cols := v.doc.Board.Columns
	inner := width - 4
	minCol := 24
	visible := cols
	first := 0
	if inner/len(cols) < minCol { // narrow: a window of columns around the cursor
		n := max(1, inner/minCol)
		first = min(max(0, v.cur.col-n/2), max(0, len(cols)-n))
		visible = cols[first:min(len(cols), first+n)]
	}
	gap := 1
	cw := (inner - gap*(len(visible)-1)) / len(visible)
	rendered := make([][]string, len(visible))
	tallest := 0
	for ci, c := range visible {
		focused := first+ci == v.cur.col
		var lines []string
		bar := "▔"
		if u.r.Styles.PlainUI {
			bar = "="
		}
		lines = append(lines, u.style(u.base, u.tone(c.Tone)).Render(strings.Repeat(bar, cw)))
		ts := u.style(u.base, u.tone(c.Tone))
		if focused {
			ts = ts.Bold(true).Underline(true)
		}
		lines = append(lines, u.cell(ts.Render(c.Title), cw, u.base))
		total := u.style(u.base, u.text).Bold(true).Render(c.Total)
		if c.Note != "" {
			total += u.style(u.base, u.muted).Render("  " + c.Note)
		}
		lines = append(lines, u.cell(total, cw, u.base), u.blank(cw))
		start := 0
		per := 5
		if focused {
			budget := max(1, (avail-len(lines))/per)
			start = max(0, min(v.cur.row-budget+1, max(0, len(c.Cards)-budget)))
			if v.cur.row < start {
				start = v.cur.row
			}
		}
		budget := max(1, (avail-len(lines))/per)
		for ri := start; ri < len(c.Cards) && ri < start+budget; ri++ {
			lines = append(lines, m.extCard(u, c.Cards[ri], cw, focused && ri == v.cur.row)...)
		}
		if len(c.Cards) == 0 {
			lines = append(lines, u.cell(u.style(u.base, u.muted).Render("  —"), cw, u.base))
		}
		if hidden := len(c.Cards) - (start + budget); hidden > 0 {
			lines = append(lines, u.cell(u.style(u.base, u.muted).Render(fmt.Sprintf("  +%d more", hidden)), cw, u.base))
		}
		rendered[ci] = lines
		tallest = max(tallest, len(lines))
	}
	var out []string
	for i := 0; i < min(tallest, avail); i++ {
		parts := make([]string, len(visible))
		for ci := range visible {
			if i < len(rendered[ci]) {
				parts[ci] = rendered[ci][i]
			} else {
				parts[ci] = u.blank(cw)
			}
		}
		out = append(out, u.line("  "+strings.Join(parts, strings.Repeat(" ", gap)), width))
	}
	if first > 0 || first+len(visible) < len(cols) {
		out = append(out, u.line(u.style(u.base, u.muted).Render(fmt.Sprintf("  stages %d–%d of %d", first+1, first+len(visible), len(cols))), width))
	}
	return out
}

// extCard renders one card: title, amount, progress, meta, badge, and a spacer.
func (m Model) extCard(u softUI, c ext.Card, w int, selected bool) []string {
	bg := u.surface
	if selected {
		bg = tideui.MixColors(u.base, u.accent, .22)
	}
	fg := tideui.ReadableText(u.text, bg, 4.5)
	row := func(s string) string { return u.cell(s, w, bg) }
	mark := " "
	if selected {
		mark = "▌"
		if u.r.Styles.PlainUI {
			mark = ">"
		}
	}
	lines := []string{row(u.style(bg, u.accent).Render(mark) + u.style(bg, fg).Bold(true).Render(c.Title))}
	amt := c.Amount
	if c.Subtitle != "" {
		amt = strings.TrimSpace(c.Subtitle + "  " + c.Amount)
	}
	lines = append(lines, row(u.style(bg, fg).Render(" "+amt)))
	if c.Progress >= 0 {
		barw := max(4, w-2)
		on, off := "━", "─"
		filled := min(barw, c.Progress*barw/100)
		lines = append(lines, row(" "+u.style(bg, u.tone(c.Tone)).Render(strings.Repeat(on, filled))+u.style(bg, u.muted).Render(strings.Repeat(off, barw-filled))))
	}
	left, right := c.Left, c.Right
	if left != "" || right != "" {
		gap := max(1, w-2-lipgloss.Width(left)-lipgloss.Width(right))
		lines = append(lines, row(u.style(bg, u.muted).Render(" "+left+strings.Repeat(" ", gap)+right)))
	}
	if c.Badge != "" {
		lines = append(lines, row(u.style(bg, u.tone(c.BadgeTone)).Render(" ! "+c.Badge)))
	}
	lines = append(lines, u.blank(w))
	return lines
}

func (m Model) extList(u softUI, v *extView, width, avail int) []string {
	l := v.doc.List
	var out []string
	if len(l.Columns) > 0 {
		out = append(out, u.line(u.style(u.base, u.muted).Render("  "+strings.Join(l.Columns, "  ·  ")), width))
	}
	start, end := tideui.VisibleRange(len(l.Rows), v.cur.row, max(1, avail-len(out)))
	for i := start; i < end; i++ {
		r := l.Rows[i]
		suffix := strings.Join(r.Cells[min(1, len(r.Cells)):], "  ")
		if r.Badge != "" {
			suffix = r.Badge + "  " + suffix
		}
		first := ""
		if len(r.Cells) > 0 {
			first = r.Cells[0]
		}
		out = append(out, u.r.RenderSoftRow(tideui.SoftRow{Text: first, Suffix: suffix + " ", Selected: i == v.cur.row}, width))
	}
	if len(l.Rows) == 0 {
		out = append(out, u.note(firstNonEmptyStr(v.doc.Empty, "Nothing to show"), width, false))
	}
	return out
}

func (m Model) extSections(u softUI, v *extView, width, avail int) []string {
	var lines []string
	selIdx, sel := -1, 0
	if v.doc.Board == nil && v.doc.List == nil {
		selIdx = v.cur.row
	}
	for _, s := range v.doc.Sections {
		if s.Title != "" {
			lines = append(lines, u.line(u.style(u.base, u.accent).Bold(true).Render("  "+s.Title), width))
		}
		for _, ln := range s.Lines {
			isSel := false
			prefix := "   "
			if ln.Target != nil {
				if sel == selIdx {
					isSel, prefix = true, " › "
					if u.r.Styles.PlainUI {
						prefix = " > "
					}
				}
				sel++
			}
			txt := u.style(u.base, u.tone(ln.Tone)).Render(ln.Text)
			if ln.Label != "" {
				txt = u.style(u.base, u.muted).Render(fmt.Sprintf("%-14s", ansi.Truncate(ln.Label, 13, "…"))) + txt
			}
			styled := u.style(u.base, u.accent).Render(prefix) + txt
			if isSel {
				styled = u.cell(u.style(tideui.MixColors(u.base, u.accent, .2), u.text).Render(prefix)+txt, width, tideui.MixColors(u.base, u.accent, .2))
			}
			lines = append(lines, u.line(styled, width))
		}
		lines = append(lines, u.blank(width))
	}
	// Window around the selected line when the body is taller than the space.
	if len(lines) > avail && selIdx >= 0 {
		at := 0
		n := 0
		for i, l := range lines {
			if strings.Contains(ansi.Strip(l), "› ") || strings.Contains(ansi.Strip(l), " > ") {
				at, n = i, n+1
			}
		}
		_ = n
		start, end := tideui.VisibleRange(len(lines), at, max(1, avail))
		return lines[start:end]
	}
	return lines[:min(len(lines), max(1, avail))]
}

func copyState(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeExtState(previous, updates map[string]string) map[string]string {
	out := copyState(previous)
	for k, value := range updates {
		out[k] = value
	}
	return out
}

// hasWorkspace reports whether an enabled extension contributes ws.
func (m Model) hasWorkspace(extID, ws string) bool {
	for _, w := range m.extensions.mgr.Workspaces() {
		if w.Extension == extID && w.ID == ws {
			return true
		}
	}
	return false
}

// firstWorkspace returns the id of an extension's first workspace.
func (m Model) firstWorkspace(extID string) string {
	for _, w := range m.extensions.mgr.Workspaces() {
		if w.Extension == extID {
			return w.ID
		}
	}
	return ""
}

// applyExtCommandResult handles a result that arrived without a workspace open
// (a palette command or a form started from a contact): open the workspace at
// the requested place, or the requested form.
func (m Model) applyExtCommandResult(extID string, res ext.ActionResult) (Model, tea.Cmd) {
	ws := m.firstWorkspace(extID)
	if res.Open != nil && res.Open.Workspace != "" && m.hasWorkspace(extID, res.Open.Workspace) {
		ws = res.Open.Workspace
	}
	if res.Form != nil {
		v := &extView{ext: extID, ws: ws}
		return m, m.openExtForm(v, *res.Form)
	}
	if res.Open != nil && ws != "" {
		t := *res.Open
		if t.Kind == "ext" {
			m2, _ := m.openExtWorkspace(extID, ws)
			v := m2.extensions.view
			v.stack = nil
			v.cur = extFrame{tab: t.Tab, item: t.ID, state: res.State}
			return m2, m2.extRender(v)
		}
		v := &extView{ext: extID, ws: ws}
		return m.followExtTarget(v, t)
	}
	if res.Notice != "" {
		m.feedback = res.Notice
	}
	return m, nil
}
