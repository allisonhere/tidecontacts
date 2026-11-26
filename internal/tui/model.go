package tui

import (
	"context"
	"fmt"
	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/config"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/ext"
	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

type Ready struct {
	// Extensions is optional; nil means the extension system is not in use.
	Extensions *ext.Manager
	Repo       app.Contacts
	MCP        app.MCPState
	Context    context.Context
	Config     config.Config
	Err        error
}
type overlay int

const (
	none overlay = iota
	palette
	help
)

type Preferences struct {
	Left, Right float64
	Theme       string
}
type Model struct {
	helpBrowser                                                 tideui.HelpBrowser
	menuOpen                                                    bool
	menuSelected                                                int
	crm                                                         crmState
	contacts                                                    contactsState
	cfg                                                         config.Config
	registry                                                    app.Registry
	state                                                       app.State
	width, height, focus, selected, paletteSelected, helpOffset int
	overlay                                                     overlay
	search, prompt                                              textinput.Model
	queries                                                     [8]string
	editing, ready                                              bool
	load                                                        tea.Cmd
	Err                                                         error
	left, right                                                 tideui.PaneRatio
	feedback                                                    string
	extensions                                                  extState
	mcp                                                         mcpSettings
}

func New(load tea.Cmd) Model {
	m := NewReady(config.Defaults())
	m.ready = false
	m.load = load
	return m
}
func NewReady(c config.Config) Model {
	r, _ := c.Registry()
	s := textinput.New()
	s.Prompt = "/ "
	s.CharLimit = 256
	p := textinput.New()
	p.Prompt = ": "
	p.CharLimit = 128
	m := Model{cfg: c, registry: r, search: s, prompt: p, ready: true, focus: 0}
	m.state.Navigate(c.DefaultWorkspace)
	m.selected = m.state.Active
	m.left = tideui.NewPaneRatio(tideui.PaneRatioOptions{Initial: c.Layout.Left, Min: .15, Max: .35, Step: .02})
	m.right = tideui.NewPaneRatio(tideui.PaneRatioOptions{Initial: c.Layout.Right, Min: .15, Max: .35, Step: .02})
	return m
}
func (m Model) IsReady() bool { return m.ready }

func (m Model) Preferences() Preferences {
	theme := m.cfg.Theme
	if p := m.crm.picker; p != nil && p.originalTheme != "" {
		theme = p.originalTheme
	}
	return Preferences{Left: m.left.Value(), Right: m.right.Value(), Theme: theme}
}

func (m Model) Init() tea.Cmd { return tea.Batch(m.load, tickReminders()) }
func (m Model) mode() tideui.LayoutMode {
	if m.width < m.cfg.Layout.Narrow {
		return tideui.Tabbed
	}
	if m.width < m.cfg.Layout.Wide {
		return tideui.SidebarOnly
	}
	return tideui.ThreeColumn
}
func (m Model) focusCount() int {
	if m.mode() == tideui.SidebarOnly {
		return 2
	}
	return 3
}
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.updateMCP(msg); handled {
		return next, cmd
	}
	if next, cmd, handled := m.updateExtensions(msg); handled {
		return next, cmd
	}
	if next, cmd, handled := m.updateDirectories(msg); handled {
		return next, cmd
	}
	if k, ok := msg.(tea.KeyMsg); ok && m.ready && m.overlay == none && !m.editing && m.contacts.editor == nil && m.crm.form == nil && m.crm.prompt == nil && m.crm.picker == nil && m.crm.confirm == nil && m.contacts.transfer == nil && m.contacts.confirm == "" {
		if k.String() == "ctrl+g" {
			m.menuOpen = !m.menuOpen
			m.menuSelected = m.state.Active
			return m, nil
		}
		if m.menuOpen {
			return m.updateMenu(k)
		}

	}
	if next, cmd, handled := m.updateCRM(msg); handled {
		return next, cmd
	}
	if next, cmd, handled := m.updateContacts(msg); handled {
		if _, ok := msg.(detailResult); ok {
			return next, tea.Batch(cmd, next.loadDossier())
		}
		return next, cmd
	}
	switch v := msg.(type) {
	case editMsg:
		return m.openEdit(v)
	case Ready:
		if v.Err != nil {
			m.Err = v.Err
			return m, tea.Quit
		}
		n := NewReady(v.Config)
		n.width, n.height = m.width, m.height
		n.contacts.repo = v.Repo
		n.mcp.state = v.MCP
		n.extensions.mgr = v.Extensions
		n.contacts.ctx = v.Context
		if n.contacts.ctx == nil {
			n.contacts.ctx = context.Background()
		}
		n.contacts.order = "name"
		if crm, ok := v.Repo.(app.CRM); ok {
			n.crm.repo = crm
			n.crm.lists[wsTasks].view = 1 // Tasks opens on Today
		}
		cmd := tea.Batch(n.refreshContacts(), n.loadCRM(), n.loadDue(), n.refreshDirectories(nil))
		if v.Config.Mouse {
			return n, tea.Batch(cmd, tea.EnableMouseCellMotion)
		}
		return n, cmd
	case tea.WindowSizeMsg:
		wasNarrow := m.width < extNarrowFrom
		m.width, m.height = max(0, v.Width), max(0, v.Height)
		if m.mode() == tideui.SidebarOnly && m.focus == 2 {
			m.focus = 1
		}
		m.search.Width = max(1, m.width-10)
		m.prompt.Width = max(1, min(60, m.width-12))
		if v := m.extensions.view; v != nil && wasNarrow != (m.width < extNarrowFrom) {
			return m, m.extRender(v)
		}
		return m, nil
	case tea.KeyMsg:
		key := v.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if key == "ctrl+z" {
			return m, tea.Suspend
		}
		if !m.ready {
			return m, nil
		}
		if m.overlay == palette {
			switch key {
			case "esc":
				m.overlay = none
				m.prompt.Blur()
				return m, nil
			case "down", "ctrl+n":
				m.paletteSelected = min(m.paletteSelected+1, max(0, len(m.paletteMatches())-1))
				return m, nil
			case "up", "ctrl+p":
				m.paletteSelected = max(0, m.paletteSelected-1)
				return m, nil
			case "enter":
				matches := m.paletteMatches()
				if len(matches) > 0 {
					m.overlay = none
					m.prompt.Blur()
					return m.dispatch(matches[m.paletteSelected].ID)
				}
				return m, nil
			}
			old := m.prompt.Value()
			var cmd tea.Cmd
			m.prompt, cmd = m.prompt.Update(v)
			if m.prompt.Value() != old {
				m.paletteSelected = 0
			}
			return m, cmd
		}
		if m.overlay == help {
			var cmd tea.Cmd
			var close bool
			m.helpBrowser.Width, m.helpBrowser.Height = m.width, m.height-1
			m.helpBrowser, cmd, close = m.helpBrowser.Update(v)
			if close {
				m.overlay = none
			}
			return m, cmd
		}

		if m.editing {
			switch key {
			case "esc":
				m.search.SetValue("")
				m.queries[m.state.Active] = ""
				m.editing = false
				m.search.Blur()
				if m.isContacts() {
					return m, m.refreshContacts()
				}
				return m, m.loadCRM()
			case "enter", "tab":
				m.editing = false
				m.search.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(v)
			m.queries[m.state.Active] = m.search.Value()
			if m.isContacts() {
				return m, tea.Batch(cmd, m.debounceSearch())
			}
			if m.isCRM() {
				return m, tea.Batch(cmd, m.debounceCRM())
			}
			return m, cmd
		}
		if v.Paste {
			return m, nil
		}
		id := m.registry.Key(key)
		if m.isContacts() || m.isCRM() {
			if id == "previous" && key != "shift+tab" && m.focus == 0 {
				m.menuSelected = m.state.Active
				m.menuOpen = true
				return m, nil
			}
			last := m.focusCount() - 1
			if m.mode() == tideui.ThreeColumn && m.state.Active != wsPipelines {
				last = 1
			}
			if id == "next" && key != "tab" && m.focus >= last && !(m.state.Active == wsPipelines && m.focus == 1) {
				return m, nil
			}
		}
		if m.state.Active == wsPipelines && ((key == "tab" && id == "next") || (key == "shift+tab" && id == "previous")) {
			step := 1
			if id == "previous" {
				step = -1
			}
			m.focus = (m.focus + step + m.focusCount()) % m.focusCount()
			return m, nil
		}
		return m.dispatch(id)
	case tea.MouseMsg:
		if m.cfg.Mouse && m.overlay == none && !m.editing && v.Action == tea.MouseActionPress && v.Button == tea.MouseButtonLeft {
			if v.Y < 1 || v.Y >= m.height-1 {
				return m, nil
			}
			left, right := m.ratios()
			if m.mode() == tideui.ThreeColumn {
				if v.X < int(float64(m.width)*left) {
					if !m.menuOpen {
						m.menuSelected = m.state.Active
					}
					m.menuOpen = true
				} else if v.X >= int(float64(m.width)*(left+right)) {
					m.focus = 1
					m.menuOpen = false
				} else {
					m.focus = 0
					m.menuOpen = false
				}
			} else if m.mode() == tideui.SidebarOnly {
				if v.X < int(float64(m.width)*left) {
					m.focus = 0
				} else {
					m.focus = 1
				}
			}
		}
		return m, nil
	}
	var cmd tea.Cmd
	if m.editing {
		m.search, cmd = m.search.Update(msg)
	} else if m.overlay == palette {
		m.prompt, cmd = m.prompt.Update(msg)
	}
	return m, cmd
}
func (m Model) dispatch(id string) (tea.Model, tea.Cmd) {
	if id == "mcp-manage" {
		return m.openMCP()
	}
	if id == "extensions-manage" || strings.HasPrefix(id, "ext:") || strings.HasPrefix(id, "ext-ws:") {
		return m.extDispatch(id)
	}
	if id == "refresh-address-books" {
		return m, m.refreshDirectories(nil)
	}
	if id == "import-contacts" && m.contacts.repo != nil {
		return m.startImportModal()
	}
	if id == "" {
		return m, nil
	}
	m.feedback = ""
	if next, cmd, handled := m.crmGlobal(id); handled {
		return next, cmd
	}
	if id == "back" && m.isContacts() && len(m.crm.nav) > 0 && m.contacts.confirm == "" {
		next, cmd, _ := m.back()
		return next, cmd
	}
	if next, cmd, handled := m.dossierAction(id); handled {
		return next, cmd
	}
	if next, cmd, handled := m.contactAction(id); handled {
		if next.dossierVisible() && (strings.HasPrefix(id, "profile-") || next.crm.dossierFor != next.contacts.contact.ID) {
			return next, tea.Batch(cmd, next.loadDossier())
		}
		return next, cmd
	}
	if next, cmd, handled := m.crmAction(id); handled {
		return next, cmd
	}
	if strings.HasPrefix(id, "workspace-") {
		m.closeExtView()
		m.crm.nav, m.crm.itemOpen, m.crm.err = nil, false, ""
		m.contacts.listSeq++
		m.contacts.detailSeq++
		if m.contacts.cancelList != nil {
			m.contacts.cancelList()
		}
		if m.contacts.cancelDetail != nil {
			m.contacts.cancelDetail()
		}
		m.state.Navigate(strings.TrimPrefix(id, "workspace-"))
		m.selected = m.state.Active
		m.focus = 1
		m.search.SetValue(m.queries[m.state.Active])
		if m.isContacts() {
			m.focus = 0
			return m, m.refreshContacts()
		}
		if m.isCRM() {
			m.focus = 0
			if m.state.Active == wsPipelines {
				m.focus = 1
			}
			return m, m.loadCRM()
		}
		return m, nil
	}
	switch id {
	case "quit":
		return m, tea.Quit
	case "down":
		if m.focus == 0 {
			m.selected = min(7, m.selected+1)
		}
	case "up":
		if m.focus == 0 {
			m.selected = max(0, m.selected-1)
		}
	case "next":
		m.focus = (m.focus + 1) % m.focusCount()
	case "previous":
		m.focus = (m.focus + m.focusCount() - 1) % m.focusCount()
	case "activate":
		if m.focus == 0 {
			return m.dispatch("workspace-" + domain.Workspaces()[m.selected].ID)
		}
	case "back":
		m.focus = 0
	case "search":
		m.focus = 1
		m.editing = true
		return m, m.search.Focus()
	case "palette":
		m.overlay = palette
		m.prompt.SetValue("")
		m.paletteSelected = 0
		return m, m.prompt.Focus()
	case "help":
		m.helpBrowser = tideui.NewHelpBrowser(m.helpSections())
		m.overlay = help
		m.helpOffset = 0
	case "context":
		m.helpBrowser = tideui.NewHelpBrowser(m.helpSections())
		m.overlay = help

	case "left-grow":
		m.resizeFocused(true)
		m.feedback = "Layout changed; saved on exit"
	case "left-shrink":
		m.resizeFocused(false)
		m.feedback = "Layout changed; saved on exit"
	case "right-grow":
		m.right.Grow()
		m.feedback = "Layout changed; saved on exit"
	case "right-shrink":
		m.right.Shrink()
		m.feedback = "Layout changed; saved on exit"
	case "themes":
		options := []domain.Ref{}
		if m.cfg.TideMailThemeName != "" {
			options = append(options, domain.Ref{ID: "match-tidemail", Name: "Match TideMail · " + m.cfg.TideMailThemeName})
		}
		for _, t := range tideui.BuiltinThemes {
			options = append(options, domain.Ref{ID: t.Name, Name: t.Name})
		}
		options = append(options, domain.Ref{ID: "contrast", Name: "High contrast"})
		original := m.cfg.Theme
		cmd := m.openPicker("Themes", "↑↓ live preview · enter keep · esc restore", "", options, func(m *Model, r domain.Ref) tea.Cmd { m.cfg.Theme = r.ID; m.feedback = "Theme: " + r.Name; return nil })
		p := m.crm.picker
		p.originalTheme = original
		current := original
		if original == "dark" {
			current = "catppuccin-mocha"
		}
		if original == "light" {
			current = "catppuccin-latte"
		}
		for i, o := range p.options {
			if o.ID == current {
				p.selected = i
				break
			}
		}
		p.preview = func(m *Model, r domain.Ref) { m.cfg.Theme = r.ID }
		p.cancel = func(m *Model) { m.cfg.Theme = original }
		return m, cmd
	case "theme-dark", "theme-light", "theme-contrast":
		m.cfg.Theme = strings.TrimPrefix(id, "theme-")
		m.feedback = "Theme: " + m.cfg.Theme
	}
	return m, nil
}
func (m Model) hint(id string) string {
	for _, c := range m.registry.Commands() {
		if c.ID == id && len(c.Keys) > 0 {
			return c.Keys[0]
		}
	}
	return ": commands"
}
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if !m.ready {
		return ansi.Truncate("TideContacts | Loading local workspace...", m.width, "")
	}
	r := tideui.NewRenderer(m.theme(), tideui.StyleOptions{Density: tideui.Compact})
	w := domain.Workspaces()[m.state.Active]
	workspace := w.Name
	if v := m.extensions.view; v != nil {
		workspace = v.title
	}
	header := r.Styles.DetailTitle.Render(ansi.Truncate("TideContacts / "+workspace+"  |  local", m.width, ""))
	header = ansi.Truncate(header, m.width, "")
	if m.height == 1 {
		return header
	}
	headerLines := 1
	navWidth := max(1, int(float64(m.width)*m.left.Value())-4)
	if m.mode() == tideui.Tabbed {
		navWidth = max(1, m.width-4)
	}
	rows := []string{}
	for i, ws := range domain.Workspaces() {
		prefix := "  "
		if i == m.selected {
			prefix = "> "
		}
		suffix := ""
		if i == m.state.Active {
			suffix = "*"
		}
		rows = append(rows, r.RenderRow(tideui.Row{Prefix: prefix, Text: fmt.Sprintf("%d %s", i+1, ws.Name), Suffix: suffix, Selected: i == m.selected && m.focus == 0}, navWidth))
	}
	contentWidth := max(1, m.width-4)
	if m.mode() == tideui.SidebarOnly {
		contentWidth = max(1, m.width-int(float64(m.width)*m.left.Value())-4)
	}
	if m.mode() == tideui.ThreeColumn {
		contentWidth = max(1, int(float64(m.width)*(1-m.left.Value()-m.right.Value()))-4)
	}
	body := "No " + strings.ToLower(w.Name) + " yet.\n\n" + w.Description + "\n\nLocal storage is not connected in this session."
	if m.queries[m.state.Active] != "" {
		body = "No results for " + fmt.Sprintf("%q", m.queries[m.state.Active]) + ".\n\nThere are no records to search yet.\nEsc while editing clears the query."
	}
	search := m.search
	search.Width = max(1, contentWidth-3)
	search.TextStyle = r.Styles.DetailBody
	search.PromptStyle = r.Styles.DetailTitle
	if m.editing {
		body = search.View() + "\n\n" + body
	} else {
		body = "Search: " + m.queries[m.state.Active] + "\n\n" + body
	}
	body = r.Styles.DetailBody.Width(contentWidth).Render(body)
	contextText := w.Name + "\n\n" + w.Description + "\n\n" + m.hint("help") + " contextual help\n" + m.hint("back") + " back to navigation"
	titles := [3]string{"Workspaces", w.Name, "Context"}

	panes := [3]tideui.Pane{{Title: titles[0], Content: strings.Join(rows, "\n"), Focused: m.focus == 0, ScrollOffset: max(0, m.selected-max(1, m.height-7)+1)}, {Title: titles[1], Content: body, Focused: m.focus == 1}, {Title: titles[2], Content: contextText, Focused: m.focus == 2}}
	status := "NORMAL / " + []string{"navigation", "workspace", "context"}[m.focus]
	hints := m.hint("search") + " search  " + m.hint("palette") + " commands  " + m.hint("help") + " help"
	if m.feedback != "" {
		status = m.feedback
	}
	if m.mode() == tideui.Tabbed {
		hints = m.hint("back") + " back  " + m.hint("next") + " pane  " + m.hint("palette") + " commands"
	}
	if m.editing {
		status = "SEARCH / " + w.Name
		hints = "enter finish  esc clear"
	}
	if m.overlay == palette {
		status = "COMMANDS"
		hints = "up/down select  enter run  esc close"
	}
	if m.overlay == help {
		status = "HELP / " + w.Name
		hints = "j/k scroll  esc close"
	}
	if m.isContacts() {
		panes = m.contactPanes(r)
		status = "CONTACTS / " + profileViews[m.contacts.view]
		hints = "← menu  " + m.hint("search") + " search  " + m.hint("activate") + " open  " + m.hint("edit-contact") + " edit"
		if m.focus != 0 {
			hints = m.hint("back") + " back  " + m.hint("edit-contact") + " edit  " + m.hint("search") + " search"
		}
		if m.editing {
			status = "SEARCH"
			hints = "enter finish  esc clear"
		}
		if m.contacts.err != "" {
			status = "! " + clip(domain.SafeText(m.contacts.err), max(1, m.width-4))
			hints = ""
		}
		if m.contacts.busy {
			status = "Saving..."
		}
	}
	if m.isCRM() {
		panes = m.crmPanes(r)
		status, hints = m.crmStatus()
		if m.editing {
			status, hints = "FILTER", "enter finish  esc clear"
		}
		if m.crm.err != "" {
			status, hints = "! "+clip(domain.SafeText(m.crm.err), max(1, m.width-4)), ""
		}
		if m.crm.busy {
			status = "Saving..."
		}
	}
	if m.feedback != "" && (m.isCRM() && m.crm.err == "" || m.isContacts() && m.contacts.err == "") && !m.editing {
		status = clip(domain.SafeText(m.feedback), max(1, m.width-4))
	}
	if m.crm.due > 0 && m.crm.err == "" {
		hints = fmt.Sprintf("%d reminder(s) due  ", m.crm.due) + hints
	}
	if m.contacts.editor != nil || m.crm.form != nil {
		status, hints = "EDITING", ""
	}
	if m.crm.prompt != nil || m.crm.picker != nil || m.crm.confirm != nil {
		status, hints = "INPUT", "enter accept  esc cancel"
	}
	if m.overlay == palette {
		status = "COMMANDS"
		hints = "up/down select  enter run  esc close"
	}
	if m.overlay == help {
		status = "HELP"
		hints = "/ search  arrows choose/scroll  esc close"
	}
	if m.mode() == tideui.ThreeColumn && (m.isContacts() || m.isCRM()) {
		detail := panes[1]
		if m.focus == 2 {
			detail = panes[2]
		}
		panes = [3]tideui.Pane{m.menuPane(), panes[0], detail}
		if m.menuOpen {
			panes[1].Focused = false
			panes[2].Focused = false
		}
	}
	if m.menuOpen {
		hints = "→ records  ↑↓ choose  enter open  esc close"
	} else if m.overlay == none && !m.editing && m.contacts.editor == nil && m.crm.form == nil && m.crm.picker == nil && m.crm.prompt == nil && m.contacts.transfer == nil {
		hints += "  ctrl+g Menu"
	}
	if m.mcp.modal != nil {
		status, hints = "MCP / PROFILES", ""
	}
	for ansi.StringWidth(status)+ansi.StringWidth(hints)+4 > m.width {
		i := strings.LastIndex(hints, "  ")
		if i < 0 {
			break
		}
		hints = hints[:i]
	}
	leftRatio, rightRatio := m.ratios()
	if m.cfg.Demo {
		status = "DEMO · " + status
	}
	layout := tideui.Layout{Width: m.width, Height: m.height - headerLines, Mode: m.mode(), Panes: panes, SidebarRatio: leftRatio, ColumnRatios: [3]float64{leftRatio, rightRatio, 1 - leftRatio - rightRatio}, Status: &tideui.StatusBar{Left: status, Right: hints}}
	if m.menuOpen && m.mode() != tideui.ThreeColumn {
		p := m.menuPane()
		panel := r.SoftPanelOverlay(tideui.SoftPanel{Title: "Menu · enter select · esc close", Width: max(1, min(54, m.width-4)), Content: p.Content})
		layout.Modal = &panel
	} else if m.overlay == help {
		panel := m.helpBrowser.Overlay(r, m.width, m.height-headerLines)
		layout.Modal = &panel
	} else if m.overlay != none {
		width := max(1, min(68, m.width-4))
		inner := max(1, width-4)
		lines := []string{}
		title := "Commands"
		if m.overlay == palette {
			prompt := m.prompt
			prompt.Width = max(1, inner-3)
			prompt.TextStyle = r.Styles.DetailBody
			lines = append(lines, prompt.View(), "")
			matches := m.paletteMatches()
			visible := max(1, m.height-10)
			start := max(0, m.paletteSelected-visible+1)
			for i := start; i < min(len(matches), start+visible); i++ {
				c := matches[i]
				prefix := "  "
				if i == m.paletteSelected {
					prefix = "> "
				}
				lines = append(lines, r.RenderSoftRow(tideui.SoftRow{Prefix: prefix, Text: c.Label, Suffix: strings.Join(c.Keys, "/"), Selected: i == m.paletteSelected}, inner))
			}
			if len(matches) == 0 {
				lines = append(lines, "No matching commands")
			}
		} else {
			title = "Help / " + w.Name
			lines = append(lines, w.Description, "n new · e edit · / filter · [ ] views · Enter open or follow · Esc back.", "T task and A activity capture work everywhere and link to the selection.", "Text input owns all printable keys. Ctrl+C exits; Esc closes this help.", "")
			if m.isCRM() {
				lines = append(lines, crmHints[m.state.Active]...)
				lines = append(lines, "")
			}
			for _, c := range m.registry.Commands() {
				lines = append(lines, fmt.Sprintf("%-18s %s", strings.Join(c.Keys, " / "), c.Label))
			}
			m.helpOffset = min(m.helpOffset, max(0, len(lines)-1))
			lines = lines[m.helpOffset:]
			lines = lines[:min(len(lines), max(1, m.height-8))]
		}
		overlay := r.SoftPanelOverlay(tideui.SoftPanel{Prefix: "tidecontacts", Title: title, Width: width, Content: strings.Join(lines, "\n")})
		layout.Modal = &overlay
	} else if panel := m.overlayPanel(r); panel != nil {
		layout.Modal = panel
	}
	if m.contacts.transfer != nil {
		layout.Modal = m.transferPanel(r)
	}
	if m.mcp.modal != nil {
		layout.Modal = m.mcpPanel(r)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, r.Render(layout))
}

// resizeFocused widens or narrows the pane you are in. In three columns the
// menu owns the left ratio, the list owns the right ratio, and the detail pane
// takes whatever is left; its edge moves with the arrow key.
func (m *Model) resizeFocused(grow bool) {
	switch {
	case m.mode() != tideui.ThreeColumn:
		if grow {
			m.left.Grow()
		} else {
			m.left.Shrink()
		}
	case m.menuOpen:
		if grow {
			m.left.Grow()
		} else {
			m.left.Shrink()
		}
	case m.focus == 0:
		if grow {
			m.right.Grow()
		} else {
			m.right.Shrink()
		}
	default:
		// The detail pane's left edge is the list's right edge, so the arrow
		// moves that edge: right narrows the detail, left widens it.
		if grow {
			m.right.Grow()
		} else {
			m.right.Shrink()
		}
	}
}
