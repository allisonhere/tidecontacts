package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
	tea "github.com/charmbracelet/bubbletea"
)

// clock is the presentation time source; tests replace it.
var clock = time.Now

const (
	wsPeople = iota
	wsOrganizations
	wsRelationships
	wsActivities
	wsTasks
	wsProjects
	wsPipelines
	wsInsights
)

// crmViews names the views cycled with [ and ]. For Relationships and Projects they
// are sections of the detail pane; elsewhere they select what the list shows.
var crmViews = [8][]string{
	wsRelationships: {"Tree", "People", "Organizations", "Projects", "Profile"},
	wsActivities:    {"All", "Meetings", "Calls", "Notes", "Emails", "Messages", "Conversations", "Events"},
	wsTasks:         {"Inbox", "Today", "Upcoming", "Waiting", "All", "By person", "By project"},
	wsProjects:      {"Overview", "People", "Tasks", "Timeline", "Milestones", "Notes"},
	wsInsights:      {"Inbox", "Radar", "Collections", "Reminders", "Follow-ups"},
}
var activityViewTypes = []string{"", "meeting", "call", "note", "email", "message", "conversation", "event"}
var taskViewKeys = []string{"inbox", "today", "upcoming", "waiting", "", "person", "project"}
var projectFilters = []string{"", "all", "planned", "active", "on-hold", "completed", "archived"}
var taskSorts = []string{"due", "priority", "title", "updated", "created"}
var crmScopes = [8]string{wsRelationships: "contact", wsActivities: "activity", wsTasks: "task", wsProjects: "project", wsPipelines: "item"}

type crmList struct {
	rows           []crmRow
	selected, view int
	pages          int
	seq            uint64
	loading        bool
	want           string // ID to select once the next load arrives
}

type navPoint struct {
	ws, view, focus int
	id, query, name string
}

type crmState struct {
	repo                    app.CRM
	lists                   [8]crmList
	detail                  []crmRow
	detailTitle, detailFor  string
	cursor, scroll          int
	detailSeq               uint64
	err                     string
	busy                    bool
	prompt                  *crmPrompt
	picker                  *crmPicker
	confirm                 *crmConfirm
	form                    *crmForm
	nav                     []navPoint
	pipelines               []domain.Pipeline
	pipeline, col           int
	board                   [][]domain.PipelineItem
	rowSel                  []int
	itemOpen                bool
	collections             []domain.Collection
	radar                   domain.RadarConfig
	due                     int
	projectFilter, taskSort int
	dossier                 []crmRow
	dossierFor              string
	dossierSeq              uint64
	dossierCursor           int
}

type crmListMsg struct {
	ws          int
	seq         uint64
	rows        []crmRow
	pipelines   []domain.Pipeline
	board       [][]domain.PipelineItem
	pipeline    int
	collections []domain.Collection
	radar       domain.RadarConfig
	err         error
}
type crmDetailMsg struct {
	seq   uint64
	id    string
	title string
	rows  []crmRow
	err   error
}
type crmDoneMsg struct {
	note, want string
	err        error
}
type crmSearchTick struct {
	ws  int
	seq uint64
}
type dossierMsg struct {
	seq  uint64
	id   string
	rows []crmRow
	err  error
}
type dueMsg struct{ count int }
type reminderTick struct{}

func (m Model) isCRM() bool { return m.crm.repo != nil && m.state.Active >= wsRelationships }

func (m *Model) list() *crmList { return &m.crm.lists[m.state.Active] }

// taskViewNames includes pinned task collections as custom views.
func (m Model) viewNames() []string {
	names := crmViews[m.state.Active]
	if m.state.Active == wsTasks {
		names = append([]string{}, names...)
		for _, c := range m.pinnedTaskViews() {
			names = append(names, c.Name)
		}
	}
	if m.state.Active == wsPipelines {
		names = []string{}
		for _, p := range m.crm.pipelines {
			names = append(names, p.Name)
		}
	}
	return names
}

func (m Model) pinnedTaskViews() []domain.Collection {
	out := []domain.Collection{}
	for _, c := range m.crm.collections {
		if c.Pinned && c.Entity == "task" {
			out = append(out, c)
		}
	}
	return out
}

