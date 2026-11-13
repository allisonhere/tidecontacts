package tui

import (
	"fmt"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// fit clips or pads rendered text to exactly width cells.
func fit(s string, width int) string {
	s = ansi.Truncate(s, max(0, width), "")
	if gap := width - lipgloss.Width(s); gap > 0 {
		s += strings.Repeat(" ", gap)
	}
	return s
}

// renderRows turns rows into physical lines and reports the line holding the cursor.
// The cursor row is marked with "> " as well as the selection style, so focus never
// depends on colour.
func (m Model) renderRows(r tideui.Renderer, rows []crmRow, cursor int, active bool, width int) ([]string, int) {
	lines := []string{}
	at := 0
	for i, row := range rows {
		switch {
		case row.kind == "header":
			lines = append(lines, r.RenderSectionDivider(tideui.SectionDivider{Label: domain.SafeText(row.text), Width: width}, r.Styles.Theme.Bg))
		case !row.selectable():
			style := r.Styles.DetailBody
			if row.muted {
				style = r.Styles.DetailMeta
			}
			for _, line := range strings.Split(ansi.Wrap(domain.SafeText(row.text), max(1, width), ""), "\n") {
				lines = append(lines, style.Render(clip(line, width)))
			}
		default:
			prefix := "  "
			if i == cursor {
				prefix = "> "
				at = len(lines)
			}
			if row.tag != "" {
				prefix += row.tag
				if !strings.HasSuffix(row.tag, " ") {
					prefix += " "
				}
			}
			lines = append(lines, r.RenderRow(tideui.Row{Prefix: prefix, Text: domain.SafeText(row.text), Suffix: domain.SafeText(row.meta), Selected: i == cursor && active, Muted: row.muted}, width))
		}
	}
	return lines, at
}

// window returns the lines around at that fit in height, keeping at visible.
func window(lines []string, at, height int) []string {
	start, end := tideui.VisibleRange(len(lines), at, max(1, height))
	return lines[start:end]
}

// ratios returns the left and right pane shares. CRM lists carry whole rows of
// information (a timeline entry, a task with its date), so they get more room than a
// list of names; the pipeline list is only a switcher, so the board gets the space.
func (m Model) ratios() (float64, float64) {
	left, right := m.left.Value(), m.right.Value()
	if m.isCRM() {
		if m.state.Active == wsPipelines {
			left *= 0.75
		} else {
			left = min(0.46, left*1.75)
			right = min(right, 0.22)
		}
	}
	return left, right
}

func (m Model) paneWidths() (int, int, int) {
	left, right := m.ratios()
	nav := max(1, int(float64(m.width)*left)-4)
	detail := max(1, int(float64(m.width)*(1-left-right))-4)
	side := max(1, int(float64(m.width)*right)-4)
	switch m.mode() {
	case tideui.SidebarOnly:
		detail = max(1, m.width-int(float64(m.width)*left)-4)
	case tideui.Tabbed:
		nav = max(1, m.width-4)
		detail, side = nav, nav
	}
	return nav, detail, side
}

// boardContent draws the pipeline as keyboard-driven columns, or as a stage list with
// the focused stage's items when only one column fits.
func (m Model) boardContent(r tideui.Renderer, width, height int) string {
	c := m.crm
	p, ok := m.currentPipeline()
	if !ok {
		return r.Styles.DetailBody.Width(width).Render("No pipelines yet.\n\nA pipeline tracks anything that moves through stages: job applications, partnerships, client onboarding, personal goals.\n\nPress n to create one.")
	}
	active := m.focus == 1
	today := domain.Day(clock())
	const minimum = 22
	columns := min(len(p.Stages), max(1, width/minimum))
	lines := []string{}
	if columns <= 1 {
		for i, g := range p.Stages {
			prefix := "  "
			if i == c.col {
				prefix = "> "
			}
			lines = append(lines, r.RenderRow(tideui.Row{Prefix: prefix, Text: domain.SafeText(g.Name), Suffix: fmt.Sprint(len(c.board[i])), Selected: i == c.col && active, Muted: len(c.board[i]) == 0}, width))
		}
		if c.col < len(p.Stages) {
			lines = append(lines, r.RenderSectionDivider(tideui.SectionDivider{Label: domain.SafeText(p.Stages[c.col].Name), Width: width}, r.Styles.Theme.Bg))
			rows := []crmRow{}
			for _, it := range c.board[c.col] {
				rows = append(rows, itemRow(it, today))
			}
			items, at := m.renderRows(r, rows, c.rowSel[c.col], active, width)
			if len(items) == 0 {
				items = []string{r.Styles.DetailMeta.Render("  No items in this stage — n adds one")}
			}
			lines = append(lines, window(items, at, height-len(lines)-1)...)
		}
		return strings.Join(lines, "\n")
	}
	start, end := tideui.VisibleRange(len(p.Stages), c.col, columns)
	cell := max(1, (width-(end-start-1))/(end-start))
	cells := make([][]string, 0, end-start)
	for i := start; i < end; i++ {
		g := p.Stages[i]
		mark := "  "
		if i == c.col {
			mark = "▸ "
		}
		column := []string{r.RenderRow(tideui.Row{Prefix: mark, Text: domain.SafeText(g.Name), Suffix: fmt.Sprint(len(c.board[i])), Selected: i == c.col && active}, cell),
			r.Styles.DetailMeta.Render(strings.Repeat("─", cell))}
		rows := []crmRow{}
		for _, it := range c.board[i] {
			rows = append(rows, itemRow(it, today))
		}
		cursor := -1
		if i == c.col {
			cursor = c.rowSel[i]
		}
		items, at := m.renderRows(r, rows, cursor, active, cell)
		if len(items) == 0 {
			items = []string{r.Styles.DetailMeta.Render("  —")}
		}
		cells = append(cells, append(column, window(items, at, height-3)...))
	}
	for line := 0; line < height; line++ {
		parts := []string{}
		any := false
		for _, column := range cells {
			text := ""
			if line < len(column) {
				text, any = column[line], true
			}
			parts = append(parts, fit(text, cell))
		}
		if !any {
			break
		}
		lines = append(lines, strings.Join(parts, " "))
	}
	if start > 0 || end < len(p.Stages) {
		lines = append(lines, r.Styles.DetailMeta.Render(clip(fmt.Sprintf("stages %d–%d of %d · ←/→ for more", start+1, end, len(p.Stages)), width)))
	}
	return strings.Join(lines, "\n")
}

var crmHints = [8][]string{
	wsRelationships: {"Enter explore · e profile", "r relate · x remove", "F follow-up · d date", "! flag · A log · T task"},
	wsActivities:    {"n/A log · e edit", "t make task · L link", "o open link · D delete", "Enter follow links"},
	wsTasks:         {"n/T capture · e edit", "c complete · R reopen", "z snooze · S reschedule", "p priority · </> status", "L link · y duplicate", "a archive · D delete", "s sort"},
	wsProjects:      {"n new · e edit", "T task · A log · M milestone", "L link people · x unlink", "</> status · a archive", "s status filter"},
	wsPipelines:     {"←/→ stages · ↑/↓ items", "</> move item", "n new · e edit · Enter open", "[ ] switch pipeline", ": stage and pipeline commands"},
	wsInsights:      {"Enter open source", "c complete · t task", "z snooze · x dismiss", "v reviewed · S schedule", "n new · f pin · e edit"},
}

func (m Model) emptyText() string {
	ws := m.state.Active
	if m.queries[ws] != "" {
		return "Nothing matches this filter.\nEsc while searching clears it."
	}
	switch ws {
	case wsRelationships:
		return "No contacts yet.\nAdd people in workspace 1."
	case wsActivities:
		return "No activities recorded.\nPress n to log a call, meeting or note.\nNothing appears here unless you record or import it."
	case wsTasks:
		return "No tasks in this view.\nPress n to capture one."
	case wsProjects:
		return "No projects here.\nPress n to create one, or s to change the status filter."
	case wsInsights:
		return []string{"Inbox zero. Nothing needs your attention.", "No signals right now.", "No saved collections.\nPress C in any workspace to save its current filter.",
			"No pending reminders.\nPress b to add one.", "No follow-up schedules.\nPress F on a person to set one."}[min(m.list().view, 4)]
	}
	return ""
}

// crmPanes renders the list, detail and context panes of a CRM workspace.
func (m Model) crmPanes(r tideui.Renderer) [3]tideui.Pane {
	c := m.crm
	ws := m.state.Active
	l := c.lists[ws]
	navWidth, detailWidth, sideWidth := m.paneWidths()
	bodyHeight := max(1, m.height-6)
	name := domain.Workspaces()[ws].Name

	list := []string{}
	input := m.search
	input.Width = max(1, navWidth-3)
	input.TextStyle = r.Styles.DetailBody
	searchLine := clip("/ "+m.queries[ws], navWidth)
	if m.editing {
		searchLine = input.View()
	}
	list = append(list, searchLine)
	rows, at := m.renderRows(r, l.rows, l.selected, m.focus == 0, navWidth)
	list = append(list, window(rows, at, bodyHeight-1)...)
	if l.loading && len(l.rows) == 0 {
		list = append(list, "Loading…")
	} else if !hasSelectable(l.rows) {
		list = append(list, r.Styles.DetailMeta.Width(navWidth).Render(m.emptyText()))
	}
	if c.err != "" {
		list = append(list, r.Styles.DetailBody.Foreground(r.Styles.Theme.Error).Render("! "+clip(domain.SafeText(c.err), navWidth-2)))
	}
	count, position := 0, 0
	for i, row := range l.rows {
		if row.selectable() {
			count++
			if i <= l.selected {
				position = count
			}
		}
	}
	listTitle := fmt.Sprintf("%s %d/%d", name, position, count)
	if ws != wsRelationships && ws != wsProjects && ws != wsPipelines {
		listTitle = fmt.Sprintf("%s · %s %d/%d", name, m.viewName(), position, count)
	}

	detailTitle := orDefault(domain.SafeText(c.detailTitle), "Details")
	if ws == wsRelationships || ws == wsProjects {
		detailTitle = m.viewName() + " · " + detailTitle
	}
	detailLines, cursorLine := m.renderRows(r, c.detail, c.cursor, m.focus == 1 || ws == wsPipelines && m.focus == 2, detailWidth)
	if ws == wsPipelines {
		detailLines, cursorLine = m.renderRows(r, c.detail, c.cursor, m.focus == 2 || c.itemOpen, max(1, map[bool]int{true: sideWidth, false: detailWidth}[m.mode() == tideui.ThreeColumn]))
	}
	offset := 0
	if hasSelectable(c.detail) {
		offset = max(0, cursorLine-bodyHeight+2)
	} else {
		offset = min(c.scroll, max(0, len(detailLines)-bodyHeight))
	}
	if len(c.detail) == 0 {
		hint := "Select a record to see its details."
		if !hasSelectable(l.rows) {
			hint = m.emptyText()
		}
		detailLines = []string{r.Styles.DetailMeta.Width(detailWidth).Render(hint)}
	}
	detail := tideui.Pane{Title: detailTitle, Content: strings.Join(detailLines, "\n"), ScrollOffset: offset, Focused: m.focus == 1}

	side := []string{}
	if names := m.viewNames(); len(names) > 0 {
		side = append(side, "Views  [ / ]")
		current := l.view
		if ws == wsPipelines {
			current = c.pipeline
		}
		for i, n := range names {
			prefix := "  "
			if i == current {
				prefix = "> "
			}
			side = append(side, clip(prefix+domain.SafeText(n), sideWidth))
		}
		side = append(side, "")
	}
	side = append(side, crmHints[ws]...)
	switch ws {
	case wsTasks:
		side = append(side, "", "Sort: "+taskSorts[c.taskSort])
	case wsProjects:
		side = append(side, "", "Showing: "+orDefault(projectFilters[c.projectFilter], "current"))
	}
	if n := len(c.nav); n > 0 {
		side = append(side, "", clip("Esc ← "+domain.SafeText(c.nav[n-1].name), sideWidth))
	}
	if c.busy {
		side = append(side, "", "Saving…")
	}
	context := tideui.Pane{Title: "Context", Content: strings.Join(side, "\n"), Focused: m.focus == 2}

	if ws == wsPipelines {
		title := "Board"
		if p, ok := m.currentPipeline(); ok {
			title = domain.SafeText(p.Name)
			if m.queries[ws] != "" {
				title += " · /" + m.queries[ws]
			}
		}
		board := tideui.Pane{Title: title, Content: m.boardContent(r, detailWidth, bodyHeight), Focused: m.focus == 1}
		if c.itemOpen && m.mode() != tideui.ThreeColumn {
			board = tideui.Pane{Title: detailTitle + " · Esc board", Content: detail.Content, ScrollOffset: offset, Focused: true}
		}
		item := tideui.Pane{Title: detailTitle, Content: detail.Content, ScrollOffset: offset, Focused: m.focus == 2}
		if _, ok := m.boardItem(); !ok || m.focus == 0 {
			item = context
		}
		return [3]tideui.Pane{{Title: listTitle, Content: strings.Join(list, "\n"), Focused: m.focus == 0}, board, item}
	}
	return [3]tideui.Pane{{Title: listTitle, Content: strings.Join(list, "\n"), Focused: m.focus == 0}, detail, context}
}

func (m Model) crmStatus() (string, string) {
	ws := m.state.Active
	status := strings.ToUpper(domain.Workspaces()[ws].Name)
	if v := m.viewName(); v != "" {
		status += " / " + v
	}
	hints := "n new  / filter  : commands  ? help"
	if n := len(m.crm.nav); n > 0 {
		hints = "esc ← " + clip(domain.SafeText(m.crm.nav[n-1].name), 18) + "  " + hints
	}
	return status, hints
}
