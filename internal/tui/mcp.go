package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/mcpserver"
	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type mcpMode uint8

const (
	mcpList mcpMode = iota
	mcpEdit
	mcpConfirm
	mcpTools
	mcpAudit
)

type mcpSettings struct {
	state app.MCPState
	modal *mcpModal
}

type mcpModal struct {
	mode                    mcpMode
	busy, saving            bool
	profiles                []app.MCPProfile
	selected, field, scroll int
	profile                 app.MCPProfile
	name                    textinput.Model
	scopes                  []string
	lines                   []string
	message                 string
	failed                  bool
}

type mcpLoaded struct {
	modal    *mcpModal
	profiles []app.MCPProfile
	err      error
}

type mcpSaved struct {
	modal   *mcpModal
	profile app.MCPProfile
	err     error
}

type mcpInspected struct {
	modal   *mcpModal
	lines   []string
	problem string
}

func (m Model) openMCP() (tea.Model, tea.Cmd) {
	m.menuOpen = false
	if m.mcp.state == nil || m.crm.repo == nil {
		m.feedback = "MCP profiles are unavailable without local CRM storage"
		return m, nil
	}
	m.mcp.modal = &mcpModal{}
	return m, m.loadMCPProfiles()
}

func (m Model) loadMCPProfiles() tea.Cmd {
	modal, state, ctx := m.mcp.modal, m.mcp.state, m.contacts.ctx
	modal.busy = true
	return func() tea.Msg {
		profiles, err := state.MCPProfiles(ctx)
		return mcpLoaded{modal, profiles, err}
	}
}

func (m Model) saveMCPProfile() tea.Cmd {
	modal, state, ctx := m.mcp.modal, m.mcp.state, m.contacts.ctx
	profile := modal.profile
	profile.Scopes = slices.Clone(profile.Scopes)
	modal.busy, modal.saving = true, true
	return func() tea.Msg {
		profile, err := state.SaveMCPProfile(ctx, profile)
		return mcpSaved{modal, profile, err}
	}
}

func (m Model) inspectMCP(profile app.MCPProfile) tea.Cmd {
	modal, repo, state, ctx, mgr := m.mcp.modal, m.crm.repo, m.mcp.state, m.contacts.ctx, m.extensions.mgr
	modal.mode, modal.scroll = mcpTools, 0
	if !profile.Enabled {
		modal.lines = []string{mcpText(profile.Name) + " · disabled", "This profile cannot start an MCP session.", "Esc, then Enter to edit the profile."}
		return nil
	}
	modal.busy = true
	prefix := "tidecontacts "
	serveArgs := "mcp serve " + profile.Name
	if m.cfg.Demo {
		prefix += "--demo "
		serveArgs = "--demo " + serveArgs
	}
	return func() tea.Msg {
		server, err := mcpserver.New(ctx, repo, state, profile.Name, mgr)
		if err != nil {
			return mcpInspected{modal: modal, problem: "Could not open this profile. Reload profiles and check that it is enabled."}
		}
		info, err := server.Diagnose(ctx)
		if err != nil {
			return mcpInspected{modal: modal, problem: "In-memory MCP tool discovery failed."}
		}
		lines := []string{
			fmt.Sprintf("%s · enabled · revision %d", info.Profile.Name, info.Profile.Revision),
			fmt.Sprintf("%s discovery passed · %d tools · none invoked", info.Transport, len(info.Tools)),
			"Use a local stdio client you trust. TideContacts starts nothing here.",
			"Generate settings for codex, claude, or hermes:",
			prefix + "mcp config " + info.Profile.Name + " CLIENT",
			"Client executable: tidecontacts (use its absolute path)",
			"Client arguments: " + serveArgs,
			"",
			"Discovered tools",
		}
		for _, tool := range info.Tools {
			scope := tool.Scope
			if scope == "" {
				scope = "metadata only"
			} else if tool.Write {
				scope = "WRITE · " + scope
			}
			lines = append(lines, tool.Name+" · "+scope)
		}
		return mcpInspected{modal: modal, lines: lines}
	}
}

