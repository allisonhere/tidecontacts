package tui

import (
	"strings"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/ext"
	"github.com/allisonhere/tideui"
	tea "github.com/charmbracelet/bubbletea"
)

type settingsState struct {
	section, row int
	open, detail bool
}

type settingRow struct{ id, label, help string }

type settingsSection struct {
	title, description string
	rows               []settingRow
}

var settingsSections = []settingsSection{
	{"Display", "Appearance and terminal input", []settingRow{
		{"themes", "Theme", "Preview a theme before keeping it."},
		{"density", "Contact list density", "Choose compact rows or more breathing room."},
		{"mouse", "Mouse support", "Captures clicks; turn off for native terminal text selection."},
	}},
	{"General", "Startup and relationship preferences", []settingRow{
		{"startup", "Startup workspace", "Takes effect the next time TideContacts opens."},
		{"radar-settings", "Relationship Radar", "Choose signals and their time windows."},
	}},
	{"Connections", "Address books and TideMail", []settingRow{
		{"import-contacts", "Address books & import", "Manage connected books or import local records."},
		{"tidemail-status", "TideMail sync status", "Check when the local mail cache was last imported."},
		{"tidemail-sync", "Sync TideMail", "Import activity metadata from the local TideMail cache."},
	}},
	{"Extensions", "Optional features and permissions", []settingRow{
		{"extensions-manage", "Manage extensions", "Inspect grants and enable or disable installed extensions."},
	}},
	{"Agents (MCP)", "Local integrations with agent clients", []settingRow{
		{"mcp-manage", "Manage MCP profiles", "Review tool grants, setup and access history."},
	}},
	{"About", "TideContacts is a local-first terminal CRM.", nil},
}

func (m *Model) toggleDensity() {
	if m.cfg.Density == "comfortable" {
		m.cfg.Density = "compact"
	} else {
		m.cfg.Density = "comfortable"
	}
	m.prefs.Density = m.cfg.Density
}

func (m Model) settingsHasOverlay() bool {
	c := m.crm
	return c.form != nil || c.picker != nil || c.prompt != nil || c.confirm != nil ||
		m.extensions.modal != nil || m.mcp.modal != nil || m.contacts.transfer != nil
}

func (m Model) settingsKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Paste {
		return m, nil
	}
	s := &m.settings
	rows := settingsSections[s.section].rows
	limit, at := len(settingsSections)-1, s.section
	if s.detail {
		limit, at = max(0, len(rows)-1), s.row
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "ctrl+z":
		return m, tea.Suspend
	case "ctrl+g":
		s.open = false
		m.menuOpen, m.menuSelected = true, m.state.Active
	case "esc":
		if s.detail {
			s.detail = false
		} else {
			s.open = false
		}
	case "tab", "shift+tab":
		s.detail = !s.detail
	case "left", "h":
		s.detail = false
	case "right", "l":
		s.detail = true
	case "enter", " ":
		if !s.detail {
			s.detail = true
		} else if len(rows) > 0 {
			return m.activateSetting(rows[s.row].id)
		}
	case "up", "k", "ctrl+p":
		m.selectSetting(max(0, at-1))
	case "down", "j", "ctrl+n":
		m.selectSetting(min(limit, at+1))
	case "home", "pgup":
		m.selectSetting(0)
	case "end", "pgdown":
		m.selectSetting(limit)
	}
	return m, nil
}

func (m *Model) selectSetting(index int) {
	s := &m.settings
	if s.detail {
		s.row = index
	} else if s.section != index {
		s.section, s.row = index, 0
		m.feedback = ""
	}
}

