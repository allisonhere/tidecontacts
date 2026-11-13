package tui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tideui"
	tea "github.com/charmbracelet/bubbletea"
)

// editMsg carries a freshly loaded record to its form, so edits never start from a stale list row.
type editMsg struct {
	record any
	err    error
}

func (m *Model) fetchEdit(fn func(ctx context.Context) (any, error)) tea.Cmd {
	ctx := m.contacts.ctx
	return func() tea.Msg {
		v, e := fn(ctx)
		return editMsg{v, e}
	}
}

func (m Model) openEdit(v editMsg) (Model, tea.Cmd) {
	if v.err != nil {
		m.crm.err = v.err.Error()
		return m, nil
	}
	switch r := v.record.(type) {
	case domain.Task:
		return m, m.taskForm(r)
	case domain.Activity:
		if r.Provenance != domain.Local {
			m.crm.err = "This activity belongs to " + r.SourceApp + "; it can be linked (L) but not edited here"
			return m, nil
		}
		return m, m.activityForm(r)
	case domain.Project:
		return m, m.projectForm(r)
	case domain.PipelineItem:
		if p, ok := m.currentPipeline(); ok {
			return m, m.itemForm(r, p)
		}
	case domain.RadarConfig:
		return m, m.radarFormFor(r)
	case domain.Connections:
		ref := r.Root
		return m, m.openForm("Relationship notes · "+ref.Name, []formField{textArea("notes", "How you know each other, context worth remembering", r.Notes)}, func(m *Model, v map[string]string) (tea.Cmd, error) {
			repo := m.crm.repo
			return m.crmDo("Relationship notes saved", func(ctx context.Context) (string, error) {
				return ref.ID, repo.SetRelationshipNotes(ctx, ref.ID, v["notes"])
			}), nil
		})
	}
	return m, nil
}

func (m *Model) patchTask(id, note string, change func(*domain.Task) error) tea.Cmd {
	repo := m.crm.repo
	return m.crmDo(note, func(ctx context.Context) (string, error) {
		t, e := repo.Task(ctx, id)
		if e == nil {
			e = change(&t)
		}
		if e == nil {
			_, e = repo.SaveTask(ctx, t)
		}
		return id, e
	})
}

func (m *Model) patchProject(id, note string, change func(*domain.Project) error) tea.Cmd {
	repo := m.crm.repo
	return m.crmDo(note, func(ctx context.Context) (string, error) {
		p, e := repo.Project(ctx, id)
		if e == nil {
			e = change(&p)
		}
		if e == nil {
			_, e = repo.SaveProject(ctx, p)
		}
		return id, e
	})
}

func (m *Model) patchItem(id, note string, change func(*domain.PipelineItem)) tea.Cmd {
	repo := m.crm.repo
	return m.crmDo(note, func(ctx context.Context) (string, error) {
		it, e := repo.Item(ctx, id)
		if e == nil {
			change(&it)
			_, e = repo.SaveItem(ctx, it)
		}
		return id, e
	})
}

func (m *Model) patchPipeline(note string, change func(*domain.Pipeline) error) tea.Cmd {
	p, ok := m.currentPipeline()
	if !ok {
		m.crm.err = "Create a pipeline first"
		return nil
	}
	p.Stages = append([]domain.Stage{}, p.Stages...)
	repo := m.crm.repo
	return m.crmDo(note, func(ctx context.Context) (string, error) {
		if e := change(&p); e != nil {
			return "", e
		}
		_, e := repo.SavePipeline(ctx, p)
		return "", e
	})
}

func cycle(options []string, current string, delta int) string {
	for i, o := range options {
		if o == current {
			return options[(i+delta+len(options))%len(options)]
		}
	}
	return options[0]
}

// openURL hands an http(s) or mailto reference to the operating system, on explicit request only.
func openURL(url string) error {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "mailto:") {
		return fmt.Errorf("only http(s) and mailto references can be opened; this one is %q", url)
	}
	name, args := "xdg-open", []string{url}
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	}
	return exec.Command(name, args...).Start()
}

func (m Model) gotoInsights(view int) (Model, tea.Cmd) {
	m.pushNav()
	return m.goTo(navPoint{ws: wsInsights, view: view})
}