func (m Model) loadMCPAudit() tea.Cmd {
	modal, state, ctx := m.mcp.modal, m.mcp.state, m.contacts.ctx
	modal.mode, modal.busy, modal.scroll = mcpAudit, true, 0
	return func() tea.Msg {
		audit, err := state.MCPAuditPage(ctx, 0, 20)
		if err != nil {
			return mcpInspected{modal: modal, problem: "Could not load recent MCP activity."}
		}
		lines := []string{"Latest 20 workspace events · no arguments or results"}
		if len(audit) == 0 {
			lines = append(lines, "No MCP activity recorded.")
		}
		for _, entry := range audit {
			access := "read"
			if entry.Write {
				access = "write"
			}
			lines = append(lines,
				fmt.Sprintf("%s · %s · %s", access, mcpText(entry.Outcome), mcpText(entry.Tool)),
				fmt.Sprintf("  %s · %s", entry.At.UTC().Format("Jan 02 15:04Z"), mcpText(entry.Profile)),
			)
		}
		return mcpInspected{modal: modal, lines: lines}
	}
}

func mcpText(s string) string { return strings.Join(strings.Fields(domain.SafeText(s)), " ") }

func (modal *mcpModal) edit(p app.MCPProfile) tea.Cmd {
	p.Scopes = slices.Clone(p.Scopes)
	modal.profile, modal.scopes = p, mcpserver.Scopes()
	modal.mode, modal.field = mcpEdit, 1
	modal.name = textinput.New()
	modal.name.Prompt = ""
	modal.name.CharLimit = 32
	modal.name.Placeholder = "my-client"
	if p.Revision == 0 {
		modal.field = 0
		return modal.name.Focus()
	}
	return nil
}

func (m Model) updateMCP(msg tea.Msg) (Model, tea.Cmd, bool) {
	modal := m.mcp.modal
	switch v := msg.(type) {
	case mcpLoaded:
		if modal != nil && modal == v.modal {
			modal.busy = false
			if v.err != nil {
				modal.profiles, modal.selected = nil, 0
				modal.message, modal.failed = "Could not load profiles. Press r to reload.", true
			} else {
				modal.profiles = v.profiles
				modal.selected = min(modal.selected, max(0, len(v.profiles)-1))
				for i, p := range modal.profiles {
					if p.Name == modal.profile.Name {
						modal.selected = i
						break
					}
				}
			}
		}
		return m, nil, true
	case mcpSaved:
		if modal != nil && modal == v.modal {
			modal.busy, modal.saving = false, false
			modal.mode = mcpList
			if v.err != nil {
				modal.message, modal.failed = "Not saved. The revision may have changed or storage rejected the update.", true
			} else {
				modal.profile = v.profile
				modal.message = fmt.Sprintf("Saved %s · revision %d", v.profile.Name, v.profile.Revision)
			}
			return m, m.loadMCPProfiles(), true
		}
		return m, nil, true
	case mcpInspected:
		if modal != nil && modal == v.modal {
			modal.busy = false
			modal.lines, modal.message, modal.failed = v.lines, v.problem, v.problem != ""
		}
		return m, nil, true
	}
	if modal == nil {
		return m, nil, false
	}
	switch v := msg.(type) {
	case tea.MouseMsg:
		return m, nil, true
	case cursor.BlinkMsg:
		if modal.mode == mcpEdit && modal.field == 0 {
			var cmd tea.Cmd
			modal.name, cmd = modal.name.Update(msg)
			return m, cmd, true
		}
	case tea.KeyMsg:
		return m.mcpKey(v)
	}
	return m, nil, false
}

func (m Model) mcpKey(k tea.KeyMsg) (Model, tea.Cmd, bool) {
	modal := m.mcp.modal
	key := k.String()
	switch key {
	case "ctrl+c":
		return m, tea.Quit, true
	case "ctrl+z":
		return m, tea.Suspend, true
	}
	if modal.busy {
		if key == "esc" && !modal.saving {
			m.mcp.modal = nil
		}
		return m, nil, true
	}
	if k.Paste && (modal.mode != mcpEdit || modal.field != 0) {
		return m, nil, true
	}
	if key == "esc" || (key == "n" && modal.mode == mcpConfirm) {
		modal.message, modal.failed = "", false
		switch modal.mode {
		case mcpConfirm:
			modal.mode = mcpEdit
			if modal.field == 0 {
				return m, modal.name.Focus(), true
			}
		case mcpList:
			m.mcp.modal = nil
		default:
			modal.mode = mcpList
		}
		return m, nil, true
	}
	switch modal.mode {
	case mcpList:
		switch key {
		case "up", "k":
			modal.selected = max(0, modal.selected-1)
		case "down", "j":
			modal.selected = min(max(0, len(modal.profiles)-1), modal.selected+1)
		case "n":
			if !modal.failed || len(modal.profiles) > 0 {
				modal.message, modal.failed = "", false
				return m, modal.edit(app.MCPProfile{}), true
			}
		case "enter", "d":
			if len(modal.profiles) > 0 {
				modal.message, modal.failed = "", false
				profile := modal.profiles[modal.selected]
				if key == "enter" {
					return m, modal.edit(profile), true
				}
				return m, m.inspectMCP(profile), true
			}
		case "a":
			modal.message, modal.failed = "", false
			return m, m.loadMCPAudit(), true
		case "r":
			modal.message, modal.failed = "", false
			return m, m.loadMCPProfiles(), true
		}
	case mcpEdit:
		return m, modal.editKey(k), true
	case mcpConfirm:
		if key == "y" {
			return m, m.saveMCPProfile(), true
		}
		m.scrollMCP(key, len(modal.reviewLines()))
	case mcpAudit, mcpTools:
		m.scrollMCP(key, len(modal.lines))
	}
	return m, nil, true
}

