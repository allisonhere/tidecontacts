package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/ext"
	"github.com/allisonhere/tideui"
	tea "github.com/charmbracelet/bubbletea"
)

// Extension UI: a manager modal reached from the command palette ("Manage
// extensions…") and host-rendered contributions from enabled extensions. The
// host owns layout, focus, theme and keys; extension text is sanitized again
// here, and extension work always runs in tea.Cmds, never in Update or View.

const extPanelSlot = ext.SlotContactDossier

type extState struct {
	mgr       *ext.Manager
	view      *extView
	resume    map[string]*extView
	modal     *extModal
	panelFor  string
	panelRows []crmRow
}

type extModal struct {
	mode        string // list, detail, result, confirm
	items       []ext.Info
	sel, scroll int
	busy        bool
	err, note   string
	title       string
	content     ext.Content
	warnings    []string
}

type (
	extLoadedMsg struct {
		items    []ext.Info
		warnings []string
	}
	extDoneMsg struct {
		note string
		err  error
	}
	extResultMsg struct {
		owner *extModal
		ext   string
		title string
		res   ext.ActionResult
		err   error
	}
	extPanelMsg struct {
		seq  uint64
		id   string
		rows []crmRow
	}
)

func extTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, 45*time.Second)
}

func extSafe(s string, n int) string { return ext.Clean(s, n, false) }

// extCommandRegistry adds enabled extension commands to the palette. With no
// extensions it is exactly the existing registry.
func (m Model) paletteMatches() []app.Command {
	matches := m.registry.Filter(m.prompt.Value())
	if m.extensions.mgr == nil {
		return matches
	}
	refs := m.extensions.mgr.Commands()
	spaces := m.extWorkspaceEntries()
	if len(refs) == 0 && len(spaces) == 0 {
		return matches
	}
	r := app.Registry{}
	for _, c := range m.registry.Commands() {
		_ = r.Register(c)
	}
	for _, w := range spaces {
		_ = r.Register(app.Command{ID: w.ID, Label: "Open " + w.Label})
	}
	for _, c := range refs {
		_ = r.Register(app.Command{ID: "ext:" + c.Extension + ":" + c.ID, Label: extSafe(c.ExtensionName, 40) + ": " + extSafe(c.Title, 60)})
	}
	return r.Filter(m.prompt.Value())
}

func (m Model) extDispatch(id string) (tea.Model, tea.Cmd) {
	m.menuOpen = false
	mgr := m.extensions.mgr
	if mgr == nil {
		m.feedback = "Extensions are unavailable"
		return m, nil
	}
	if id == "extensions-manage" {
		m.extensions.modal = &extModal{mode: "list", busy: true}
		return m, m.extLoad()
	}
	if strings.HasPrefix(id, "ext-ws:") {
		p := strings.SplitN(strings.TrimPrefix(id, "ext-ws:"), ":", 2)
		if len(p) == 2 {
			return m.openExtWorkspace(p[0], p[1])
		}
		return m, nil
	}
	parts := strings.SplitN(strings.TrimPrefix(id, "ext:"), ":", 2)
	if len(parts) != 2 {
		return m, nil
	}
	modal := &extModal{mode: "result", busy: true, title: parts[1]}
	for _, c := range mgr.Commands() {
		if c.Extension == parts[0] && c.ID == parts[1] {
			modal.title = c.Title
		}
	}
	m.extensions.modal = modal
	args := map[string]string{}
	// Context for commands that act on the selection (read-only facts: an id and a kind).
	if m.isContacts() && m.contacts.contact.ID != "" {
		args["context_contact"], args["context_kind"] = m.contacts.contact.ID, m.contacts.contact.Kind
	}
	ctx := m.contacts.ctx
	return m, func() tea.Msg {
		c, cancel := extTimeout(ctx)
		defer cancel()
		res, err := mgr.RunCommandResult(c, parts[0], parts[1], args)
		return extResultMsg{owner: modal, ext: parts[0], title: modal.title, res: res, err: err}
	}
}

