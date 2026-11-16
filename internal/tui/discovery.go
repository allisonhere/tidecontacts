package tui

import (
	"fmt"
	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tideui"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
)

// workspaceActions keeps everyday actions visible and uses configured bindings.
func (m Model) workspaceActions() string {
	actions := [8][][2]string{
		{{"new-contact", "Add person"}, {"import-contacts", "Import contacts"}, {"search", "Search"}},
		{{"new-contact", "Add organization"}, {"search", "Search"}},
		{{"relationship-add", "Add relationship"}, {"followup-schedule", "Set follow-up"}, {"search", "Search"}},
		{{"activity-log", "Log activity"}, {"search", "Search"}},
		{{"task-new", "Add task"}, {"complete", "Complete"}, {"search", "Search"}},
		{{"new-contact", "Add project"}, {"search", "Search"}},
		{{"new-contact", "Add item / pipeline"}, {"stage-next", "Next stage"}},
		{{"reminder-add", "Add reminder"}, {"profile-next", "Next view"}},
	}
	parts := []string{}
	for _, a := range actions[m.state.Active] {
		parts = append(parts, fmt.Sprintf("%s [%s]", a[1], m.hint(a[0])))
	}
	parts = append(parts, "All actions ["+m.hint("palette")+"]", "Help ["+m.hint("help")+"]")
	return strings.Join(parts, "  ·  ")
}

func (m Model) workspaceNavigation() string {
	parts := []string{}
	for i, w := range domain.Workspaces() {
		label := fmt.Sprintf("%s %s", m.hint("workspace-"+w.ID), w.Name)
		if i == m.state.Active {
			label = "[" + label + "]"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "  ")
}

func (m Model) menuCommands() []app.Command {
	out := []app.Command{}
	for _, w := range domain.Workspaces() {
		out = append(out, app.Command{ID: "workspace-" + w.ID, Label: w.Name})
	}
	ids := [][]string{{"new-contact", "tidemail-sync"}, {"new-contact"}, {"relationship-add", "followup-schedule"}, {"activity-log", "tidemail-sync"}, {"task-new", "complete"}, {"project-new"}, {"pipeline-new", "item-new"}, {"reminder-add"}}
	for _, id := range ids[m.state.Active] {
		for _, c := range m.registry.Commands() {
			if c.ID == id {
				out = append(out, c)
			}
		}
	}
	out = append(out, app.Command{ID: "import-contacts", Label: "Import data…"}, app.Command{ID: "refresh-address-books", Label: "Refresh address books"}, app.Command{ID: "themes", Label: "Themes"}, app.Command{ID: "palette", Label: "All actions"}, app.Command{ID: "help", Label: "Help"})
	return out
}
func (m Model) menuPane() tideui.Pane {
	lines := []string{}
	r := tideui.NewRenderer(m.theme(), tideui.StyleOptions{Density: tideui.Compact})
	left, _ := m.ratios()
	width := max(1, int(float64(m.width)*left)-4)
	if m.mode() != tideui.ThreeColumn {
		width = max(1, min(54, m.width-4)-4)
	}
	for i, c := range m.menuCommands() {
		prefix := "  "
		selected := i == m.state.Active
		if m.menuOpen {
			selected = i == m.menuSelected
		}
		if selected {
			prefix = "> "
		}
		if i == 8 {
			lines = append(lines, "", "Actions")
		}
		label := c.Label
		if m.width < 140 {
			short := map[string]string{"workspace-organizations": "Organizations", "relationship-add": "Add relationship", "followup-schedule": "Follow-up", "new-contact": "Add record", "import-contacts": "Import", "tidemail-sync": "Sync TideMail", "activity-log": "Log activity", "task-new": "Add task", "project-new": "Add project", "pipeline-new": "Add pipeline", "item-new": "Add item", "reminder-add": "Reminder", "complete": "Complete"}
			if v, ok := short[c.ID]; ok {
				label = v
			}
		}
		key := m.hint(c.ID)
		if key == ": commands" {
			key = ""
		}
		rowRenderer := r
		if selected && !m.menuOpen {
			bg := tideui.MixColors(r.Styles.Theme.Bg, r.Styles.Theme.BorderFocus, .12)
			rowRenderer.Styles.ItemSelected = rowRenderer.Styles.ItemSelected.Background(bg).Foreground(tideui.ReadableText(r.Styles.Theme.BorderFocus, bg, 4.5))
		}
		lines = append(lines, rowRenderer.RenderRow(tideui.Row{Prefix: prefix, Text: label, Suffix: key, Selected: selected}, width))
	}
	return tideui.Pane{Title: "Menu", Content: strings.Join(lines, "\n"), Focused: m.menuOpen, ScrollOffset: max(0, m.menuSelected-max(1, m.height-9))}
}

// newFromMenu creates in the highlighted workspace, just as New does in its list.
func (m Model) newFromMenu() (tea.Model, tea.Cmd) {
	commands := m.menuCommands()
	selected := commands[min(max(0, m.menuSelected), len(commands)-1)]
	m.menuOpen = false
	var navigate tea.Cmd
	if strings.HasPrefix(selected.ID, "workspace-") {
		next, cmd := m.dispatch(selected.ID)
		m, navigate = next.(Model), cmd
	}
	// New in the workspace menu creates the workspace's primary record. In the
	// pipeline board, list/detail focus otherwise changes New into a new item.
	m.focus = 0
	next, create := m.dispatch("new-contact")
	return next, tea.Batch(navigate, create)
}