// crmGlobal handles commands that work from every workspace, using the current selection as context.
func (m Model) crmGlobal(id string) (Model, tea.Cmd, bool) {
	c := &m.crm
	if c.repo == nil {
		return m, nil, false
	}
	if c.busy {
		switch id {
		case "task-new", "activity-log", "meeting-log", "call-log", "note-add", "reminder-add", "followup-schedule", "flag", "date-add", "relationship-add", "merge-contacts":
			return m, nil, true
		}
	}
	link := m.contextRef()
	needPerson := func() (domain.Ref, bool) {
		ref, ok := m.personRef()
		if !ok {
			c.err = "Select a person or organization first"
			if m.isContacts() {
				m.contacts.err = c.err
			}
		}
		return ref, ok
	}
	switch id {
	case "task-new":
		return m, m.quickTask("", link, nil), true
	case "activity-log":
		return m, m.quickActivity("conversation", link), true
	case "meeting-log", "call-log", "note-add":
		return m, m.quickActivity(map[string]string{"meeting-log": "meeting", "call-log": "call", "note-add": "note"}[id], link), true
	case "reminder-add":
		return m, m.quickReminder(link), true
	case "followup-schedule":
		if ref, ok := needPerson(); ok {
			return m, m.followUpPrompt(ref), true
		}
		return m, nil, true
	case "date-add":
		if ref, ok := needPerson(); ok {
			return m, m.datePrompt(ref), true
		}
		return m, nil, true
	case "relationship-add":
		if ref, ok := needPerson(); ok {
			return m, m.relationshipAdd(ref), true
		}
		return m, nil, true
	case "relationship-notes":
		if ref, ok := needPerson(); ok {
			repo := c.repo
			return m, m.fetchEdit(func(ctx context.Context) (any, error) { return repo.Connections(ctx, ref.ID) }), true
		}
		return m, nil, true
	case "flag":
		if ref, ok := needPerson(); ok {
			repo := c.repo
			return m, m.crmDo("Review flag toggled for "+ref.Name, func(ctx context.Context) (string, error) {
				conn, e := repo.Connections(ctx, ref.ID)
				if e != nil {
					return "", e
				}
				return "", repo.SetFlag(ctx, ref.ID, !conn.Flagged, "")
			}), true
		}
		return m, nil, true
	case "relationship-open":
		if ref, ok := needPerson(); ok {
			m.pushNav()
			next, cmd := m.goTo(navPoint{ws: wsRelationships, id: ref.ID, focus: 1})
			return next, cmd, true
		}
		return m, nil, true
	case "inbox-review", "radar-open", "collections-open", "reminders-open", "followups-open":
		next, cmd := m.gotoInsights(map[string]int{"inbox-review": 0, "radar-open": 1, "collections-open": 2, "reminders-open": 3, "followups-open": 4}[id])
		return next, cmd, true
	case "radar-settings":
		repo := c.repo
		return m, m.fetchEdit(func(ctx context.Context) (any, error) { return repo.RadarConfig(ctx) }), true
	case "project-new":
		return m, m.projectForm(domain.Project{}), true
	case "project-open":
		return m, m.openPicker("Open project", "Type to search projects", "project", nil, func(m *Model, ref domain.Ref) tea.Cmd {
			next, cmd := m.follow(ref)
			*m = next
			return cmd
		}), true
	case "pipeline-new":
		return m, m.pipelineForm(domain.Pipeline{}), true
	case "collection-save":
		entity := map[int]string{wsPeople: "person", wsOrganizations: "organization", wsRelationships: "person", wsActivities: "activity", wsTasks: "task", wsProjects: "project", wsPipelines: "item"}[m.state.Active]
		if entity == "" {
			return m, m.collectionForm(domain.Collection{Entity: "person"}), true
		}
		query := strings.TrimSpace(m.queries[m.state.Active])
		order := ""
		switch m.state.Active {
		case wsPeople, wsOrganizations:
			order = m.contacts.order
		case wsTasks:
			order = taskSorts[c.taskSort]
			if l := m.list(); l.view < len(taskViewKeys) {
				query = strings.TrimSpace(map[string]string{"inbox": "status:inbox", "today": "is:open due:today", "upcoming": "is:open due:>today", "waiting": "status:waiting", "person": "is:open", "project": "is:open"}[taskViewKeys[l.view]] + " " + query)
			}
		case wsActivities:
			if t := activityViewTypes[min(m.list().view, len(activityViewTypes)-1)]; t != "" {
				query = strings.TrimSpace("type:" + t + " " + query)
			}
		}
		return m, m.openPrompt("Save as smart collection", fmt.Sprintf("Name for the %s filter “%s”. It stays live: results always reflect current data.", entity, orDefault(query, "everything")), "", func(m *Model, name string) (tea.Cmd, error) {
			col := domain.Collection{Name: name, Entity: entity, Query: query, Sort: order}
			col.Normalize()
			if e := col.Validate(); e != nil {
				return nil, e
			}
			repo := m.crm.repo
			return m.crmDo("Collection saved: "+name, func(ctx context.Context) (string, error) {
				_, e := repo.SaveCollection(ctx, col)
				return "", e
			}), nil
		}), true
	case "merge-contacts":
		if !m.isContacts() || m.contacts.contact.ID == "" {
			c.err = "Open People or Organizations, mark the duplicate with Space, select the record to keep, then run this command"
			return m, nil, true
		}
		keep := m.contacts.contact
		marked := []string{}
		for id, on := range m.contacts.marked {
			if on && id != keep.ID {
				marked = append(marked, id)
			}
		}
		if len(marked) != 1 {
			m.contacts.err = "Mark exactly one other contact with Space, then select the one to keep"
			return m, nil, true
		}
		name := marked[0]
		for _, row := range m.contacts.rows {
			if row.ID == marked[0] {
				name = row.Name
			}
		}
		repo, gone := c.repo, marked[0]
		m.confirmDo(fmt.Sprintf("Merge “%s” into “%s”?\n\nMethods, tags, notes, relationships, tasks, activities, projects and reminders move to “%s”. The merged record becomes a recoverable tombstone, but its links are not moved back if you restore it.", name, keep.DisplayName, keep.DisplayName),
			"Merged "+name+" into "+keep.DisplayName, func(ctx context.Context) error {
				_, e := repo.MergeContacts(ctx, keep.ID, gone)
				return e
			})
		return m, nil, true
	}
	return m, nil, false
}