func (m Model) extLoad() tea.Cmd {
	mgr, ctx := m.extensions.mgr, m.contacts.ctx
	return func() tea.Msg {
		c, cancel := extTimeout(ctx)
		defer cancel()
		_ = mgr.Discover(c)
		return extLoadedMsg{items: mgr.List(), warnings: mgr.Warnings()}
	}
}

func (m Model) extAction(name string, do func(ctx context.Context) error) tea.Cmd {
	ctx := m.contacts.ctx
	return func() tea.Msg {
		c, cancel := extTimeout(ctx)
		defer cancel()
		return extDoneMsg{note: name, err: do(c)}
	}
}

// updateExtensions routes extension messages and, while the modal is open,
// every key. It returns handled=false when extensions are not involved.
func (m Model) updateExtensions(msg tea.Msg) (Model, tea.Cmd, bool) {
	e := &m.extensions
	switch v := msg.(type) {
	case extLoadedMsg:
		if e.modal != nil && (e.modal.mode == "list" || e.modal.mode == "detail" || e.modal.mode == "confirm") {
			e.modal.items, e.modal.warnings, e.modal.busy = v.items, v.warnings, false
			e.modal.sel = min(e.modal.sel, max(0, len(v.items)-1))
		}
		return m, nil, true
	case extDoneMsg:
		if e.modal != nil {
			e.modal.busy, e.modal.note, e.modal.err = false, "", ""
			if v.err != nil {
				e.modal.err = extSafe(v.err.Error(), 240)
			} else {
				e.modal.note = v.note
			}
			if e.modal.mode == "confirm" {
				e.modal.mode = "list"
			}
		}
		return m, m.extLoad(), true
	case extResultMsg:
		if e.modal != v.owner || e.modal == nil {
			return m, nil, true
		}
		e.modal.busy = false
		if v.err != nil {
			e.modal.err = extSafe(v.err.Error(), 240)
			return m, nil, true
		}
		if v.res.Form != nil || v.res.Open != nil {
			e.modal = nil
			next, cmd := m.applyExtCommandResult(v.ext, v.res)
			return next, cmd, true
		}
		e.modal.content = ext.Content{Title: v.res.Title, Lines: v.res.Lines}
		if v.res.Notice != "" {
			e.modal.content.Lines = append(e.modal.content.Lines, ext.Line{Text: v.res.Notice})
		}
		return m, nil, true
	case extPanelMsg:
		if v.seq == m.crm.dossierSeq {
			e.panelFor, e.panelRows = v.id, v.rows
		}
		return m, nil, true
	case tea.KeyMsg:
		if e.modal == nil {
			return m.updateExtView(msg)
		}
		next, cmd := m.extKey(v)
		return next, cmd, true
	case extViewMsg, extActionMsg, extSubmitMsg:
		return m.updateExtView(msg)
	}
	return m, nil, false
}