func (m Model) activateSetting(id string) (tea.Model, tea.Cmd) {
	_, disabled := m.settingValue(id)
	if disabled {
		return m, nil
	}
	m.feedback = ""
	switch id {
	case "density":
		m.toggleDensity()
		m.feedback = "Density changed; saved on exit"
		return m, nil
	case "mouse":
		mouse := !m.cfg.Mouse
		m.cfg.Mouse, m.prefs.Mouse = mouse, &mouse
		m.feedback = "Mouse support changed; saved on exit"
		if mouse {
			return m, tea.EnableMouseCellMotion
		}
		return m, tea.DisableMouse
	case "startup":
		workspaces := domain.Workspaces()
		options := make([]domain.Ref, 0, len(workspaces))
		for _, w := range workspaces {
			options = append(options, domain.Ref{ID: w.ID, Name: w.Name})
		}
		cmd := m.openPicker("Startup workspace", "Choose where to start next time", "", options, func(m *Model, chosen domain.Ref) tea.Cmd {
			m.cfg.DefaultWorkspace = chosen.ID
			m.prefs.DefaultWorkspace = chosen.ID
			return nil
		})
		m.crm.picker.selected = max(0, domain.WorkspaceIndex(m.cfg.DefaultWorkspace))
		return m, cmd
	case "radar-settings", "tidemail-status", "tidemail-sync":
		m.crm.err = ""
	}
	return m.dispatch(id)
}

func (m Model) settingValue(id string) (value string, disabled bool) {
	switch id {
	case "themes":
		return m.cfg.Theme, false
	case "density":
		if m.cfg.Density == "comfortable" {
			return "Comfortable", false
		}
		return "Compact", false
	case "mouse":
		if m.cfg.Mouse {
			return "On", false
		}
		return "Off", false
	case "startup":
		return domain.Workspaces()[max(0, domain.WorkspaceIndex(m.cfg.DefaultWorkspace))].Name, false
	case "extensions-manage":
		disabled = m.extensions.mgr == nil
	case "mcp-manage":
		disabled = m.mcp.state == nil || m.crm.repo == nil
	case "import-contacts":
		disabled = m.contacts.repo == nil
	case "radar-settings":
		disabled = m.crm.repo == nil
	case "tidemail-status", "tidemail-sync":
		if m.cfg.Demo {
			return "Disabled in demo", true
		}
		disabled = m.crm.repo == nil
	}
	if disabled {
		return "Unavailable", true
	}
	return "", false
}

func settingsSidebarWidth(width int) int {
	if width < 52 {
		return 0
	}
	return min(18, max(15, width/4))
}

func settingsRowHeight(height int) int {
	if height < 9 {
		return 1
	}
	return 2
}

func (m Model) settingsMouse(msg tea.MouseMsg) Model {
	if !m.cfg.Mouse || msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft || msg.Y < 3 || msg.Y >= m.height-1 {
		return m
	}
	left, height := settingsSidebarWidth(m.width), m.height-2
	if left == 0 || msg.X != left {
		if left > 0 {
			m.settings.detail = msg.X > left
		}
		if m.settings.detail {
			rows, stride := settingsSections[m.settings.section].rows, settingsRowHeight(height)
			start, end := tideui.VisibleRange(len(rows), m.settings.row, max(1, (height-2)/stride))
			if index := start + (msg.Y-3)/stride; index < end {
				m.selectSetting(index)
			}
		} else {
			start, end := tideui.VisibleRange(len(settingsSections), m.settings.section, max(1, height-2))
			if index := start + msg.Y - 3; index < end {
				m.selectSetting(index)
			}
		}
	}
	return m
}

func (m Model) settingsView(r tideui.Renderer) string {
	w, h := m.width, m.height
	u := newSoftUI(r)
	bg := r.Styles.Theme.Bg
	accent := tideui.ReadableText(u.accent, bg, 4.5)
	header := " TideContacts / Settings"
	if m.cfg.Demo {
		header += "  |  demo"
	}
	lines := make([]string, 0, h)
	lines = append(lines, u.cell(u.style(bg, accent).Bold(true).Render(header), w, bg))
	if h == 1 {
		return lines[0]
	}
	left, bodyH := settingsSidebarWidth(w), h-2
	if bodyH > 0 {
		switch {
		case left > 0:
			sidebar := m.settingsSidebar(r, u, left, bodyH)
			detail := m.settingsDetail(r, u, w-left-1, bodyH)
			divider := "│"
			if r.Styles.PlainUI {
				divider = "|"
			}
			rule := u.style(bg, r.Styles.Theme.Border).Render(divider)
			for i := range sidebar {
				lines = append(lines, sidebar[i]+rule+detail[i])
			}
		case m.settings.detail:
			lines = append(lines, m.settingsDetail(r, u, w, bodyH)...)
		default:
			lines = append(lines, m.settingsSidebar(r, u, w, bodyH)...)
		}
	}
	footer := " ↑↓ move   Tab switch pane   Enter open   Esc close"
	if m.settings.detail {
		footer = " ↑↓ move   Tab switch pane   Enter change   Esc back"
	}
	if m.feedback != "" {
		footer = " " + domain.SafeText(m.feedback) + "   ·   Esc back"
	} else if m.settings.detail && m.crm.err != "" {
		section := settingsSections[m.settings.section]
		if len(section.rows) > 0 {
			switch section.rows[m.settings.row].id {
			case "radar-settings", "tidemail-status", "tidemail-sync":
				footer = " " + domain.SafeText(m.crm.err) + "   ·   Esc back"
			}
		}
	}
	lines = append(lines, u.cell(r.Styles.StatusBar.UnsetPadding().Render(footer), w, r.Styles.Theme.StatusBar))
	view := strings.Join(lines, "\n")
	if p := m.settingsOverlay(r); p != nil {
		return r.OverlayModal(view, p.Content, w, h)
	}
	return view
}