func (modal *mcpModal) editKey(k tea.KeyMsg) tea.Cmd {
	key := k.String()
	if k.Paste {
		var cmd tea.Cmd
		modal.name, cmd = modal.name.Update(k)
		return cmd
	}
	switch key {
	case "tab", "shift+tab", "up", "down":
		delta := 1
		if key == "shift+tab" || key == "up" {
			delta = -1
		}
		first, last := 0, len(modal.scopes)+1
		if modal.profile.Revision > 0 {
			first = 1
		}
		next := modal.field + delta
		if key == "tab" || key == "shift+tab" {
			span := last - first + 1
			next = first + (next-first+span)%span
		}
		modal.field = min(last, max(first, next))
		if modal.field == 0 {
			if !modal.name.Focused() {
				return modal.name.Focus()
			}
		} else {
			modal.name.Blur()
		}
	case "enter":
		profile := modal.profile
		if profile.Revision == 0 {
			profile.Name = modal.name.Value()
		}
		if !mcpserver.ValidProfileName(profile.Name) {
			modal.field = 0
			modal.message, modal.failed = "Use 1–32 lowercase letters, digits or hyphens; start with a letter or digit.", true
			return modal.name.Focus()
		}
		if err := mcpserver.ValidateProfile(&profile); err != nil {
			modal.message, modal.failed = err.Error(), true
			return nil
		}
		modal.name.Blur()
		modal.profile, modal.mode, modal.scroll = profile, mcpConfirm, 0
		modal.message, modal.failed = "", false
	default:
		if modal.field == 0 {
			var cmd tea.Cmd
			modal.name, cmd = modal.name.Update(k)
			return cmd
		}
		if key != " " && key != "left" && key != "right" {
			return nil
		}
		if modal.field == 1 {
			modal.profile.Enabled = !modal.profile.Enabled
		} else {
			scope := modal.scopes[modal.field-2]
			if i := slices.Index(modal.profile.Scopes, scope); i >= 0 {
				modal.profile.Scopes = slices.Delete(modal.profile.Scopes, i, i+1)
			} else {
				modal.profile.Scopes = append(modal.profile.Scopes, scope)
			}
		}
	}
	return nil
}

func mcpWriteScope(scope string) bool {
	return strings.HasSuffix(scope, ".write") || scope == "playbooks.execute"
}

func (modal *mcpModal) reviewLines() []string {
	profile := modal.profile
	status := "disabled · no active access"
	if profile.Enabled {
		status = "enabled · grants apply to clients using this profile"
	}
	lines := []string{fmt.Sprintf("Save %s · revision %d", profile.Name, profile.Revision), status}
	if len(profile.Scopes) == 0 {
		return append(lines, "No CRM scopes granted.")
	}
	writes := 0
	for _, scope := range profile.Scopes {
		if mcpWriteScope(scope) {
			writes++
		}
	}
	lines = append(lines, fmt.Sprintf("%d scopes selected · %d with write access", len(profile.Scopes), writes))
	for _, write := range []bool{true, false} {
		for _, scope := range profile.Scopes {
			if mcpWriteScope(scope) != write {
				continue
			}
			if write {
				scope = "WRITE · " + scope
			}
			lines = append(lines, scope)
		}
	}
	return lines
}