func (m Model) extKey(k tea.KeyMsg) (Model, tea.Cmd) {
	e := &m.extensions
	d := e.modal
	key := k.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	var cur *ext.Info
	if len(d.items) > 0 && d.sel < len(d.items) {
		cur = &d.items[d.sel]
	}
	switch d.mode {
	case "result":
		if key == "esc" || key == "enter" || key == "q" {
			e.modal = nil
		}
		return m, nil
	case "confirm":
		switch key {
		case "y", "enter":
			if cur == nil {
				d.mode = "list"
				return m, nil
			}
			id, mgr := cur.ID, e.mgr
			d.busy, d.err = true, ""
			return m, m.extAction("Enabled "+id, func(ctx context.Context) error { return mgr.Enable(ctx, id, true) })
		case "n", "esc":
			d.mode = "list"
		}
		return m, nil
	}
	switch key {
	case "esc":
		if d.mode == "detail" {
			d.mode, d.scroll = "list", 0
		} else {
			e.modal = nil
		}
	case "down", "j", "ctrl+n":
		if d.mode == "detail" {
			d.scroll++
		} else {
			d.sel = min(d.sel+1, max(0, len(d.items)-1))
		}
	case "up", "k", "ctrl+p":
		if d.mode == "detail" {
			d.scroll = max(0, d.scroll-1)
		} else {
			d.sel = max(0, d.sel-1)
		}
	case "enter":
		if d.mode == "list" && cur != nil {
			d.mode, d.scroll, d.err, d.note = "detail", 0, "", ""
		}
	case "e":
		if cur == nil || d.busy {
			return m, nil
		}
		d.err, d.note = "", ""
		id, mgr := cur.ID, e.mgr
		switch cur.Status {
		case ext.StatusEnabled, ext.StatusFailed:
			d.busy = true
			return m, m.extAction("Disabled "+id, func(ctx context.Context) error { return mgr.Disable(ctx, id) })
		case ext.StatusInvalid, ext.StatusIncompatible, ext.StatusConflict:
			d.err = "Cannot enable: " + extSafe(cur.StatusDetail, 160)
		default:
			// Always show what is being approved, even when nothing is new.
			d.mode = "confirm"
		}
	case "h":
		if cur != nil && !d.busy && (cur.Status == ext.StatusEnabled) {
			id, mgr := cur.ID, e.mgr
			d.busy, d.err, d.note = true, "", ""
			return m, m.extAction("Healthy", func(ctx context.Context) error { _, err := mgr.CheckHealth(ctx, id); return err })
		}
	}
	return m, nil
}

// extensionsPanel renders the manager, a confirmation or a command result.
func (m Model) extensionsPanel(u softUI) *tideui.Overlay {
	d := m.extensions.modal
	width, height := m.formSize(76)
	list := max(1, height-6)
	var body []string
	add := func(lines ...string) { body = append(body, lines...) }
	status := func() {
		switch {
		case d.busy:
			add(u.note("Working…", width, false))
		case d.err != "":
			add(u.wrapNote(d.err, width, 3, true)...)
		case d.note != "":
			add(u.note(d.note, width, false))
		}
	}
	switch d.mode {
	case "result":
		if d.busy {
			add(u.note("Running…", width, false))
		} else if d.err != "" {
			add(u.wrapNote(d.err, width, 4, true)...)
		}
		for _, l := range d.content.Lines {
			prefix := ""
			switch l.Level {
			case "warn":
				prefix = "! "
			case "ok":
				prefix = "✓ "
			}
			text := prefix + l.Text
			if l.Label != "" {
				text = fmt.Sprintf("%s  %s", l.Label, text)
			}
			add(u.wrapNote(text, width, 2, false)...)
		}
		if len(d.content.Lines) == 0 && !d.busy && d.err == "" {
			add(u.note("Nothing to show", width, false))
		}
		add(u.blank(width), u.hints(width, "esc", "close"))
		title := d.title
		if d.content.Title != "" {
			title = d.content.Title
		}
		return u.panel(strings.ToLower(extSafe(title, 60)), width, body)
	case "confirm":
		cur := d.items[d.sel]
		add(u.wrapNote(extSafe(cur.Name, 60)+" "+extSafe(cur.Version, 20)+" asks for these permissions:", width, 2, false)...)
		for _, p := range cur.Permissions {
			mark := "  · "
			if p.Sensitive {
				mark = "  ! "
			}
			note := ""
			if p.Enforcement != ext.EnforcedByHost {
				note = " (" + string(p.Enforcement) + ")"
			}
			add(u.line(u.style(u.base, u.text).Render(mark+p.ID)+u.style(u.base, u.muted).Render("  "+p.Description+note), width))
		}
		if len(cur.Permissions) == 0 {
			add(u.note("No permissions requested", width, false))
		}
		add(u.blank(width))
		add(u.wrapNote("Permissions limit what the host API returns. They do not sandbox the program, which runs as you.", width, 3, false)...)
		status()
		add(u.blank(width), u.hints(width, "y", "enable", "esc", "cancel"))
		return u.panel("enable extension", width, body)
	case "detail":
		cur := d.items[d.sel]
		lines := m.extDetailLines(u, cur, width)
		visible := max(3, height-5)
		d.scroll = min(d.scroll, max(0, len(lines)-visible))
		start, end := d.scroll, min(len(lines), d.scroll+visible)
		add(lines[start:end]...)
		status()
		add(u.blank(width), u.hints(width, "↑↓", "scroll", "e", "enable/disable", "h", "health", "esc", "back"))
		return u.panel(strings.ToLower(extSafe(cur.Name, 40)), width, body)
	}
	if len(d.items) == 0 && !d.busy {
		add(u.note("No extensions installed", width, false), u.blank(width))
		add(u.wrapNote("Extensions are optional. Place one in its own folder inside the extensions directory; see docs/extensions.", width, 3, false)...)
	}
	r := u.r
	start, end := tideui.VisibleRange(len(d.items), d.sel, list)
	for i := start; i < end; i++ {
		it := d.items[i]
		add(r.RenderSoftRow(tideui.SoftRow{Text: extSafe(it.Name, 40), Suffix: "v" + extSafe(it.Version, 16) + " · " + string(it.Status) + " ", Selected: i == d.sel}, width))
	}
	for _, w := range d.warnings {
		add(u.note(extSafe(w, 120), width, true))
	}
	status()
	add(u.blank(width), u.hints(width, "↑↓", "select", "enter", "details", "e", "enable/disable", "esc", "close"))
	return u.panel("extensions", width, body)
}