// dossierAction handles link navigation inside the contact profile's CRM views.
func (m Model) dossierAction(id string) (Model, tea.Cmd, bool) {
	c := &m.crm
	if !m.dossierVisible() || c.repo == nil {
		return m, nil, false
	}
	if m.focus == 1 {
		switch id {
		case "down":
			c.dossierCursor = step(c.dossier, c.dossierCursor, 1)
			return m, nil, true
		case "up":
			c.dossierCursor = step(c.dossier, c.dossierCursor, -1)
			return m, nil, true
		case "top":
			c.dossierCursor = firstSelectable(c.dossier, 0)
			return m, nil, true
		case "bottom":
			c.dossierCursor = firstSelectable(c.dossier, len(c.dossier)-1)
			return m, nil, true
		case "activate":
			if row, ok := m.target(); ok && row.ref.ID != "" {
				next, cmd := m.follow(row.ref)
				return next, cmd, true
			}
			return m, nil, true
		}
		if row, ok := m.target(); ok {
			switch {
			case row.kind == "task":
				return m.recordAction(id, row)
			case row.kind == "date" && id == "dismiss":
				d := row.data.(domain.ImportantDate)
				repo := c.repo
				m.confirmDo("Remove the date “"+d.Label+"”?", "Date removed", func(ctx context.Context) error { return repo.DeleteDate(ctx, d.ID) })
				return m, nil, true
			}
		}
	}
	return m, nil, false
}

// recordAction applies a contextual command to one task, wherever it is shown.
func (m Model) recordAction(id string, row crmRow) (Model, tea.Cmd, bool) {
	c := &m.crm
	repo := c.repo
	today := domain.Day(clock())
	switch row.kind {
	case "task":
		t, _ := row.data.(domain.Task)
		switch id {
		case "complete":
			if !t.Open() {
				m.feedback = "Already completed — R reopens"
				return m, nil, true
			}
			return m, m.crmDo("Completed: "+t.Title, func(ctx context.Context) (string, error) { _, e := repo.CompleteTask(ctx, t.ID); return t.ID, e }), true
		case "reopen":
			return m, m.crmDo("Reopened: "+t.Title, func(ctx context.Context) (string, error) { _, e := repo.ReopenTask(ctx, t.ID); return t.ID, e }), true
		case "duplicate":
			return m, m.crmDo("Duplicated: "+t.Title, func(ctx context.Context) (string, error) { d, e := repo.DuplicateTask(ctx, t.ID); return d.ID, e }), true
		case "priority":
			return m, m.patchTask(t.ID, "Priority changed", func(t *domain.Task) error { t.Priority = (t.Priority + 1) % 4; return nil }), true
		case "stage-next", "stage-previous":
			delta := 1
			if id == "stage-previous" {
				delta = -1
			}
			next := cycle(domain.TaskStatuses[:4], t.Status, delta)
			return m, m.patchTask(t.ID, "Status: "+next, func(t *domain.Task) error { t.Status = next; return nil }), true
		case "archive":
			return m, m.patchTask(t.ID, "Archive toggled", func(t *domain.Task) error { t.Archived = !t.Archived; return nil }), true
		case "snooze", "reschedule":
			title, hint := "Snooze “"+t.Title+"” until", "tomorrow, fri, +3d, +1w, YYYY-MM-DD · none wakes it now"
			if id == "reschedule" {
				title, hint = "Reschedule “"+t.Title+"”", "A due date, optionally with a time: fri · +1w · 2026-12-01 14:30 · none clears it"
			}
			return m, m.openPrompt(title, hint, map[bool]string{true: "tomorrow"}[id == "snooze"], func(m *Model, value string) (tea.Cmd, error) {
				day, clockTime := value, ""
				if i := strings.LastIndex(value, " "); i > 0 && domain.ClockValid(value[i+1:]) && id == "reschedule" {
					day, clockTime = value[:i], value[i+1:]
				}
				resolved, e := domain.ParseWhen(day, today)
				if e != nil {
					return nil, e
				}
				return m.patchTask(t.ID, map[bool]string{true: "Snoozed until ", false: "Due "}[id == "snooze"]+orDefault(resolved, "cleared"), func(t *domain.Task) error {
					if id == "snooze" {
						t.SnoozedUntil = resolved
						return nil
					}
					t.Due, t.DueTime = resolved, clockTime
					if resolved == "" {
						t.DueTime = ""
					} else if t.Status == "inbox" {
						t.Status = "scheduled"
					}
					return nil
				}), nil
			}), true
		case "edit-contact":
			return m, m.fetchEdit(func(ctx context.Context) (any, error) { return repo.Task(ctx, t.ID) }), true
		case "delete-contact":
			m.confirmDo("Delete the task “"+t.Title+"”?\n\nIts reminders are dismissed. This cannot be undone from the interface.", "Task deleted", func(ctx context.Context) error { return repo.DeleteTask(ctx, t.ID) })
			return m, nil, true
		}
	}
	return m, nil, false
}

// linkPicker links a person, organization or project to the selected record.
func (m *Model) linkPicker(owner domain.Ref) tea.Cmd {
	// Each step uses the model it is handed: an overlay callback runs on a later copy.
	pick := func(m *Model, source string) tea.Cmd {
		return m.openPicker("Link to “"+owner.Name+"”", "Type to search · Enter links · x on a linked row unlinks", source, nil, func(m *Model, ref domain.Ref) tea.Cmd {
			repo := m.crm.repo
			return m.crmDo("Linked "+ref.Name, func(ctx context.Context) (string, error) { return owner.ID, repo.Link(ctx, owner, ref, true) })
		})
	}
	if owner.Kind == "project" {
		return pick(m, "entity")
	}
	return m.openPicker("Link what?", "", "", []domain.Ref{{ID: "entity", Name: "Person or organization"}, {ID: "project", Name: "Project"}}, func(m *Model, ref domain.Ref) tea.Cmd {
		return pick(m, ref.ID)
	})
}