func (m Model) viewName() string {
	names := m.viewNames()
	i := m.crm.lists[m.state.Active].view
	if m.state.Active == wsPipelines {
		i = m.crm.pipeline
	}
	if i >= 0 && i < len(names) {
		return names[i]
	}
	return ""
}

func tickReminders() tea.Cmd {
	return tea.Tick(time.Minute, func(time.Time) tea.Msg { return reminderTick{} })
}

// loadDue counts due reminders so the status bar can show them while the app runs.
func (m Model) loadDue() tea.Cmd {
	repo, ctx := m.crm.repo, m.contacts.ctx
	if repo == nil {
		return nil
	}
	return func() tea.Msg {
		list, e := repo.Reminders(ctx, true)
		if e != nil {
			return dueMsg{-1}
		}
		return dueMsg{len(list)}
	}
}

// runCollection evaluates a saved filter against current data.
func runCollection(ctx context.Context, repo app.CRM, c domain.Collection, extra string, now time.Time) ([]crmRow, error) {
	today := domain.Day(now)
	q, e := search.ParseScope(c.Entity, strings.TrimSpace(c.Query+" "+extra))
	if e != nil {
		return nil, fmt.Errorf("collection %q: %w", c.Name, e)
	}
	switch c.Entity {
	case "person", "organization":
		order := c.Sort
		if order == "" {
			order = "name"
		}
		list, e := repo.List(ctx, c.Entity, q, order)
		if len(list) > 500 {
			list = list[:500]
		}
		rows := summaryRows(list)
		if c.Group == "org" {
			grouped := []crmRow{}
			last := "\x00"
			for _, r := range rows {
				if r.meta != last {
					last = r.meta
					grouped = append(grouped, head(orDefault(r.meta, "No organization")))
				}
				grouped = append(grouped, r)
			}
			rows = grouped
		}
		return rows, e
	case "task":
		list, e := repo.Tasks(ctx, app.TaskFilter{Query: q, Sort: c.Sort})
		view := "group:" + c.Group
		if c.Group == "person" || c.Group == "project" {
			view = c.Group
		}
		return taskRows(list, view, today), e
	case "activity":
		list, e := repo.Activities(ctx, app.ActivityFilter{Query: q, Limit: 300})
		return activityRows(list, today), e
	case "project":
		list, e := repo.Projects(ctx, "", q)
		return projectRows(list, today), e
	default:
		list, e := repo.Items(ctx, "", q)
		rows := []crmRow{}
		stage := ""
		for _, it := range list {
			if label := it.Pipeline + " › " + it.Stage; label != stage {
				stage = label
				rows = append(rows, head(label))
			}
			rows = append(rows, itemRow(it, today))
		}
		return rows, e
	}
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// loadCRM reloads the active workspace list off the UI loop. Results carry a
// sequence number so anything superseded by later input is discarded.
func (m *Model) loadCRM() tea.Cmd {
	if !m.isCRM() {
		return nil
	}
	ws := m.state.Active
	l := m.list()
	text := m.queries[ws]
	var q search.Query
	if scope := crmScopes[ws]; scope != "" {
		var e error
		if q, e = search.ParseScope(scope, text); e != nil {
			m.crm.err = e.Error()
			return nil
		}
	}
	m.crm.err = ""
	l.seq++
	l.loading = true
	seq, view, want, pages := l.seq, l.view, l.want, max(1, l.pages)
	repo, ctx, now := m.crm.repo, m.contacts.ctx, clock()
	today := domain.Day(now)
	filter, order := projectFilters[m.crm.projectFilter], taskSorts[m.crm.taskSort]
	pipeline := m.crm.pipeline
	pinned := m.pinnedTaskViews()
	return func() tea.Msg {
		msg := crmListMsg{ws: ws, seq: seq, pipeline: pipeline}
		switch ws {
		case wsRelationships:
			for _, kind := range []string{"person", "organization"} {
				list, e := repo.List(ctx, kind, q, "name")
				if e != nil {
					msg.err = e
					break
				}
				msg.rows = append(msg.rows, summaryRows(list)...)
			}
		case wsActivities:
			if t := activityViewTypes[min(view, len(activityViewTypes)-1)]; t != "" {
				q.Filters = append(q.Filters, search.Filter{Key: "type", Value: t})
			}
			limit := 200 * pages
			list, e := repo.Activities(ctx, app.ActivityFilter{Query: q, Limit: limit, Include: want})
			msg.rows, msg.err = activityRows(list, today), e
			if len(list) >= limit {
				msg.rows = append(msg.rows, crmRow{kind: "more", id: "more", text: "Load older activities…"})
			}
		case wsTasks:
			msg.collections, _ = repo.Collections(ctx)
			if view >= len(taskViewKeys) {
				if i := view - len(taskViewKeys); i < len(pinned) {
					msg.rows, msg.err = runCollection(ctx, repo, pinned[i], text, now)
				}
				break
			}
			key := taskViewKeys[view]
			f := app.TaskFilter{View: key, Query: q, Sort: order}
			if key == "person" || key == "project" {
				f.View = "open"
			}
			list, e := repo.Tasks(ctx, f)
			msg.rows, msg.err = taskRows(list, key, today), e
		case wsProjects:
			list, e := repo.Projects(ctx, filter, q)
			msg.rows, msg.err = projectRows(list, today), e
		case wsPipelines:
			msg.pipelines, msg.err = repo.Pipelines(ctx, true)
			if msg.err != nil || len(msg.pipelines) == 0 {
				break
			}
			if want != "" {
				if it, e := repo.Item(ctx, want); e == nil {
					for i, p := range msg.pipelines {
						if p.ID == it.PipelineID {
							msg.pipeline = i
						}
					}
				}
			}
			msg.pipeline = min(max(0, msg.pipeline), len(msg.pipelines)-1)
			p := msg.pipelines[msg.pipeline]
			items, e := repo.Items(ctx, p.ID, q)
			msg.err = e
			msg.board = make([][]domain.PipelineItem, len(p.Stages))
			for _, it := range items {
				for i, g := range p.Stages {
					if g.ID == it.StageID {
						msg.board[i] = append(msg.board[i], it)
					}
				}
			}
			for _, p := range msg.pipelines {
				row := crmRow{kind: "pipeline", id: p.ID, text: p.Name, muted: p.Archived, data: p}
				if p.Archived {
					row.meta = "archived"
				}
				msg.rows = append(msg.rows, row)
			}
		case wsInsights:
			switch view {
			case 0:
				items, e := repo.Inbox(ctx)
				msg.rows, msg.err = inboxRows(items), e
			case 1:
				cfg, e := repo.RadarConfig(ctx)
				if e == nil {
					var signals []domain.Signal
					signals, e = repo.Radar(ctx, cfg)
					msg.rows = radarRows(signals, cfg)
				}
				msg.radar, msg.err = cfg, e
			case 2:
				list, e := repo.Collections(ctx)
				msg.rows, msg.collections, msg.err = collectionRows(list), list, e
			case 3:
				list, e := repo.Reminders(ctx, false)
				msg.rows, msg.err = reminderRows(list, now), e
			default:
				list, e := repo.FollowUps(ctx)
				msg.rows, msg.err = followRows(list, today), e
			}
			if terms := strings.Fields(strings.ToLower(text)); len(terms) > 0 {
				kept := []crmRow{}
				for _, r := range msg.rows {
					ok := r.kind != "header"
					for _, t := range terms {
						ok = ok && strings.Contains(strings.ToLower(r.text+" "+r.meta), t)
					}
					if ok {
						kept = append(kept, r)
					}
				}
				msg.rows = kept
			}
		}
		return msg
	}
}

// listRow is the selected row of the active list, or the selected board item.
func (m Model) listRow() (crmRow, bool) {
	c := m.crm
	if m.state.Active == wsPipelines && (m.focus >= 1 || c.itemOpen) {
		if it, ok := m.boardItem(); ok {
			return itemRow(it, domain.Day(clock())), true
		}
		return crmRow{}, false
	}
	l := c.lists[m.state.Active]
	if l.selected >= 0 && l.selected < len(l.rows) && l.rows[l.selected].selectable() {
		return l.rows[l.selected], true
	}
	return crmRow{}, false
}

func (m Model) boardItem() (domain.PipelineItem, bool) {
	c := m.crm
	if c.col < 0 || c.col >= len(c.board) || c.col >= len(c.rowSel) {
		return domain.PipelineItem{}, false
	}
	if i := c.rowSel[c.col]; i >= 0 && i < len(c.board[c.col]) {
		return c.board[c.col][i], true
	}
	return domain.PipelineItem{}, false
}

func (m Model) currentPipeline() (domain.Pipeline, bool) {
	if m.crm.pipeline >= 0 && m.crm.pipeline < len(m.crm.pipelines) {
		return m.crm.pipelines[m.crm.pipeline], true
	}
	return domain.Pipeline{}, false
}

// detailRows returns the link-navigable rows and cursor of the focused detail pane,
// which is the contact dossier in People and Organizations.
func (m *Model) detailRows() ([]crmRow, *int) {
	if m.isContacts() {
		return m.crm.dossier, &m.crm.dossierCursor
	}
	return m.crm.detail, &m.crm.cursor
}

// target is the record a contextual key acts on: the row under the detail cursor
// when the detail pane is focused and that row is itself a record, else the list row.
func (m Model) target() (crmRow, bool) {
	detailFocused := m.focus == 1
	switch {
	case m.isContacts():
		detailFocused = detailFocused && m.dossierVisible()
	case m.state.Active == wsPipelines:
		detailFocused = m.focus == 2 || m.crm.itemOpen
	}
	if detailFocused {
		rows, cursor := m.detailRows()
		if *cursor >= 0 && *cursor < len(rows) && rows[*cursor].selectable() {
			row := rows[*cursor]
			if row.kind == "link" {
				row.kind = "link:" + row.ref.Kind
			}
			return row, true
		}
	}
	if m.isContacts() {
		return crmRow{}, false
	}
	return m.listRow()
}

func step(rows []crmRow, from, delta int) int {
	for i := from + delta; i >= 0 && i < len(rows); i += delta {
		if rows[i].selectable() {
			return i
		}
	}
	return from
}

func firstSelectable(rows []crmRow, from int) int {
	from = min(max(0, from), max(0, len(rows)-1))
	if from < len(rows) && rows[from].selectable() {
		return from
	}
	if i := step(rows, from, 1); i != from {
		return i
	}
	return step(rows, from, -1)
}

func hasSelectable(rows []crmRow) bool {
	for _, r := range rows {
		if r.selectable() {
			return true
		}
	}
	return false
}

// loadCRMDetail fetches the detail pane for the selected record.
func (m *Model) loadCRMDetail() tea.Cmd {
	c := &m.crm
	c.detailSeq++
	row, ok := m.listRow()
	if !ok {
		c.detail, c.detailTitle, c.detailFor = nil, "", ""
		return nil
	}
	if c.detailFor != row.id {
		c.detail, c.cursor, c.scroll = nil, 0, 0
	}
	seq, repo, ctx, now := c.detailSeq, c.repo, m.contacts.ctx, clock()
	today := domain.Day(now)
	sub := c.lists[m.state.Active].view
	return func() tea.Msg {
		msg := crmDetailMsg{seq: seq, id: row.id, title: row.text}
		switch row.kind {
		case "task":
			t, e := repo.Task(ctx, row.id)
			msg.title, msg.rows, msg.err = t.Title, taskDetail(t, today), e
		case "activity":
			a, e := repo.Activity(ctx, row.id)
			msg.title, msg.rows, msg.err = a.Title, activityDetail(a), e
		case "project":
			p, e := repo.Project(ctx, row.id)
			var tasks []domain.Task
			var acts []domain.Activity
			if e == nil && sub == 2 {
				tasks, e = repo.Tasks(ctx, app.TaskFilter{ProjectID: row.id})
			}
			if e == nil && sub != 1 && sub != 2 && sub != 4 {
				acts, e = repo.Activities(ctx, app.ActivityFilter{ProjectID: row.id, Limit: 100})
			}
			msg.title, msg.rows, msg.err = p.Name, projectDetail(p, sub, tasks, acts, today), e
		case "item":
			it, e := repo.Item(ctx, row.id)
			msg.title, msg.rows, msg.err = it.Title, itemDetail(it, today), e
		case "person", "organization":
			conn, e := repo.Connections(ctx, row.id)
			msg.rows, msg.err = connectionsDetail(conn, sub, today), e
		case "inbox":
			it := row.data.(domain.InboxItem)
			msg.rows = []crmRow{txt(domain.InboxKinds[it.Kind]), head("Why this is here"), txt(it.Reason)}
			if it.Due != "" {
				msg.rows = append(msg.rows, txt("Date: "+it.Due+" ("+domain.Relative(it.Due, today)+")"))
			}
			msg.rows = append(msg.rows, head("Source"))
			if it.Source.Kind != "reminder" {
				msg.rows = append(msg.rows, lnk(it.Source, refTag(it.Source.Kind), it.Source.Name, ""))
			} else {
				msg.rows = append(msg.rows, txt("Reminder: "+it.Source.Name))
			}
			if it.Person.ID != "" && it.Person.ID != it.Source.ID {
				msg.rows = append(msg.rows, lnk(it.Person, refTag(it.Person.Kind), it.Person.Name, ""))
			}
			msg.rows = append(msg.rows, txt(""), dim("In inbox since "+stampDay(it.FirstSeen)+" · "+it.Status),
				head("Actions"), txt("Enter open source   c complete   t create/open task"), txt("z snooze   x dismiss   v mark reviewed   S change schedule"))
		case "signal":
			s := row.data.(domain.Signal)
			label := s.Kind
			for _, def := range domain.RadarSignals {
				if def[0] == s.Kind {
					label = def[1]
				}
			}
			msg.rows = []crmRow{txt(label), head("Why"), txt(s.Reason), head("Person"), lnk(s.Person, refTag(s.Person.Kind), s.Person.Name, ""),
				txt(""), dim("Signals are computed from recorded facts only. Adjust or disable any of them with the Radar settings command.")}
		case "collection":
			col := row.data.(domain.Collection)
			rows, e := runCollection(ctx, repo, col, "", now)
			n := 0
			for _, r := range rows {
				if r.selectable() {
					n++
				}
			}
			msg.rows = []crmRow{dim(fmt.Sprintf("%s · %s · %d result(s) right now", col.Entity, orDefault(col.Query, "no filter"), n))}
			if col.Description != "" {
				msg.rows = append(msg.rows, txt(col.Description))
			}
			msg.rows, msg.err = append(msg.rows, rows...), e
		case "reminder":
			r := row.data.(domain.Reminder)
			msg.rows = []crmRow{txt("Due " + r.Due().Format("Mon 2006-01-02 15:04 MST"))}
			if !r.Recurrence.IsZero() {
				msg.rows = append(msg.rows, txt("Repeats "+r.Recurrence.String()))
			}
			if r.SnoozedUntil.After(r.At) {
				msg.rows = append(msg.rows, txt("Snoozed; originally "+r.At.Format("2006-01-02 15:04")))
			}
			if r.Task.ID != "" {
				msg.rows = append(msg.rows, head("Task"), lnk(r.Task, "task", r.Task.Name, ""))
			}
			if r.Entity.ID != "" {
				msg.rows = append(msg.rows, head("Person"), lnk(r.Entity, refTag(r.Entity.Kind), r.Entity.Name, ""))
			}
			msg.rows = append(msg.rows, txt(""), dim("c or x dismiss · z snooze · S reschedule"))
		case "followup":
			f := row.data.(domain.FollowUp)
			msg.rows = append(followLines(f, today), head("Person"), lnk(f.Entity, refTag(f.Entity.Kind), f.Entity.Name, ""), txt(""), dim("F change schedule · A log an interaction · z snooze"))
		case "pipeline":
			p := row.data.(domain.Pipeline)
			if p.Description != "" {
				msg.rows = append(msg.rows, txt(p.Description))
			}
			msg.rows = append(msg.rows, head("Stages"))
			for _, g := range p.Stages {
				msg.rows = append(msg.rows, txt(fmt.Sprintf("%-24s %d", g.Name, g.Count)))
			}
		}
		return msg
	}
}

func (m Model) dossierVisible() bool { return m.isContacts() && m.contacts.view >= 7 }

// loadDossier fetches the bounded CRM context for the selected contact, only while
// one of the dossier views is showing.
func (m *Model) loadDossier() tea.Cmd {
	c := &m.crm
	id := m.contacts.contact.ID
	if c.repo == nil || !m.dossierVisible() || id == "" {
		return nil
	}
	c.dossierSeq++
	if c.dossierFor != id {
		c.dossier, c.dossierCursor = nil, 0
	}
	seq, repo, ctx, timeline, today := c.dossierSeq, c.repo, m.contacts.ctx, m.contacts.view == 8, domain.Day(clock())
	return func() tea.Msg {
		d, e := repo.Dossier(ctx, id)
		return dossierMsg{seq: seq, id: id, rows: dossierRows(d, timeline, today), err: e}
	}
}

func (m *Model) pushNav() {
	p := navPoint{ws: m.state.Active, focus: m.focus, query: m.queries[m.state.Active]}
	if m.isContacts() {
		p.view, p.id, p.name = m.contacts.view, m.contacts.contact.ID, m.contacts.contact.DisplayName
	} else {
		p.view = m.list().view
		if row, ok := m.listRow(); ok {
			p.id, p.name = row.id, row.text
		}
		if m.state.Active == wsPipelines {
			p.view = m.crm.pipeline
		}
	}
	if p.name == "" {
		p.name = domain.Workspaces()[p.ws].Name
	}
	m.crm.nav = append(m.crm.nav, p)
	if len(m.crm.nav) > 32 {
		m.crm.nav = m.crm.nav[1:]
	}
}

// goTo switches to a workspace position and reloads it, selecting p.id when it arrives.
func (m Model) goTo(p navPoint) (Model, tea.Cmd) {
	m.contacts.listSeq++
	m.contacts.detailSeq++
	m.state.Active, m.selected, m.focus = p.ws, p.ws, p.focus
	m.queries[p.ws] = p.query
	m.search.SetValue(p.query)
	m.editing = false
	m.search.Blur()
	m.crm.itemOpen = false
	if m.isContacts() {
		m.contacts.view = p.view
		if p.id != "" {
			m.contacts.contact = domain.Contact{ID: p.id}
		}
		return m, m.refreshContacts()
	}
	l := m.list()
	l.view, l.want = p.view, p.id
	if p.ws == wsPipelines {
		m.crm.pipeline, l.view = p.view, 0
	}
	return m, m.loadCRM()
}

// follow opens a linked record, remembering where the user came from.
func (m Model) follow(ref domain.Ref) (Model, tea.Cmd) {
	if ref.ID == "" || m.crm.repo == nil {
		return m, nil
	}
	p := navPoint{id: ref.ID, focus: 1}
	switch ref.Kind {
	case "person", "organization":
		if m.state.Active == wsRelationships {
			p.ws, p.view, p.focus = wsRelationships, m.list().view, 1
			break
		}
		p.ws, p.view = wsPeople, m.contacts.view
		if ref.Kind == "organization" {
			p.ws = wsOrganizations
		}
	case "task":
		p.ws, p.view = wsTasks, 4
	case "activity":
		p.ws = wsActivities
	case "project":
		p.ws, p.view = wsProjects, m.crm.lists[wsProjects].view
		m.crm.projectFilter = 1
	case "item":
		p.ws, p.view = wsPipelines, m.crm.pipeline
	case "reminder":
		p.ws, p.view = wsInsights, 3
	case "timeline":
		// The profile shows only the latest entries; the rest is paged in Activities.
		p.ws, p.id, p.focus, p.query = wsActivities, "", 0, "with:"+search.Quote(ref.Name)
	default:
		return m, nil
	}
	m.pushNav()
	return m.goTo(p)
}

func (m Model) back() (Model, tea.Cmd, bool) {
	n := len(m.crm.nav)
	if n == 0 {
		return m, nil, false
	}
	p := m.crm.nav[n-1]
	m.crm.nav = m.crm.nav[:n-1]
	next, cmd := m.goTo(p)
	return next, cmd, true
}

// updateCRM handles CRM messages and any open form or overlay before normal key routing.
func (m Model) updateCRM(msg tea.Msg) (Model, tea.Cmd, bool) {
	c := &m.crm
	switch v := msg.(type) {
	case reminderTick:
		return m, tea.Batch(tickReminders(), m.loadDue()), true
	case dueMsg:
		if v.count >= 0 {
			changed := v.count != c.due
			c.due = v.count
			if changed && m.isCRM() && m.state.Active == wsInsights && !c.busy && c.form == nil {
				return m, m.loadCRM(), true
			}
		}
		return m, nil, true
	case pickerResult:
		if c.picker != nil && v.seq == c.picker.seq {
			c.picker.options = v.options
			c.picker.selected = min(c.picker.selected, max(0, len(v.options)-1))
		}
		return m, nil, true
	case crmSearchTick:
		if v.ws == m.state.Active && m.isCRM() && v.seq == m.list().seq {
			return m, m.loadCRM(), true
		}
		return m, nil, true
	case crmListMsg:
		if v.ws != m.state.Active || !m.isCRM() || v.seq != m.list().seq {
			return m, nil, true
		}
		l := m.list()
		l.loading = false
		if v.collections != nil {
			c.collections = v.collections
		}
		if v.err != nil {
			if v.err != context.Canceled {
				c.err = v.err.Error()
			}
			return m, nil, true
		}
		keep := l.want
		if keep == "" && l.selected < len(l.rows) && l.selected >= 0 {
			keep = l.rows[l.selected].id
		}
		l.rows = v.rows
		at := -1
		for i, r := range l.rows {
			if r.id == keep && r.selectable() && keep != "" {
				at = i
				break
			}
		}
		if at < 0 {
			at = firstSelectable(l.rows, l.selected)
		}
		l.selected = at
		if v.ws == wsInsights && l.view == 1 {
			c.radar = v.radar
		}
		if v.ws == wsPipelines {
			c.pipelines, c.board, c.pipeline = v.pipelines, v.board, v.pipeline
			old := c.rowSel
			c.rowSel = make([]int, len(c.board))
			c.col = min(max(0, c.col), max(0, len(c.board)-1))
			for i := range c.board {
				if i < len(old) {
					c.rowSel[i] = min(old[i], max(0, len(c.board[i])-1))
				}
				for j, it := range c.board[i] {
					if it.ID == l.want && l.want != "" {
						c.col, c.rowSel[i] = i, j
					}
				}
			}
			for i, r := range l.rows {
				if p, ok := m.currentPipeline(); ok && r.id == p.ID {
					l.selected = i
				}
			}
		}
		l.want = ""
		return m, m.loadCRMDetail(), true
	case crmDetailMsg:
		if v.seq != c.detailSeq {
			return m, nil, true
		}
		if v.err != nil {
			if v.err != context.Canceled {
				c.err = v.err.Error()
			}
			return m, nil, true
		}
		c.detail, c.detailTitle, c.detailFor = v.rows, v.title, v.id
		c.cursor = firstSelectable(c.detail, c.cursor)
		return m, nil, true
	case dossierMsg:
		if v.seq != c.dossierSeq {
			return m, nil, true
		}
		if v.err != nil {
			m.contacts.err = v.err.Error()
			return m, nil, true
		}
		c.dossier, c.dossierFor = v.rows, v.id
		c.dossierCursor = firstSelectable(c.dossier, c.dossierCursor)
		return m, nil, true
	case crmDoneMsg:
		c.busy = false
		if v.err != nil {
			c.err = v.err.Error()
			if c.form != nil {
				c.form.err = v.err.Error()
			}
			if m.isContacts() {
				m.contacts.err = v.err.Error()
			}
			return m, nil, true
		}
		c.form, c.err = nil, ""
		m.feedback = v.note
		if m.isContacts() {
			m.contacts.err = ""
			if strings.HasPrefix(v.note, "Merged") {
				m.contacts.marked = nil
			}
			return m, tea.Batch(m.refreshContacts(), m.loadDossier(), m.loadDue()), true
		}
		if v.want != "" {
			m.list().want = v.want
		}
		return m, tea.Batch(m.loadCRM(), m.loadDue()), true
	}
	if c.form != nil {
		next, cmd := m.updateForm(msg)
		return next, cmd, true
	}
	if k, ok := msg.(tea.KeyMsg); ok && (c.prompt != nil || c.picker != nil || c.confirm != nil) {
		if k.String() == "ctrl+c" {
			return m, tea.Quit, true
		}
		next, cmd := m.updateOverlay(k)
		return next, cmd, true
	}
	return m, nil, false
}

func (m *Model) debounceCRM() tea.Cmd {
	l := m.list()
	l.seq++
	ws, seq := m.state.Active, l.seq
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return crmSearchTick{ws, seq} })
}