func (m Model) extDetailLines(u softUI, i ext.Info, width int) []string {
	var out []string
	kv := func(k, v string) {
		if v != "" {
			out = append(out, u.row(k, false, u.style(u.base, u.text).Render(extSafe(v, 200)), width, 12))
		}
	}
	out = append(out, u.wrapNote(i.Description, width, 3, false)...)
	kv("id", i.ID)
	kv("author", i.Author)
	kv("status", string(i.Status))
	kv("detail", i.StatusDetail)
	kv("api", fmt.Sprint(i.API))
	out = append(out, u.blank(width), u.note("Permissions", width, false))
	for _, p := range i.Permissions {
		state := "requested"
		if p.Granted {
			state = "granted"
		}
		note := ""
		if p.Enforcement != ext.EnforcedByHost {
			note = " · " + string(p.Enforcement)
		}
		out = append(out, u.line(u.style(u.base, u.text).Render("  "+p.ID)+u.style(u.base, u.muted).Render("  "+state+note), width))
	}
	if len(i.Permissions) == 0 {
		out = append(out, u.note("none requested", width, false))
	}
	list := func(name string, items []string) {
		if len(items) > 0 {
			out = append(out, u.blank(width), u.note(name, width, false))
			for _, s := range items {
				out = append(out, u.line(u.style(u.base, u.text).Render("  "+extSafe(s, 120)), width))
			}
		}
	}
	var provides, cmds, panels []string
	for _, p := range i.Provides {
		provides = append(provides, fmt.Sprintf("%s v%d", p.ID, p.Version))
	}
	for _, c := range i.Commands {
		cmds = append(cmds, c.Title)
	}
	for _, p := range i.Panels {
		panels = append(panels, p.Title+" ("+p.Slot+")")
	}
	list("Requires", i.Requires)
	list("Provides", provides)
	list("Commands", cmds)
	list("Panels", panels)
	list("Events", i.Events)
	out = append(out, u.blank(width), u.note("Health", width, false))
	h := i.Health
	kv("last ok", h.LastOK)
	kv("calls", fmt.Sprintf("%d · failures in a row %d", h.Invocations, h.ConsecutiveFailures))
	kv("events", fmt.Sprintf("%d delivered · %d dropped", h.EventsDelivered, h.EventsDropped))
	kv("last error", h.LastError)
	kv("diagnostic", h.Diagnostic)
	return out
}