// crmAction handles navigation and contextual commands inside the CRM workspaces.
func (m Model) crmAction(id string) (Model, tea.Cmd, bool) {
	if !m.isCRM() {
		return m, nil, false
	}
	c := &m.crm
	l := m.list()
	ws := m.state.Active
	repo := c.repo
	today := domain.Day(clock())
	board := ws == wsPipelines && m.focus == 1 && !c.itemOpen
	detailFocus := m.focus == 1 && !board || ws == wsPipelines && m.focus == 2
	reloadDetail := func() (Model, tea.Cmd, bool) { return m, m.loadCRMDetail(), true }
	if c.busy {
		return m, nil, true
	}
	switch id {
	case "down", "up", "page-down", "page-up", "top", "bottom":
		delta := map[string]int{"down": 1, "up": -1, "page-down": max(1, m.height-8), "page-up": -max(1, m.height-8), "top": -1 << 20, "bottom": 1 << 20}[id]
		sign := 1
		if delta < 0 {
			sign = -1
		}
		switch {
		case board:
			if c.col < len(c.board) {
				c.rowSel[c.col] = min(max(0, c.rowSel[c.col]+delta), max(0, len(c.board[c.col])-1))
			}
			return reloadDetail()
		case detailFocus:
			if !hasSelectable(c.detail) {
				c.scroll = max(0, c.scroll+delta)
				return m, nil, true
			}
			for i := 0; i < min(sign*delta, len(c.detail)); i++ {
				next := step(c.detail, c.cursor, sign)
				if next == c.cursor {
					break
				}
				c.cursor = next
			}
			return m, nil, true
		default:
			before := l.selected
			for i := 0; i < min(sign*delta, len(l.rows)); i++ {
				next := step(l.rows, l.selected, sign)
				if next == l.selected {
					break
				}
				l.selected = next
			}
			if l.selected == before {
				return m, nil, true
			}
			if ws == wsPipelines {
				c.pipeline, c.col = l.selected, 0
				return m, m.loadCRM(), true
			}
			return reloadDetail()
		}
	case "next", "previous":
		delta := 1
		if id == "previous" {
			delta = -1
		}
		if board {
			// On the board, left and right walk the stages; the edges lead to the neighbouring panes.
			if next := c.col + delta; next >= 0 && next < len(c.board) {
				c.col = next
				return reloadDetail()
			}
			if delta < 0 && m.mode() != tideui.Tabbed {
				m.focus = 0
			} else if delta > 0 && m.mode() == tideui.ThreeColumn {
				m.focus = 2
			}
			return m, nil, true
		}
		m.focus = (m.focus + delta + m.focusCount()) % m.focusCount()
		return m, nil, true
	case "activate":
		switch {
		case board:
			if _, ok := m.boardItem(); ok {
				if m.mode() == tideui.ThreeColumn {
					m.focus = 2
				} else {
					c.itemOpen = true
				}
			}
			return m, nil, true
		case detailFocus:
			if c.cursor < len(c.detail) && c.detail[c.cursor].selectable() && c.detail[c.cursor].ref.ID != "" {
				next, cmd := m.follow(c.detail[c.cursor].ref)
				return next, cmd, true
			}
			return m, nil, true
		}
		row, ok := m.listRow()
		if !ok {
			return m, nil, true
		}
		switch row.kind {
		case "more":
			l.pages = max(1, l.pages) + 1
			return m, m.loadCRM(), true
		case "inbox":
			if it := row.data.(domain.InboxItem); it.Source.Kind != "reminder" {
				next, cmd := m.follow(it.Source)
				return next, cmd, true
			}
		case "signal", "followup":
			next, cmd := m.follow(row.ref)
			return next, cmd, true
		case "collection":
			col := row.data.(domain.Collection)
			target := map[string]int{"person": wsPeople, "organization": wsOrganizations, "task": wsTasks, "activity": wsActivities, "project": wsProjects, "item": wsPipelines}[col.Entity]
			m.pushNav()
			p := navPoint{ws: target, query: col.Query}
			if target == wsTasks {
				p.view = 4
			}
			if target == wsProjects {
				c.projectFilter = 0
			}
			if col.Sort != "" && target <= wsOrganizations {
				m.contacts.order = col.Sort
			}
			next, cmd := m.goTo(p)
			next.feedback = "Collection: " + col.Name
			return next, cmd, true
		}
		m.focus = 1
		return m, nil, true
	case "back":
		if c.itemOpen {
			c.itemOpen = false
			return m, nil, true
		}
		if next, cmd, ok := m.back(); ok {
			return next, cmd, true
		}
		m.focus = 0
		return m, nil, true
	case "search":
		m.focus, m.editing, c.itemOpen = 0, true, false
		if ws == wsPipelines {
			m.focus = 1
		}
		return m, m.search.Focus(), true
	case "profile-next", "profile-previous":
		delta := 1
		if id == "profile-previous" {
			delta = -1
		}
		names := m.viewNames()
		if len(names) == 0 {
			return m, nil, true
		}
		if ws == wsPipelines {
			c.pipeline, c.col, c.itemOpen = (c.pipeline+delta+len(names))%len(names), 0, false
			return m, m.loadCRM(), true
		}
		l.view = (l.view + delta + len(names)) % len(names)
		if ws == wsRelationships || ws == wsProjects {
			c.cursor, c.scroll = 0, 0
			m.focus = 1
			return reloadDetail()
		}
		l.selected, l.pages = 0, 1
		return m, m.loadCRM(), true
	case "sort":
		switch ws {
		case wsTasks:
			c.taskSort = (c.taskSort + 1) % len(taskSorts)
			m.feedback = "Sort: " + taskSorts[c.taskSort]
		case wsProjects:
			c.projectFilter = (c.projectFilter + 1) % len(projectFilters)
			m.feedback = "Showing: " + orDefault(projectFilters[c.projectFilter], "current") + " projects"
		default:
			return m, nil, true
		}
		return m, m.loadCRM(), true
	case "new-contact":
		switch ws {
		case wsRelationships:
			return m.crmGlobal("relationship-add")
		case wsActivities:
			return m.crmGlobal("activity-log")
		case wsProjects:
			return m.crmGlobal("project-new")
		case wsPipelines:
			if p, ok := m.currentPipeline(); ok && m.focus >= 1 {
				it := domain.PipelineItem{}
				if c.col < len(p.Stages) {
					it.StageID = p.Stages[c.col].ID
				}
				return m, m.itemForm(it, p), true
			}
			return m.crmGlobal("pipeline-new")
		case wsInsights:
			switch l.view {
			case 2:
				return m, m.collectionForm(domain.Collection{Entity: "task"}), true
			case 3:
				return m.crmGlobal("reminder-add")
			}
		}
		return m.crmGlobal("task-new")
	case "item-new":
		if p, ok := m.currentPipeline(); ok && ws == wsPipelines {
			return m, m.itemForm(domain.PipelineItem{}, p), true
		}
		c.err = "Open Pipelines and create a pipeline first"
		return m, nil, true
	case "pipeline-edit":
		if p, ok := m.currentPipeline(); ok {
			return m, m.pipelineForm(p), true
		}
		return m, nil, true
	case "pipeline-archive":
		return m, m.patchPipeline("Pipeline archive toggled", func(p *domain.Pipeline) error { p.Archived = !p.Archived; return nil }), true
	case "pipeline-up", "pipeline-down":
		if p, ok := m.currentPipeline(); ok {
			delta := map[string]int{"pipeline-up": -1, "pipeline-down": 1}[id]
			c.pipeline = min(max(0, c.pipeline+delta), len(c.pipelines)-1)
			return m, m.crmDo("Pipeline moved", func(ctx context.Context) (string, error) { return "", repo.MovePipeline(ctx, p.ID, delta) }), true
		}
		return m, nil, true
	case "stage-add":
		return m, m.openPrompt("Add stage", "Name of the new stage; it is added after the current column", "", func(m *Model, name string) (tea.Cmd, error) {
			at := m.crm.col + 1
			return m.patchPipeline("Stage added: "+name, func(p *domain.Pipeline) error {
				at = min(at, len(p.Stages))
				p.Stages = append(p.Stages[:at], append([]domain.Stage{{Name: name}}, p.Stages[at:]...)...)
				return nil
			}), nil
		}), true
	case "stage-rename":
		if p, ok := m.currentPipeline(); ok && c.col < len(p.Stages) {
			col := c.col
			return m, m.openPrompt("Rename stage “"+p.Stages[col].Name+"”", "Stage history keeps the name each move was recorded under", p.Stages[col].Name, func(m *Model, name string) (tea.Cmd, error) {
				return m.patchPipeline("Stage renamed", func(p *domain.Pipeline) error { p.Stages[col].Name = name; return nil }), nil
			}), true
		}
		return m, nil, true
	case "stage-remove":
		if p, ok := m.currentPipeline(); ok && c.col < len(p.Stages) {
			col, name := c.col, p.Stages[c.col].Name
			c.confirm = &crmConfirm{text: "Remove the stage “" + name + "”?\n\nOnly an empty stage can be removed.", yes: func(m *Model) tea.Cmd {
				return m.patchPipeline("Stage removed", func(p *domain.Pipeline) error {
					p.Stages = append(p.Stages[:col], p.Stages[col+1:]...)
					return nil
				})
			}}
		}
		return m, nil, true
	case "stage-left", "stage-right":
		delta := map[string]int{"stage-left": -1, "stage-right": 1}[id]
		col := c.col
		if p, ok := m.currentPipeline(); !ok || col+delta < 0 || col+delta >= len(p.Stages) {
			return m, nil, true
		}
		c.col += delta
		return m, m.patchPipeline("Stage reordered", func(p *domain.Pipeline) error {
			p.Stages[col], p.Stages[col+delta] = p.Stages[col+delta], p.Stages[col]
			return nil
		}), true
	}

	row, ok := m.target()
	if !ok {
		switch id {
		case "complete", "reopen", "snooze", "reschedule", "priority", "stage-next", "stage-previous", "link", "convert-task", "duplicate", "dismiss", "reviewed",
			"open-external", "milestone-add", "edit-contact", "archive", "delete-contact", "favorite", "pipeline-move":
			m.feedback = "Nothing selected"
			return m, nil, true
		}
		return m, nil, false
	}
	owner, _ := m.listRow()
	kind := strings.TrimPrefix(row.kind, "link:")
	if strings.HasPrefix(row.kind, "link:") && id != "dismiss" && id != "activate" {
		// A plain link line stands for the list record, except for unlinking.
		row, kind = owner, owner.kind
	}
	if kind == "task" {
		if next, cmd, handled := m.recordAction(id, row); handled {
			return next, cmd, true
		}
	}
	switch id {
	case "edit-contact":
		switch kind {
		case "activity":
			return m, m.fetchEdit(func(ctx context.Context) (any, error) { return repo.Activity(ctx, row.id) }), true
		case "project":
			return m, m.fetchEdit(func(ctx context.Context) (any, error) { return repo.Project(ctx, row.id) }), true
		case "item":
			return m, m.fetchEdit(func(ctx context.Context) (any, error) { return repo.Item(ctx, row.id) }), true
		case "pipeline":
			return m, m.pipelineForm(row.data.(domain.Pipeline)), true
		case "collection":
			return m, m.collectionForm(row.data.(domain.Collection)), true
		case "person", "organization":
			m.pushNav()
			target := wsPeople
			if kind == "organization" {
				target = wsOrganizations
			}
			next, cmd := m.goTo(navPoint{ws: target, id: row.id, focus: 1})
			return next, cmd, true
		case "followup", "signal":
			return m.crmGlobal("followup-schedule")
		case "inbox":
			if it := row.data.(domain.InboxItem); it.Source.Kind != "reminder" {
				next, cmd := m.follow(it.Source)
				return next, cmd, true
			}
		}
		return m, nil, true
	case "complete":
		switch kind {
		case "milestone":
			ms := row.data.(domain.Milestone)
			return m, m.patchProject(owner.id, "Milestone updated", func(p *domain.Project) error {
				for i := range p.Milestones {
					if p.Milestones[i].ID == ms.ID {
						if p.Milestones[i].DoneAt == "" {
							p.Milestones[i].DoneAt = today
						} else {
							p.Milestones[i].DoneAt = ""
						}
					}
				}
				return nil
			}), true
		case "reminder":
			return m, m.crmDo("Reminder dismissed", func(ctx context.Context) (string, error) { return "", repo.DismissReminder(ctx, row.id) }), true
		case "inbox":
			it := row.data.(domain.InboxItem)
			switch {
			case it.Kind == "reminder":
				id := strings.TrimPrefix(it.Key, "reminder:")
				return m, m.crmDo("Reminder dismissed", func(ctx context.Context) (string, error) { return "", repo.DismissReminder(ctx, id) }), true
			case it.Source.Kind == "task":
				return m, m.crmDo("Completed: "+it.Title, func(ctx context.Context) (string, error) { _, e := repo.CompleteTask(ctx, it.Source.ID); return "", e }), true
			case it.Kind == "flagged":
				return m, m.crmDo("Flag cleared for "+it.Title, func(ctx context.Context) (string, error) { return "", repo.SetFlag(ctx, it.Source.ID, false, "") }), true
			case strings.HasPrefix(it.Kind, "followup"):
				// A follow-up is completed by recording the interaction that happened.
				return m, m.quickActivity("conversation", it.Person), true
			default:
				return m, m.crmDo("Marked reviewed", func(ctx context.Context) (string, error) { return "", repo.InboxAct(ctx, it.Key, "review", "") }), true
			}
		case "followup", "signal":
			return m, m.quickActivity("conversation", row.ref), true
		}
	case "reviewed":
		if kind == "inbox" {
			it := row.data.(domain.InboxItem)
			action := "review"
			if it.Status == "reviewed" {
				action = "reopen"
			}
			return m, m.crmDo("Inbox entry "+action+"ed", func(ctx context.Context) (string, error) { return it.Key, repo.InboxAct(ctx, it.Key, action, "") }), true
		}
	case "snooze":
		ask := func(title string, apply func(ctx context.Context, day string) error) (Model, tea.Cmd, bool) {
			return m, m.openPrompt(title, "tomorrow, fri, +3d, +1w or YYYY-MM-DD", "tomorrow", func(m *Model, value string) (tea.Cmd, error) {
				day, e := domain.ParseWhen(value, today)
				if e != nil || day == "" {
					return nil, fmt.Errorf("give a date such as tomorrow, fri, +3d or 2026-12-01")
				}
				return m.crmDo("Snoozed until "+day, func(ctx context.Context) (string, error) { return "", apply(ctx, day) }), nil
			}), true
		}
		morning := func(day string) time.Time { t, _ := domain.At(day, "09:00", clock().Location()); return t }
		switch kind {
		case "inbox":
			it := row.data.(domain.InboxItem)
			return ask("Snooze “"+it.Title+"”", func(ctx context.Context, day string) error {
				if it.Kind == "reminder" {
					return repo.SnoozeReminder(ctx, strings.TrimPrefix(it.Key, "reminder:"), morning(day))
				}
				return repo.InboxAct(ctx, it.Key, "snooze", day)
			})
		case "reminder":
			return ask("Snooze reminder", func(ctx context.Context, day string) error { return repo.SnoozeReminder(ctx, row.id, morning(day)) })
		case "followup", "signal", "person", "organization":
			return ask("Snooze follow-up for "+row.ref.Name, func(ctx context.Context, day string) error {
				f, e := repo.FollowUp(ctx, row.ref.ID)
				if e != nil {
					return e
				}
				f.SnoozedUntil = day
				return repo.SetFollowUp(ctx, f)
			})
		}
	case "reschedule":
		switch kind {
		case "milestone":
			ms := row.data.(domain.Milestone)
			return m, m.openPrompt("Date for milestone “"+ms.Title+"”", "fri, +2w, YYYY-MM-DD or none", ms.Due, func(m *Model, value string) (tea.Cmd, error) {
				day, e := domain.ParseWhen(value, today)
				if e != nil {
					return nil, e
				}
				return m.patchProject(owner.id, "Milestone date set", func(p *domain.Project) error {
					for i := range p.Milestones {
						if p.Milestones[i].ID == ms.ID {
							p.Milestones[i].Due = day
						}
					}
					return nil
				}), nil
			}), true
		case "project":
			return m, m.openPrompt("Target date for “"+row.text+"”", "fri, +2w, YYYY-MM-DD or none", "", func(m *Model, value string) (tea.Cmd, error) {
				day, e := domain.ParseWhen(value, today)
				if e != nil {
					return nil, e
				}
				return m.patchProject(row.id, "Target date set", func(p *domain.Project) error { p.Target = day; return nil }), nil
			}), true
		case "reminder":
			r := row.data.(domain.Reminder)
			return m, m.openPrompt("Reschedule reminder", "A date, optionally with a time: fri · tomorrow 08:30 · 2026-12-01 14:00", "", func(m *Model, value string) (tea.Cmd, error) {
				at, e := whenFrom(value, clock())
				if e != nil {
					return nil, e
				}
				r.At, r.SnoozedUntil = at, time.Time{}
				return m.crmDo("Reminder moved to "+at.Format("2006-01-02 15:04"), func(ctx context.Context) (string, error) { _, e := repo.SaveReminder(ctx, r); return r.ID, e }), nil
			}), true
		case "inbox":
			if it := row.data.(domain.InboxItem); it.Source.Kind == "task" {
				return m.recordAction("reschedule", crmRow{kind: "task", id: it.Source.ID, data: domain.Task{ID: it.Source.ID, Title: it.Title, Status: "next"}})
			}
			return m.crmGlobal("followup-schedule")
		default:
			return m.crmGlobal("followup-schedule")
		}
	case "priority":
		if kind == "project" {
			return m, m.patchProject(row.id, "Priority changed", func(p *domain.Project) error { p.Priority = (p.Priority + 1) % 4; return nil }), true
		}
	case "stage-next", "stage-previous", "pipeline-move":
		delta := map[string]int{"stage-next": 1, "stage-previous": -1}[id]
		switch kind {
		case "project":
			p := row.data.(domain.Project)
			next := cycle(domain.ProjectStatuses, p.Status, delta)
			return m, m.patchProject(row.id, "Project status: "+next, func(p *domain.Project) error { p.Status = next; return nil }), true
		case "item":
			p, ok := m.currentPipeline()
			if !ok {
				return m, nil, true
			}
			move := func(m *Model, stage int) tea.Cmd {
				if stage < 0 || stage >= len(p.Stages) {
					m.feedback = "Already at the edge of this pipeline"
					return nil
				}
				m.crm.col = stage
				target := p.Stages[stage]
				return m.crmDo("Moved to "+target.Name, func(ctx context.Context) (string, error) { return row.id, repo.MoveItem(ctx, row.id, target.ID) })
			}
			if id == "pipeline-move" {
				options := []domain.Ref{}
				for i, g := range p.Stages {
					options = append(options, domain.Ref{ID: fmt.Sprint(i), Name: g.Name, Role: fmt.Sprintf("%d", g.Count)})
				}
				return m, m.openPicker("Move “"+row.text+"” to stage", "", "", options, func(m *Model, ref domain.Ref) tea.Cmd {
					var i int
					_, _ = fmt.Sscan(ref.ID, &i)
					return move(m, i)
				}), true
			}
			return m, move(&m, c.col+delta), true
		}
	case "link":
		switch kind {
		case "task", "activity", "project", "item":
			return m, m.linkPicker(domain.Ref{Kind: kind, ID: row.id, Name: row.text}), true
		}
		m.feedback = "Linking applies to tasks, activities, projects and pipeline items"
		return m, nil, true
	case "convert-task":
		switch kind {
		case "activity":
			a := row.data.(domain.Activity)
			return m, m.quickTask("Follow up: "+a.Title, domain.Ref{}, func(t *domain.Task) {
				for _, r := range a.Entities {
					t.Entities = append(t.Entities, domain.Ref{ID: r.ID})
				}
				for _, r := range a.Projects {
					t.Projects = append(t.Projects, domain.Ref{ID: r.ID})
				}
				t.Activities = append(t.Activities, domain.Ref{ID: a.ID})
			}), true
		case "inbox":
			it := row.data.(domain.InboxItem)
			if it.Source.Kind == "task" {
				next, cmd := m.follow(it.Source)
				return next, cmd, true
			}
			return m, m.quickTask("Follow up with "+it.Title, it.Person, nil), true
		case "item":
			return m, m.quickTask(row.text+": ", domain.Ref{}, func(t *domain.Task) {}), true
		default:
			return m.crmGlobal("task-new")
		}
	case "dismiss":
		switch kind {
		case "inbox":
			it := row.data.(domain.InboxItem)
			return m, m.crmDo("Dismissed until something changes", func(ctx context.Context) (string, error) { return "", repo.InboxAct(ctx, it.Key, "dismiss", "") }), true
		case "reminder":
			return m, m.crmDo("Reminder dismissed", func(ctx context.Context) (string, error) { return "", repo.DismissReminder(ctx, row.id) }), true
		case "milestone":
			ms := row.data.(domain.Milestone)
			m.crm.confirm = &crmConfirm{text: "Remove the milestone “" + ms.Title + "”?", yes: func(m *Model) tea.Cmd {
				return m.patchProject(owner.id, "Milestone removed", func(p *domain.Project) error {
					kept := p.Milestones[:0]
					for _, x := range p.Milestones {
						if x.ID != ms.ID {
							kept = append(kept, x)
						}
					}
					p.Milestones = kept
					return nil
				})
			}}
			return m, nil, true
		case "date":
			d := row.data.(domain.ImportantDate)
			m.confirmDo("Remove the date “"+d.Label+"”?", "Date removed", func(ctx context.Context) error { return repo.DeleteDate(ctx, d.ID) })
			return m, nil, true
		case "edge":
			e := row.data.(domain.Edge)
			m.confirmDo(fmt.Sprintf("Remove the relationship “%s → %s”?", e.Type, e.Person.Name), "Relationship removed", func(ctx context.Context) error {
				contact, err := repo.Contact(ctx, owner.id)
				if err != nil {
					return err
				}
				kept := contact.Relationships[:0]
				for _, r := range contact.Relationships {
					if r.TargetID != e.Person.ID || r.Type != e.Type {
						kept = append(kept, r)
					}
				}
				contact.Relationships = kept
				_, err = repo.SaveContacts(ctx, []domain.Contact{contact}, "relationship")
				return err
			})
			return m, nil, true
		case "collection":
			return m.crmAction("delete-contact")
		case "person", "organization", "project", "task", "activity":
			// Unlink the row under the detail cursor from the record shown in the list.
			if strings.HasPrefix(row.kind, "link:") && (owner.kind == "task" || owner.kind == "activity" || owner.kind == "project" || owner.kind == "item") {
				from := domain.Ref{Kind: owner.kind, ID: owner.id}
				return m, m.crmDo("Unlinked "+row.ref.Name, func(ctx context.Context) (string, error) { return owner.id, repo.Link(ctx, from, row.ref, false) }), true
			}
		}
		m.feedback = "Nothing to dismiss or unlink here"
		return m, nil, true
	case "duplicate":
		m.feedback = "Only tasks can be duplicated"
		return m, nil, true
	case "open-external":
		if a, ok := row.data.(domain.Activity); ok && kind == "activity" {
			if a.ExternalURL == "" {
				m.feedback = "This activity has no external link"
				if a.ExternalRef != "" {
					m.feedback = "Reference in " + a.SourceApp + ": " + a.ExternalRef
				}
				return m, nil, true
			}
			if e := openURL(a.ExternalURL); e != nil {
				c.err = e.Error()
			} else {
				m.feedback = "Opened " + a.ExternalURL
			}
		}
		return m, nil, true
	case "milestone-add":
		if owner.kind != "project" {
			m.feedback = "Select a project first"
			return m, nil, true
		}
		return m, m.openPrompt("Add milestone to “"+owner.text+"”", "Title, optionally followed by due:fri or due:2026-12-01", "", func(m *Model, value string) (tea.Cmd, error) {
			cp, e := parseCapture(value, today)
			if e != nil {
				return nil, e
			}
			if cp.title == "" {
				return nil, fmt.Errorf("a milestone needs a title")
			}
			return m.patchProject(owner.id, "Milestone added", func(p *domain.Project) error {
				p.Milestones = append(p.Milestones, domain.Milestone{Title: cp.title, Due: cp.due})
				return nil
			}), nil
		}), true
	case "favorite":
		if kind == "collection" {
			col := row.data.(domain.Collection)
			col.Pinned = !col.Pinned
			return m, m.crmDo("Pin toggled", func(ctx context.Context) (string, error) { _, e := repo.SaveCollection(ctx, col); return col.ID, e }), true
		}
		return m, nil, true
	case "archive":
		switch kind {
		case "project":
			p := row.data.(domain.Project)
			next := "archived"
			if p.Status == "archived" {
				next = "active"
			}
			return m, m.patchProject(row.id, "Project "+next, func(p *domain.Project) error { p.Status = next; return nil }), true
		case "item":
			return m, m.patchItem(row.id, "Item archive toggled", func(it *domain.PipelineItem) { it.Archived = !it.Archived }), true
		case "pipeline":
			return m.crmAction("pipeline-archive")
		}
		return m, nil, true
	case "delete-contact":
		switch kind {
		case "activity":
			m.confirmDo("Delete the activity “"+row.text+"” from your timeline?\n\nRecords owned by another application are only hidden here; nothing is deleted there.", "Activity deleted", func(ctx context.Context) error { return repo.DeleteActivity(ctx, row.id) })
		case "project":
			m.confirmDo("Delete the project “"+row.text+"”?\n\nIts tasks and activities are kept but lose this project link.", "Project deleted", func(ctx context.Context) error { return repo.DeleteProject(ctx, row.id) })
		case "item":
			m.confirmDo("Delete the pipeline item “"+row.text+"”?", "Item deleted", func(ctx context.Context) error { return repo.DeleteItem(ctx, row.id) })
		case "collection":
			m.confirmDo("Delete the collection “"+row.text+"”?\n\nOnly the saved filter is removed, never the records it lists.", "Collection deleted", func(ctx context.Context) error { return repo.DeleteCollection(ctx, row.id) })
		case "reminder":
			m.confirmDo("Dismiss the reminder “"+row.text+"”?", "Reminder dismissed", func(ctx context.Context) error { return repo.DismissReminder(ctx, row.id) })
		case "pipeline":
			m.feedback = "Pipelines are archived, not deleted — use the archive command"
		}
		return m, nil, true
	}
	return m, nil, false
}