func (m Model) scrollMCP(key string, total int) {
	modal := m.mcp.modal
	_, height := m.formSize(74)
	page := max(1, height-2)
	last := max(0, total-page)
	step := 0
	switch key {
	case "up", "k":
		step = -1
	case "down", "j":
		step = 1
	case "pgup":
		step = -page
	case "pgdown":
		step = page
	case "home", "g":
		modal.scroll = 0
	case "end", "G":
		modal.scroll = last
	}
	modal.scroll = min(last, max(0, modal.scroll+step))
}

func (m Model) mcpPanel(r tideui.Renderer) *tideui.Overlay {
	modal := m.mcp.modal
	u := newSoftUI(r)
	w, h := m.formSize(74)
	title := "MCP profiles"
	switch modal.mode {
	case mcpEdit:
		title = "Edit MCP profile"
	case mcpConfirm:
		title = "Confirm MCP grant"
	case mcpTools:
		title = "MCP discovery & setup"
	case mcpAudit:
		title = "Recent MCP audit"
	}
	if modal.busy {
		message := "Loading…"
		if modal.saving {
			message = "Saving confirmed revision…"
		}
		return u.panel(title, w, []string{u.note(message, w, false)})
	}
	var body, footer []string
	if modal.message != "" {
		footer = append(footer, u.note(modal.message, w, modal.failed))
	}
	switch modal.mode {
	case mcpList:
		workspace := "live"
		if m.cfg.Demo {
			workspace = "demo"
		}
		body = append(body, u.note(workspace+" workspace · MCP is optional", w, false))
		rows := make([]string, 0, len(modal.profiles))
		for i, p := range modal.profiles {
			status := "disabled"
			if p.Enabled {
				status = "enabled"
			}
			prefix := "  "
			if i == modal.selected {
				prefix = "> "
			}
			rows = append(rows, r.RenderSoftRow(tideui.SoftRow{Prefix: prefix, Text: mcpText(p.Name), Suffix: fmt.Sprintf("%s · %d scopes", status, len(p.Scopes)), Selected: i == modal.selected}, w))
		}
		if len(rows) == 0 && !modal.failed {
			rows = append(rows, u.line("No profiles. New profiles start disabled with no scopes.", w))
		}
		footer = append(footer, u.hints(w, "n", "new", "enter", "edit", "d", "discovery", "a", "audit", "r", "reload", "esc", "close"))
		body = append(body, u.rows(rows, modal.selected, w, h-len(footer)-len(body))...)
	case mcpEdit:
		labelW := 27
		cw := u.controlWidth(w, labelW)
		name := u.preview(mcpText(modal.profile.Name), cw)
		if modal.profile.Revision == 0 {
			name = u.input(modal.name, cw, modal.field == 0)
		}
		rows := []string{u.row("Profile name", modal.field == 0, name, w, labelW)}
		value := "disabled"
		if modal.profile.Enabled {
			value = "enabled"
		}
		rows = append(rows, u.row("Client access", modal.field == 1, u.choice(value, cw, modal.field == 1), w, labelW))
		for i, scope := range modal.scopes {
			value = "not granted"
			if slices.Contains(modal.profile.Scopes, scope) {
				value = "granted"
			}
			label := scope
			if mcpWriteScope(scope) {
				label += " (WRITE)"
			}
			focused := modal.field == i+2
			rows = append(rows, u.row(label, focused, u.choice(value, cw, focused), w, labelW))
		}
		footer = append(footer, u.hints(w, "enter", "review", "tab/↑↓", "field", "space", "toggle", "esc", "cancel"))
		body = u.rows(rows, modal.field, w, h-len(footer))
	case mcpConfirm, mcpTools, mcpAudit:
		lines := modal.lines
		if modal.mode == mcpConfirm {
			lines = modal.reviewLines()
			footer = append(footer, u.hints(w, "y", "save grant", "↑↓", "review", "esc", "back"))
		} else {
			footer = append(footer, u.hints(w, "↑↓", "scroll", "esc", "back"))
		}
		page := max(1, h-len(footer)-1)
		start := min(modal.scroll, max(0, len(lines)-page))
		end := min(start+page, len(lines))
		for _, line := range lines[start:end] {
			body = append(body, u.line(line, w))
		}
		if len(lines) > page {
			body = append(body, u.note(fmt.Sprintf("%d–%d of %d", start+1, end, len(lines)), w, false))
		}
	}
	return u.fixedFormPanel(title, w, min(h, len(body)+len(footer)), body, footer)
}