// loadExtPanels asks enabled extensions for panels in the contact dossier slot.
// It runs beside the dossier load and arrives separately, so a slow extension
// never delays the built-in rows.
func (m *Model) loadExtPanels(seq uint64, id string) tea.Cmd {
	mgr := m.extensions.mgr
	if mgr == nil || id == "" {
		return nil
	}
	panels := mgr.Panels(extPanelSlot)
	if len(panels) == 0 {
		m.extensions.panelFor, m.extensions.panelRows = "", nil
		return nil
	}
	ctx := m.contacts.ctx
	return func() tea.Msg {
		c, cancel := extTimeout(ctx)
		defer cancel()
		var rows []crmRow
		for _, p := range panels {
			content, err := mgr.RenderPanel(c, p.Extension, p.ID, id)
			rows = append(rows, crmRow{kind: "header", text: extSafe(p.Title, 40)})
			if err != nil {
				rows = append(rows, dim("unavailable: "+extSafe(err.Error(), 100)))
				continue
			}
			for _, l := range content.Lines {
				text := l.Text
				if l.Label != "" {
					text = l.Label + "  " + text
				}
				if l.Level == "warn" {
					text = "! " + text
				}
				rows = append(rows, txt(text))
			}
		}
		return extPanelMsg{seq: seq, id: id, rows: rows}
	}
}

// extPanelRows returns contributed rows for the contact on screen.
func (m Model) extPanelRows(id string) []crmRow {
	if m.extensions.panelFor != id {
		return nil
	}
	return m.extensions.panelRows
}

// ---- signals in the People Inbox ----

// extSignalRows turns contributed signals into Inbox rows under a heading per
// extension. They sit after the core entries and never reorder them.
func extSignalRows(signals []ext.SignalRef, names map[string]string) []crmRow {
	var rows []crmRow
	group := ""
	tags := map[string]string{"danger": "!!", "warn": "! ", "info": "· "}
	for _, s := range signals {
		if s.Extension != group {
			group = s.Extension
			rows = append(rows, head(extSafe(firstNonEmptyStr(names[s.Extension], s.Extension), 40)+" attention"))
		}
		rows = append(rows, crmRow{kind: "extsignal", id: s.Extension + "/" + s.Key, tag: tags[s.Severity], text: s.Title, meta: s.Type, data: s})
	}
	return rows
}

// loadExtSignals fetches signals with a short budget: a slow or failing
// extension contributes nothing and never delays the built-in Inbox for long.
func loadExtSignals(ctx context.Context, mgr *ext.Manager) []crmRow {
	if mgr == nil {
		return nil
	}
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	list := mgr.Signals(c)
	if len(list) == 0 {
		return nil
	}
	names := map[string]string{}
	for _, s := range list {
		if info, err := mgr.Inspect(s.Extension); err == nil {
			names[s.Extension] = info.Name
		}
	}
	return extSignalRows(list, names)
}

func extSignalDetail(s ext.SignalRef) []crmRow {
	rows := []crmRow{txt(extSafe(s.Type, 40)), head("Why this is here"), txt(s.Reason)}
	if len(s.Facts) > 0 {
		rows = append(rows, head("Recorded facts"))
		for _, f := range s.Facts {
			rows = append(rows, txt("· "+f))
		}
	}
	if s.Date != "" {
		rows = append(rows, txt("Date: "+s.Date))
	}
	if s.Action != "" {
		rows = append(rows, head("Suggested next step"), txt(s.Action))
	}
	return rows
}

// extSignalAction asks the owning extension to dismiss or snooze a signal.
func extSignalAction(ctx context.Context, mgr *ext.Manager, s ext.SignalRef, action string, args map[string]string) error {
	if mgr == nil {
		return fmt.Errorf("extensions are unavailable")
	}
	ws := ""
	for _, w := range mgr.Workspaces() {
		if w.Extension == s.Extension {
			ws = w.ID
		}
	}
	res, err := mgr.RunAction(ctx, s.Extension, ext.ActionRequest{Workspace: ws, Action: action, Target: s.Key, Args: args})
	if err != nil {
		return err
	}
	for _, e := range res.Errors {
		return fmt.Errorf("%s", e)
	}
	return nil
}