func (m Model) settingsOverlay(r tideui.Renderer) *tideui.Overlay {
	switch {
	case m.mcp.modal != nil:
		return m.mcpPanel(r)
	case m.contacts.transfer != nil:
		return m.transferPanel(r)
	case m.settingsHasOverlay():
		return m.overlayPanel(r)
	}
	return nil
}

func settingsPane(u softUI, title string, focused bool, width, height int) []string {
	bg := u.r.Styles.Theme.Bg
	lines := make([]string, height)
	blank := u.cell("", width, bg)
	for i := range lines {
		lines[i] = blank
	}
	lines[0] = u.cell(u.r.SoftRail(focused, bg)+u.style(bg, tideui.ReadableText(u.accent, bg, 4.5)).Bold(true).Render(title), width, bg)
	return lines
}

func (m Model) settingsSidebar(r tideui.Renderer, u softUI, width, height int) []string {
	bg := r.Styles.Theme.Bg
	lines := settingsPane(u, "Categories", !m.settings.detail, width, height)
	if height <= 2 {
		return lines
	}
	start, end := tideui.VisibleRange(len(settingsSections), m.settings.section, height-2)
	for i := start; i < end; i++ {
		active := i == m.settings.section
		row := tideui.SoftRow{Text: settingsSections[i].title, Selected: active && !m.settings.detail, Accent: active}
		if active && m.settings.detail {
			row.Prefix = "* "
		}
		lines[i-start+2] = u.cell(r.RenderSoftRowOn(row, width, bg), width, bg)
	}
	return lines
}

func (m Model) settingsDetail(r tideui.Renderer, u softUI, width, height int) []string {
	section := settingsSections[m.settings.section]
	bg := r.Styles.Theme.Bg
	muted := tideui.MutedText(r.Styles.Theme.Fg, bg)
	lines := settingsPane(u, section.title, m.settings.detail, width, height)
	if height > 1 {
		lines[1] = u.cell(u.style(bg, muted).Render("  "+section.description), width, bg)
	}
	if height <= 2 {
		return lines
	}
	if len(section.rows) == 0 {
		lines[2] = u.cell(u.style(bg, muted).Render("  Extension host "+ext.HostVersion), width, bg)
		return lines
	}
	stride := settingsRowHeight(height)
	start, end := tideui.VisibleRange(len(section.rows), m.settings.row, (height-2)/stride)
	for i := start; i < end; i++ {
		setting := section.rows[i]
		value, disabled := m.settingValue(setting.id)
		if value == "" {
			value = ">"
		}
		value = clip(value, max(1, width/3))
		y := 2 + (i-start)*stride
		row := tideui.SoftRow{Text: setting.label, Suffix: "  " + value + " ", Selected: m.settings.detail && i == m.settings.row, Muted: disabled}
		lines[y] = u.cell(r.RenderSoftRowOn(row, width, bg), width, bg)
		if stride == 2 {
			lines[y+1] = u.cell(u.style(bg, muted).Render("    "+setting.help), width, bg)
		}
	}
	return lines
}
