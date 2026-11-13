package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
)

var bg = context.Background()

// crmStore opens a store whose clock is fixed at noon on Friday 2026-11-13 UTC.
func crmStore(t testing.TB) (*Store, *time.Time) {
	t.Helper()
	s := testStore(t)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	s.SetClock(func() time.Time { return now })
	return s, &now
}

func person(t testing.TB, s *Store, name string, edit ...func(*domain.Contact)) domain.Contact {
	t.Helper()
	c := domain.NewContact("person")
	c.DisplayName = name
	for _, f := range edit {
		f(&c)
	}
	saved, e := s.SaveContacts(bg, []domain.Contact{c}, "create")
	if e != nil {
		t.Fatal(e)
	}
	return saved[0]
}

func must[T any](t testing.TB) func(T, error) T {
	return func(v T, e error) T {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
}

func q(t testing.TB, scope, s string) search.Query {
	t.Helper()
	parsed, e := search.ParseScope(scope, s)
	if e != nil {
		t.Fatal(e)
	}
	return parsed
}

func TestActivitiesTimelineAndProvenance(t *testing.T) {
	s, now := crmStore(t)
	alex, sam := person(t, s, "Alex Rivera"), person(t, s, "Sam Lee")
	project := must[domain.Project](t)(s.SaveProject(bg, domain.Project{Name: "Harbor"}))
	meeting := must[domain.Activity](t)(s.SaveActivity(bg, domain.Activity{Type: "meeting", Title: "Kickoff", Body: "Discussed the café budget",
		OccurredAt: now.Add(-2 * time.Hour), Entities: []domain.Ref{{ID: alex.ID}, {Name: "Sam Lee"}}, Projects: []domain.Ref{{Name: "harbor"}}, Tags: []string{"Planning"}}))
	if len(meeting.Entities) != 2 || meeting.Projects[0].ID != project.ID || meeting.Provenance != domain.Local || meeting.Timezone == "" {
		t.Fatalf("%+v", meeting)
	}
	must[domain.Activity](t)(s.SaveActivity(bg, domain.Activity{Type: "call", Title: "Quick call", OccurredAt: now.AddDate(0, 0, -3), Entities: []domain.Ref{{ID: alex.ID}}}))
	must[domain.Activity](t)(s.SaveActivity(bg, domain.Activity{Type: "standup", Body: "Custom type note\nsecond line", OccurredAt: now.AddDate(0, 0, -40)}))
	for query, want := range map[string]int{"": 3, "type:meeting": 1, "with:sam": 1, "with:alex": 2, "project:harbor": 1, "tag:planning": 1, "cafe": 1, "kick": 1,
		"after:2026-11-01": 2, "before:2026-11-01": 1, "on:today": 1, "source:local": 3, "source:external": 0, "type:standup custom": 1, "with:alex type:call": 1} {
		list := must[[]domain.Activity](t)(s.Activities(bg, ActivityFilter{Query: q(t, "activity", query)}))
		if len(list) != want {
			t.Errorf("%q: got %d want %d", query, len(list), want)
		}
	}
	mine := must[[]domain.Activity](t)(s.Activities(bg, ActivityFilter{EntityID: sam.ID}))
	if len(mine) != 1 || len(mine[0].Entities) != 2 {
		t.Fatal("multi-person activity not shared", mine)
	}
	// Paging: newest first, cursor excludes what was already shown.
	page := must[[]domain.Activity](t)(s.Activities(bg, ActivityFilter{Limit: 2}))
	rest := must[[]domain.Activity](t)(s.Activities(bg, ActivityFilter{Limit: 2, Before: page[1].OccurredAt}))
	if len(page) != 2 || len(rest) != 1 || page[0].Title != "Kickoff" || rest[0].Type != "standup" {
		t.Fatal(page, rest)
	}
	if got := must[[]domain.Activity](t)(s.Activities(bg, ActivityFilter{Limit: 1, Include: rest[0].ID})); len(got) != 2 {
		t.Fatal("followed activity not included")
	}
	types := must[[]string](t)(s.ActivityTypes(bg))
	if types[len(types)-1] != "standup" {
		t.Fatal(types)
	}
	// Stale edits are refused, and editing replaces links.
	stale := meeting
	meeting.Entities = meeting.Entities[:1]
	meeting = must[domain.Activity](t)(s.SaveActivity(bg, meeting))
	if _, e := s.SaveActivity(bg, stale); e == nil {
		t.Fatal("stale activity accepted")
	}
	if len(meeting.Entities) != 1 {
		t.Fatal("links not replaced")
	}

	// External upserts are idempotent, keep provenance and never drop local links.
	ext := domain.Activity{Type: "email", Title: "Re: proposal", SourceApp: "tidemail", ExternalRef: "<msg-1@example.com>", ExternalURL: "https://mail.example/1", OccurredAt: now.Add(-time.Hour)}
	alexWithMail := alex
	alexWithMail.Methods = []domain.Method{{Kind: "email", Value: "alex@example.com"}}
	must[[]domain.Contact](t)(s.SaveContacts(bg, []domain.Contact{alexWithMail}, "edit"))
	first := must[domain.Activity](t)(s.UpsertExternalActivity(bg, ext, []string{"Alex@Example.com", "stranger@example.com"}))
	if first.Provenance != domain.External || len(first.Entities) != 1 || first.Entities[0].ID != alex.ID {
		t.Fatalf("%+v", first)
	}
	if e := s.Link(bg, domain.Ref{Kind: "activity", ID: first.ID}, domain.Ref{Kind: "person", ID: sam.ID}, true); e != nil {
		t.Fatal(e)
	}
	ext.Title = "Re: proposal (updated)"
	second := must[domain.Activity](t)(s.UpsertExternalActivity(bg, ext, nil))
	if second.ID != first.ID || second.Title != ext.Title || len(second.Entities) != 2 {
		t.Fatalf("upsert duplicated or lost links: %+v", second)
	}
	if n := len(must[[]domain.Activity](t)(s.Activities(bg, ActivityFilter{Query: q(t, "activity", "source:external app:tidemail")}))); n != 1 {
		t.Fatal("duplicate external rows", n)
	}
	second.Title = "tampered"
	if _, e := s.SaveActivity(bg, second); e == nil {
		t.Fatal("external activity edited locally")
	}
	if _, e := s.UpsertExternalActivity(bg, domain.Activity{Title: "x"}, nil); e == nil {
		t.Fatal("external activity without identifiers accepted")
	}
	if e := s.DeleteActivity(bg, meeting.ID); e != nil {
		t.Fatal(e)
	}
	if n := len(must[[]domain.Activity](t)(s.Activities(bg, ActivityFilter{Query: q(t, "activity", "kickoff")}))); n != 0 {
		t.Fatal("deleted activity searchable")
	}
}

func TestTaskLifecycleViewsAndRecurrence(t *testing.T) {
	s, now := crmStore(t)
	alex := person(t, s, "Alex Rivera")
	project := must[domain.Project](t)(s.SaveProject(bg, domain.Project{Name: "Harbor"}))
	save := func(task domain.Task) domain.Task { return must[domain.Task](t)(s.SaveTask(bg, task)) }
	inbox := save(domain.Task{Title: "Unsorted idea"})
	overdue := save(domain.Task{Title: "Send proposal", Status: "next", Due: "2026-11-10", Priority: 3, Entities: []domain.Ref{{ID: alex.ID}}, Projects: []domain.Ref{{ID: project.ID}}, Tags: []string{"sales"}})
	today := save(domain.Task{Title: "Call bank", Status: "scheduled", Due: "2026-11-13", DueTime: "15:00"})
	save(domain.Task{Title: "Plan offsite", Status: "scheduled", Due: "2026-11-20"})
	waiting := save(domain.Task{Title: "Contract signature", Status: "waiting", Entities: []domain.Ref{{Name: "Alex Rivera"}}})
	save(domain.Task{Title: "Starts today", Status: "scheduled", Start: "2026-11-13"})
	if inbox.Status != "inbox" || overdue.Entities[0].Name != "Alex Rivera" {
		t.Fatal(inbox, overdue)
	}
	views := map[string]int{"inbox": 1, "today": 3, "upcoming": 1, "waiting": 1, "open": 6, "": 6}
	for view, want := range views {
		if list := must[[]domain.Task](t)(s.Tasks(bg, TaskFilter{View: view})); len(list) != want {
			t.Errorf("view %q: got %d want %d", view, len(list), want)
		}
	}
	for query, want := range map[string]int{"is:overdue": 1, "due:today": 1, "due:week": 2, "pri:high": 1, "with:alex": 2, "project:harbor": 1, "tag:sales": 1,
		"status:waiting": 1, "propos": 1, "due:none": 3, "due:<2026-11-13": 1, "is:open with:alex project:harbor": 1} {
		if list := must[[]domain.Task](t)(s.Tasks(bg, TaskFilter{Query: q(t, "task", query)})); len(list) != want {
			t.Errorf("%q: got %d want %d", query, len(list), want)
		}
	}
	if list := must[[]domain.Task](t)(s.Tasks(bg, TaskFilter{EntityID: alex.ID, Sort: "priority"})); len(list) != 2 || list[0].ID != overdue.ID {
		t.Fatal("by person", list)
	}
	for _, bad := range []domain.Task{{Title: ""}, {Title: "x", Status: "someday"}, {Title: "x", Due: "tomorrow"}, {Title: "x", DueTime: "09:00"}, {Title: "x", Timezone: "Mars/Olympus"}} {
		if _, e := s.SaveTask(bg, bad); e == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	// Snoozing hides a task from Today until the date arrives.
	today.SnoozedUntil = "2026-11-15"
	today = save(today)
	if n := len(must[[]domain.Task](t)(s.Tasks(bg, TaskFilter{View: "today"}))); n != 2 {
		t.Fatal("snoozed task still shown", n)
	}
	*now = now.AddDate(0, 0, 2)
	if n := len(must[[]domain.Task](t)(s.Tasks(bg, TaskFilter{View: "today"}))); n != 3 {
		t.Fatal("snooze did not expire", n)
	}
	*now = now.AddDate(0, 0, -2)

	// Completion: idempotent, logs one timeline entry for linked tasks, none for private chores.
	done := must[domain.Task](t)(s.CompleteTask(bg, overdue.ID))
	again := must[domain.Task](t)(s.CompleteTask(bg, overdue.ID))
	if done.Status != "completed" || done.CompletedAt == "" || again.Revision != done.Revision {
		t.Fatal("completion not idempotent", done, again)
	}
	must[domain.Task](t)(s.CompleteTask(bg, inbox.ID))
	entries := must[[]domain.Activity](t)(s.Activities(bg, ActivityFilter{Query: q(t, "activity", "type:task-completion")}))
	if len(entries) != 1 || entries[0].Entities[0].ID != alex.ID || entries[0].Projects[0].ID != project.ID {
		t.Fatal("completion entries", entries)
	}
	reopened := must[domain.Task](t)(s.ReopenTask(bg, overdue.ID))
	if reopened.Status != "scheduled" || reopened.CompletedAt != "" {
		t.Fatal(reopened)
	}
	if n := len(must[[]domain.Activity](t)(s.Activities(bg, ActivityFilter{EntityID: alex.ID}))); n != 0 {
		t.Fatal("reopen kept completion entry", n)
	}
	must[domain.Task](t)(s.CompleteTask(bg, overdue.ID))
	if n := len(must[[]domain.Activity](t)(s.Activities(bg, ActivityFilter{EntityID: alex.ID}))); n != 1 {
		t.Fatal("re-completion should restore one entry", n)
	}
	stale := waiting
	waiting.Priority = 2
	waiting = save(waiting)
	if _, e := s.SaveTask(bg, stale); e == nil {
		t.Fatal("stale task accepted")
	}
	dup := must[domain.Task](t)(s.DuplicateTask(bg, waiting.ID))
	if dup.ID == waiting.ID || dup.Title != waiting.Title || len(dup.Entities) != 1 || dup.Revision != 1 {
		t.Fatal(dup)
	}
	if e := s.DeleteTask(bg, dup.ID); e != nil {
		t.Fatal(e)
	}

	// Recurrence: exactly one successor, even when completion is repeated or reopened.
	rule, _ := domain.ParseRecurrence("weekly")
	weekly := save(domain.Task{Title: "Weekly review", Status: "scheduled", Due: "2026-11-13", DueTime: "09:00", Timezone: "America/New_York", Recurrence: rule, Entities: []domain.Ref{{ID: alex.ID}}})
	if weekly.SeriesID == "" || weekly.Recurrence.Anchor != "2026-11-13" {
		t.Fatal(weekly)
	}
	for i := 0; i < 3; i++ {
		must[domain.Task](t)(s.CompleteTask(bg, weekly.ID))
		if i == 1 {
			must[domain.Task](t)(s.ReopenTask(bg, weekly.ID))
		}
	}
	series := must[[]domain.Task](t)(s.Tasks(bg, TaskFilter{Query: q(t, "task", "is:recurring")}))
	if len(series) != 2 {
		t.Fatalf("expected original plus one successor, got %d", len(series))
	}
	next := series[0]
	if next.Due != "2026-11-20" || next.DueTime != "09:00" || next.Occurrence != 1 || next.Status != "scheduled" || len(next.Entities) != 1 || next.Timezone != "America/New_York" {
		t.Fatalf("%+v", next)
	}
	if _, e := s.SaveTask(bg, domain.Task{Title: "No date", Recurrence: rule}); e == nil {
		t.Fatal("recurring task without a date accepted")
	}
}

func TestRemindersSnoozeDismissAndRecurrence(t *testing.T) {
	s, now := crmStore(t)
	ny, e := time.LoadLocation("America/New_York")
	if e != nil {
		t.Skip("no tzdata")
	}
	*now = time.Date(2026, 10, 30, 12, 0, 0, 0, ny)
	alex := person(t, s, "Alex Rivera")
	task := must[domain.Task](t)(s.SaveTask(bg, domain.Task{Title: "Renew passport"}))
	once := must[domain.Reminder](t)(s.SaveReminder(bg, domain.Reminder{Title: "Ping Alex", At: now.Add(-time.Hour), Entity: domain.Ref{ID: alex.ID}}))
	linked := must[domain.Reminder](t)(s.SaveReminder(bg, domain.Reminder{Title: "Passport", At: now.Add(-time.Minute), Task: domain.Ref{ID: task.ID}}))
	must[domain.Reminder](t)(s.SaveReminder(bg, domain.Reminder{Title: "Later", At: now.Add(48 * time.Hour)}))
	rule, _ := domain.ParseRecurrence("daily")
	daily := must[domain.Reminder](t)(s.SaveReminder(bg, domain.Reminder{Title: "Stand up", At: time.Date(2026, 10, 30, 9, 0, 0, 0, ny), Recurrence: rule}))
	due := must[[]domain.Reminder](t)(s.Reminders(bg, true))
	if len(due) != 3 || len(must[[]domain.Reminder](t)(s.Reminders(bg, false))) != 4 {
		t.Fatal(due)
	}
	if due[0].ID != daily.ID || due[1].Entity.Name != "Alex Rivera" || due[2].Task.Name != "Renew passport" {
		t.Fatalf("%+v", due)
	}
	if e := s.SnoozeReminder(bg, once.ID, now.Add(3*time.Hour)); e != nil {
		t.Fatal(e)
	}
	if n := len(must[[]domain.Reminder](t)(s.Reminders(bg, true))); n != 2 {
		t.Fatal("snoozed reminder still due", n)
	}
	*now = now.Add(4 * time.Hour)
	if n := len(must[[]domain.Reminder](t)(s.Reminders(bg, true))); n != 3 {
		t.Fatal("snooze did not expire", n)
	}
	// Dismissing twice is harmless; a one-time reminder is done, a recurring one advances once.
	for i := 0; i < 2; i++ {
		if e := s.DismissReminder(bg, once.ID); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.DismissReminder(bg, daily.ID); e != nil {
		t.Fatal(e)
	}
	all := must[[]domain.Reminder](t)(s.Reminders(bg, false))
	var stand domain.Reminder
	for _, m := range all {
		if m.ID == daily.ID {
			stand = m
		}
	}
	if len(all) != 3 || !stand.At.Equal(time.Date(2026, 10, 31, 9, 0, 0, 0, ny)) {
		t.Fatal(all, stand.At)
	}
	// Across the Nov 1 fall-back transition the reminder stays at 09:00 local, not 08:00.
	*now = time.Date(2026, 11, 3, 12, 0, 0, 0, ny)
	if e := s.DismissReminder(bg, daily.ID); e != nil {
		t.Fatal(e)
	}
	for _, m := range must[[]domain.Reminder](t)(s.Reminders(bg, false)) {
		if m.ID == daily.ID && !m.At.Equal(time.Date(2026, 11, 4, 9, 0, 0, 0, ny)) {
			t.Fatal("recurring reminder drifted or built a backlog:", m.At)
		}
	}
	// Completing a task settles its reminders.
	must[domain.Task](t)(s.CompleteTask(bg, task.ID))
	for _, m := range must[[]domain.Reminder](t)(s.Reminders(bg, false)) {
		if m.ID == linked.ID {
			t.Fatal("task reminder survived completion")
		}
	}
	if _, e := s.SaveReminder(bg, domain.Reminder{Title: "No date"}); e == nil {
		t.Fatal("undated reminder accepted")
	}
}

func TestProjectsMilestonesAndRelationships(t *testing.T) {
	s, _ := crmStore(t)
	alex, sam := person(t, s, "Alex Rivera"), person(t, s, "Sam Lee")
	p := must[domain.Project](t)(s.SaveProject(bg, domain.Project{Name: "Harbor", Description: "Waterfront redesign", Status: "planned", Priority: 2, Start: "2026-11-01", Target: "2026-11-10",
		Members: []domain.Ref{{ID: alex.ID, Role: "lead"}, {Name: "Sam Lee"}}, Tags: []string{"client"},
		Milestones: []domain.Milestone{{Title: "Brief", Due: "2026-11-05"}, {Title: "Concept", Due: "2026-12-01"}}}))
	if len(p.Members) != 2 || p.Members[0].Role != "lead" || len(p.Milestones) != 2 || p.Milestones[0].Title != "Brief" {
		t.Fatalf("%+v", p)
	}
	must[domain.Task](t)(s.SaveTask(bg, domain.Task{Title: "Site visit", Status: "waiting", Projects: []domain.Ref{{ID: p.ID}}}))
	done := must[domain.Task](t)(s.SaveTask(bg, domain.Task{Title: "Sign NDA", Status: "next", Projects: []domain.Ref{{ID: p.ID}}}))
	must[domain.Task](t)(s.CompleteTask(bg, done.ID))
	p = must[domain.Project](t)(s.Project(bg, p.ID))
	if p.OpenTasks != 1 || p.DoneTasks != 1 {
		t.Fatal(p.OpenTasks, p.DoneTasks)
	}
	p.Status = "active"
	p.Milestones[0].DoneAt = "2026-11-06"
	p.Milestones = append(p.Milestones[:1], domain.Milestone{Title: "Launch"})
	p.Members = p.Members[:1]
	p = must[domain.Project](t)(s.SaveProject(bg, p))
	if len(p.Milestones) != 2 || p.Milestones[1].Title != "Launch" || p.Milestones[0].DoneAt == "" || len(p.Members) != 1 {
		t.Fatalf("%+v", p)
	}
	timeline := must[[]domain.Activity](t)(s.Activities(bg, ActivityFilter{ProjectID: p.ID}))
	if len(timeline) != 2 || timeline[0].Type != "project-update" && timeline[1].Type != "project-update" {
		t.Fatal("status change and completion not on project timeline", timeline)
	}
	must[domain.Project](t)(s.SaveProject(bg, domain.Project{Name: "Old site", Status: "archived"}))
	for query, want := range map[string]int{"": 1, "status:archived": 1, "is:overdue": 1, "is:waiting": 1, "with:alex": 1, "with:sam": 0, "tag:client": 1, "waterfront": 1, "pri:medium": 1, "is:open": 1} {
		if list := must[[]domain.Project](t)(s.Projects(bg, "", q(t, "project", query))); len(list) != want {
			t.Errorf("%q: got %d want %d", query, len(list), want)
		}
	}
	if n := len(must[[]domain.Project](t)(s.Projects(bg, "all", search.Query{}))); n != 2 {
		t.Fatal("all", n)
	}
	for _, bad := range []domain.Project{{}, {Name: "x", Status: "paused"}, {Name: "x", Start: "2026-02-01", Target: "2026-01-01"}, {Name: "x", Milestones: []domain.Milestone{{}}}} {
		if _, e := s.SaveProject(bg, bad); e == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	if _, e := s.SaveProject(bg, domain.Project{Name: "Ghost", Members: []domain.Ref{{Name: "Nobody"}}}); e == nil {
		t.Fatal("unknown collaborator accepted")
	}
	// Contact filter by project, and the explorer's project connections.
	if rows := must[[]domain.Summary](t)(s.List(bg, "person", q(t, "contact", "project:harbor"), "name")); len(rows) != 1 || rows[0].ID != alex.ID {
		t.Fatal(rows)
	}
	_ = sam
	if e := s.DeleteProject(bg, p.ID); e != nil {
		t.Fatal(e)
	}
	if got := must[domain.Task](t)(s.Task(bg, done.ID)); got.Title != "Sign NDA" {
		t.Fatal("task lost with project")
	}
}

func TestPipelinesStagesMovementAndHistory(t *testing.T) {
	s, now := crmStore(t)
	alex := person(t, s, "Alex Rivera")
	p := must[domain.Pipeline](t)(s.SavePipeline(bg, domain.Pipeline{Name: "Job applications", Stages: []domain.Stage{{Name: "Applied"}, {Name: "Interview"}, {Name: "Offer"}}}))
	other := must[domain.Pipeline](t)(s.SavePipeline(bg, domain.Pipeline{Name: "Partnerships", Stages: []domain.Stage{{Name: "Idea"}}}))
	if _, e := s.SavePipeline(bg, domain.Pipeline{Name: "Bad", Stages: []domain.Stage{{Name: "A"}, {Name: "a"}}}); e == nil {
		t.Fatal("duplicate stage names accepted")
	}
	if _, e := s.SavePipeline(bg, domain.Pipeline{Name: "Empty"}); e == nil {
		t.Fatal("pipeline without stages accepted")
	}
	item := must[domain.PipelineItem](t)(s.SaveItem(bg, domain.PipelineItem{PipelineID: p.ID, Title: "Riverside Studio", NextAction: "Send portfolio", Entities: []domain.Ref{{ID: alex.ID}}, Tags: []string{"design"}}))
	if item.Stage != "Applied" || len(item.History) != 1 || item.History[0].To != "Applied" || item.History[0].From != "" {
		t.Fatalf("%+v", item)
	}
	*now = now.Add(24 * time.Hour)
	if e := s.MoveItem(bg, item.ID, p.Stages[1].ID); e != nil {
		t.Fatal(e)
	}
	if e := s.MoveItem(bg, item.ID, p.Stages[1].ID); e != nil { // no-op
		t.Fatal(e)
	}
	if e := s.MoveItem(bg, item.ID, other.Stages[0].ID); e == nil {
		t.Fatal("moved across pipelines")
	}
	item = must[domain.PipelineItem](t)(s.Item(bg, item.ID))
	if item.Stage != "Interview" || len(item.History) != 2 || item.History[1].From != "Applied" || item.StageEnteredAt != item.History[1].At || item.StageEnteredAt == item.CreatedAt {
		t.Fatalf("%+v", item)
	}
	// Editing through the form to another stage also records history.
	item.StageID, item.Stage = p.Stages[2].ID, ""
	item = must[domain.PipelineItem](t)(s.SaveItem(bg, item))
	if len(item.History) != 3 || item.History[2].To != "Offer" {
		t.Fatal(item.History)
	}
	// Rename, reorder and add stages; history keeps the names that applied at the time.
	p.Stages = []domain.Stage{p.Stages[2], {Name: "Screening"}, p.Stages[0], p.Stages[1]}
	p.Stages[0].Name = "Offer received"
	p.Name = "Applications"
	p = must[domain.Pipeline](t)(s.SavePipeline(bg, p))
	list := must[[]domain.Pipeline](t)(s.Pipelines(bg, false))
	if len(list) != 2 || list[0].Name != "Applications" || len(list[0].Stages) != 4 || list[0].Stages[0].Name != "Offer received" || list[0].Stages[0].Count != 1 || list[0].Stages[1].Name != "Screening" {
		t.Fatalf("%+v", list)
	}
	item = must[domain.PipelineItem](t)(s.Item(bg, item.ID))
	if item.Stage != "Offer received" || item.History[2].To != "Offer" {
		t.Fatal(item.Stage, item.History)
	}
	// A stage holding items cannot be removed; an empty one can.
	p.Stages = p.Stages[1:]
	if _, e := s.SavePipeline(bg, p); e == nil || !strings.Contains(e.Error(), "still holds") {
		t.Fatal("occupied stage removed", e)
	}
	p = must[[]domain.Pipeline](t)(s.Pipelines(bg, false))[0]
	p.Stages = append(p.Stages[:1], p.Stages[2:]...)
	if p = must[domain.Pipeline](t)(s.SavePipeline(bg, p)); len(p.Stages) != 3 {
		t.Fatal("empty stage not removed")
	}
	for query, want := range map[string]int{"": 1, "stage:offer": 1, "stage:applied": 0, "with:alex": 1, "tag:design": 1, "riverside": 1, "pipeline:partner": 0, "is:archived": 0} {
		if got := must[[]domain.PipelineItem](t)(s.Items(bg, "", q(t, "item", query))); len(got) != want {
			t.Errorf("%q: got %d want %d", query, len(got), want)
		}
	}
	if e := s.MovePipeline(bg, other.ID, -1); e != nil {
		t.Fatal(e)
	}
	other.Archived = true
	must[domain.Pipeline](t)(s.SavePipeline(bg, other))
	if list = must[[]domain.Pipeline](t)(s.Pipelines(bg, false)); len(list) != 1 {
		t.Fatal("archived pipeline listed")
	}
	if list = must[[]domain.Pipeline](t)(s.Pipelines(bg, true)); len(list) != 2 {
		t.Fatal("archived pipeline lost")
	}
	if e := s.DeleteItem(bg, item.ID); e != nil {
		t.Fatal(e)
	}
	if got := must[[]domain.PipelineItem](t)(s.Items(bg, p.ID, search.Query{})); len(got) != 0 {
		t.Fatal("deleted item listed")
	}
}

func TestFollowUpsConnectionsAndDossier(t *testing.T) {
	s, now := crmStore(t)
	alex := person(t, s, "Alex Rivera", func(c *domain.Contact) {
		c.Birthday = "--11-20"
		c.Memberships = []domain.Membership{{Organization: "Riverside Studio", Title: "Engineer", Current: true}}
	})
	sam := person(t, s, "Sam Lee", func(c *domain.Contact) {
		c.Memberships = []domain.Membership{{Organization: "Riverside Studio", Current: true}}
		c.Relationships = []domain.Relationship{{TargetID: alex.ID, Type: "mentor", Notes: "since 2020"}}
	})
	jo := person(t, s, "Jo Park", func(c *domain.Contact) { c.Relationships = []domain.Relationship{{TargetID: sam.ID, Type: "friend"}} })
	alex.Relationships = []domain.Relationship{{TargetID: jo.ID, Type: "colleague"}}
	alex = must[[]domain.Contact](t)(s.SaveContacts(bg, []domain.Contact{alex}, "edit"))[0]
	project := must[domain.Project](t)(s.SaveProject(bg, domain.Project{Name: "Harbor", Members: []domain.Ref{{ID: alex.ID}, {ID: sam.ID}}}))

	// Without a rule nothing is ever due, however old or absent the history is.
	if f := must[domain.FollowUp](t)(s.FollowUp(bg, alex.ID)); f.Due != "" || f.Reason != "" {
		t.Fatal("follow-up inferred without a rule", f)
	}
	must[domain.Activity](t)(s.SaveActivity(bg, domain.Activity{Type: "note", Title: "Private note", OccurredAt: now.AddDate(0, 0, -1), Entities: []domain.Ref{{ID: alex.ID}}}))
	must[domain.Activity](t)(s.SaveActivity(bg, domain.Activity{Type: "call", Title: "Catch up", OccurredAt: now.AddDate(0, 0, -20), Entities: []domain.Ref{{ID: alex.ID}, {ID: sam.ID}}}))
	must[domain.Activity](t)(s.SaveActivity(bg, domain.Activity{Type: "event", Title: "Conference", OccurredAt: now.AddDate(0, 0, 30), Entities: []domain.Ref{{ID: alex.ID}}}))
	if e := s.SetFollowUp(bg, domain.FollowUp{Entity: domain.Ref{ID: alex.ID}, Interval: domain.Interval{Every: 2, Unit: "week"}}); e != nil {
		t.Fatal(e)
	}
	f := must[domain.FollowUp](t)(s.FollowUp(bg, alex.ID))
	// Notes and future events are not interactions; the call 20 days ago is.
	if f.LastInteraction != "2026-10-24" || f.LastType != "call" || f.Due != "2026-11-07" || !strings.Contains(f.Reason, "last recorded call") {
		t.Fatalf("%+v", f)
	}
	if e := s.SetFollowUp(bg, domain.FollowUp{Entity: domain.Ref{ID: jo.ID}, Interval: domain.Interval{Every: 1, Unit: "month"}}); e != nil {
		t.Fatal(e)
	}
	if f = must[domain.FollowUp](t)(s.FollowUp(bg, jo.ID)); f.Due != "2026-12-13" || !strings.Contains(f.Reason, "no interaction recorded") {
		t.Fatalf("%+v", f)
	}
	if e := s.SetFollowUp(bg, domain.FollowUp{Entity: domain.Ref{ID: sam.ID}, NextDate: "2026-11-12"}); e != nil {
		t.Fatal(e)
	}
	all := must[[]domain.FollowUp](t)(s.FollowUps(bg))
	if len(all) != 3 || all[0].Entity.ID != alex.ID || all[1].Entity.ID != sam.ID {
		t.Fatal(all)
	}
	for query, want := range map[string]int{"followup:due": 2, "followup:week": 2, "followup:set": 3, "followup:none": 0, "has:followup": 3, "seen:30d": 2, "seen:7d": 0,
		"seen:never": 1, "birthday:week": 1, "birthday:today": 0, "date:month": 1, "has:dates": 1, "is:flagged": 0, "org:riverside seen:30d": 2} {
		if rows := must[[]domain.Summary](t)(s.List(bg, "person", q(t, "contact", query), "name")); len(rows) != want {
			t.Errorf("%q: got %d want %d", query, len(rows), want)
		}
	}
	// Recording a real interaction satisfies the explicit date and moves the cadence forward.
	must[domain.Activity](t)(s.SaveActivity(bg, domain.Activity{Type: "meeting", Title: "Lunch", OccurredAt: *now, Entities: []domain.Ref{{ID: alex.ID}, {ID: sam.ID}}}))
	if f = must[domain.FollowUp](t)(s.FollowUp(bg, sam.ID)); f.NextDate != "" || f.Due != "" {
		t.Fatalf("explicit follow-up not settled: %+v", f)
	}
	if f = must[domain.FollowUp](t)(s.FollowUp(bg, alex.ID)); f.Due != "2026-11-27" {
		t.Fatal(f.Due)
	}
	if e := s.SetFollowUp(bg, domain.FollowUp{Entity: domain.Ref{ID: alex.ID}, Never: true}); e != nil {
		t.Fatal(e)
	}
	if f = must[domain.FollowUp](t)(s.FollowUp(bg, alex.ID)); f.Due != "" {
		t.Fatal("never ignored")
	}

	if e := s.SaveDate(bg, domain.ImportantDate{Entity: domain.Ref{ID: alex.ID}, Label: "Work anniversary", Date: "2020-11-15", Annual: true}); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveDate(bg, domain.ImportantDate{Entity: domain.Ref{ID: alex.ID}, Label: "", Date: "2020-11-15"}); e == nil {
		t.Fatal("unlabelled date accepted")
	}
	dates := must[[]domain.ImportantDate](t)(s.Dates(bg, alex.ID))
	if len(dates) != 2 || dates[0].Label != "Work anniversary" || dates[0].Next != "2026-11-15" || !dates[1].Builtin || dates[1].Next != "2026-11-20" {
		t.Fatalf("%+v", dates)
	}
	if n := len(must[[]domain.ImportantDate](t)(s.UpcomingDates(bg, 3))); n != 1 {
		t.Fatal("upcoming", n)
	}
	if e := s.SetFlag(bg, alex.ID, true, "check address"); e != nil {
		t.Fatal(e)
	}
	if e := s.SetRelationshipNotes(bg, alex.ID, "Met at the harbor meetup"); e != nil {
		t.Fatal(e)
	}
	c := must[domain.Connections](t)(s.Connections(bg, alex.ID))
	if !c.Flagged || c.FlagNote != "check address" || c.Notes == "" || len(c.Outgoing) != 1 || c.Outgoing[0].Person.Name != "Jo Park" || len(c.Outgoing[0].Children) != 1 ||
		c.Outgoing[0].Children[0].Person.Name != "Sam Lee" || len(c.Incoming) != 1 || c.Incoming[0].Type != "mentor" {
		t.Fatalf("%+v", c)
	}
	if len(c.Organizations) != 1 || c.Organizations[0].Title != "Engineer" || len(c.Organizations[0].People) != 1 || c.Organizations[0].People[0].Name != "Sam Lee" {
		t.Fatalf("%+v", c.Organizations)
	}
	if len(c.Projects) != 1 || c.Projects[0].Project.ID != project.ID || len(c.Projects[0].People) != 1 || len(c.Shared) != 1 || c.Shared[0].Count != 2 || len(c.Recent) != 4 {
		t.Fatalf("%+v %+v %d", c.Projects, c.Shared, len(c.Recent))
	}
	org := must[[]domain.Summary](t)(s.List(bg, "organization", search.Query{}, "name"))[0]
	if oc := must[domain.Connections](t)(s.Connections(bg, org.ID)); len(oc.Organizations) != 1 || len(oc.Organizations[0].People) != 2 {
		t.Fatalf("%+v", oc.Organizations)
	}
	must[domain.Task](t)(s.SaveTask(bg, domain.Task{Title: "Send deck", Status: "next", Entities: []domain.Ref{{ID: alex.ID}}}))
	must[domain.Task](t)(s.SaveTask(bg, domain.Task{Title: "Awaiting reply", Status: "waiting", Entities: []domain.Ref{{ID: alex.ID}}}))
	d := must[domain.Dossier](t)(s.Dossier(bg, alex.ID))
	if len(d.OpenTasks) != 1 || len(d.Commitments) != 1 || d.OpenTaskCount != 2 || d.Activities != 4 || len(d.Dates) != 2 {
		t.Fatalf("%+v", d)
	}
}

func TestInboxDeduplicationDismissalAndRadar(t *testing.T) {
	s, now := crmStore(t)
	alex := person(t, s, "Alex Rivera", func(c *domain.Contact) { c.Birthday = "1990-11-15" })
	sam := person(t, s, "Sam Lee")
	if items := must[[]domain.InboxItem](t)(s.Inbox(bg)); len(items) != 1 || items[0].Kind != "date" {
		t.Fatal("unexpected entries in a quiet database", items)
	}
	overdue := must[domain.Task](t)(s.SaveTask(bg, domain.Task{Title: "Send proposal", Status: "waiting", Due: "2026-11-10", Entities: []domain.Ref{{ID: alex.ID}}}))
	must[domain.Task](t)(s.SaveTask(bg, domain.Task{Title: "Idea"}))
	must[domain.Reminder](t)(s.SaveReminder(bg, domain.Reminder{Title: "Ping Sam", At: now.Add(-time.Hour), Entity: domain.Ref{ID: sam.ID}}))
	must[domain.Activity](t)(s.SaveActivity(bg, domain.Activity{Type: "call", Title: "Call", OccurredAt: now.AddDate(0, 0, -30), Entities: []domain.Ref{{ID: sam.ID}}}))
	if e := s.SetFollowUp(bg, domain.FollowUp{Entity: domain.Ref{ID: sam.ID}, Interval: domain.Interval{Every: 2, Unit: "week"}}); e != nil {
		t.Fatal(e)
	}
	if e := s.SetFlag(bg, alex.ID, true, ""); e != nil {
		t.Fatal(e)
	}
	items := must[[]domain.InboxItem](t)(s.Inbox(bg))
	kinds := map[string]int{}
	keys := map[string]bool{}
	for _, it := range items {
		kinds[it.Kind]++
		if keys[it.Key] || it.Reason == "" || it.Source.ID == "" || it.FirstSeen == "" || it.Status != "open" {
			t.Fatalf("duplicate or unexplained entry: %+v", it)
		}
		keys[it.Key] = true
	}
	// The overdue waiting task appears once (as overdue), not also as "waiting".
	want := map[string]int{"task-overdue": 1, "task-inbox": 1, "reminder": 1, "followup-suggested": 1, "date": 1, "flagged": 1}
	if fmt.Sprint(kinds) != fmt.Sprint(want) || items[0].Urgency != 3 {
		t.Fatalf("%v %+v", kinds, items[0])
	}
	// Generation is idempotent.
	if again := must[[]domain.InboxItem](t)(s.Inbox(bg)); len(again) != len(items) || again[0].FirstSeen != items[0].FirstSeen {
		t.Fatal("inbox not stable")
	}
	taskKey := "task:" + overdue.ID
	for _, step := range [][2]string{{"dismiss", ""}, {"reopen", ""}, {"snooze", "2026-11-16"}} {
		if e := s.InboxAct(bg, taskKey, step[0], step[1]); e != nil {
			t.Fatal(e)
		}
		if got := must[[]domain.InboxItem](t)(s.Inbox(bg)); has(got, taskKey) != (step[0] == "reopen") {
			t.Fatalf("%s had the wrong effect", step[0])
		}
	}
	// A snooze expires on its date...
	*now = now.AddDate(0, 0, 3)
	if got := must[[]domain.InboxItem](t)(s.Inbox(bg)); !has(got, taskKey) {
		t.Fatal("snooze did not expire")
	}
	// ...a dismissal does not, however much time passes...
	if e := s.InboxAct(bg, taskKey, "dismiss", ""); e != nil {
		t.Fatal(e)
	}
	*now = now.AddDate(0, 0, 30)
	if got := must[[]domain.InboxItem](t)(s.Inbox(bg)); has(got, taskKey) {
		t.Fatal("dismissed entry reappeared without a change")
	}
	// ...until the underlying facts change.
	overdue = must[domain.Task](t)(s.Task(bg, overdue.ID))
	overdue.Due = "2026-12-01"
	must[domain.Task](t)(s.SaveTask(bg, overdue))
	if got := must[[]domain.InboxItem](t)(s.Inbox(bg)); !has(got, taskKey) {
		t.Fatal("changed record did not return to the inbox")
	}
	if e := s.InboxAct(bg, taskKey, "review", ""); e != nil {
		t.Fatal(e)
	}
	got := must[[]domain.InboxItem](t)(s.Inbox(bg))
	if last := got[len(got)-1]; last.Key != taskKey || last.Status != "reviewed" {
		t.Fatal("reviewed entry should stay, last", last)
	}
	// Completing the task removes its entry entirely.
	must[domain.Task](t)(s.CompleteTask(bg, overdue.ID))
	if got = must[[]domain.InboxItem](t)(s.Inbox(bg)); has(got, taskKey) {
		t.Fatal("completed task still in inbox")
	}
	if e := s.InboxAct(bg, "task:missing", "dismiss", ""); e == nil {
		t.Fatal("unknown key accepted")
	}

	// Radar: every signal has a reason; disabled signals and thresholds are respected.
	*now = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	must[domain.Activity](t)(s.SaveActivity(bg, domain.Activity{Type: "meeting", Title: "Sync", OccurredAt: now.AddDate(0, 0, -2), Entities: []domain.Ref{{ID: sam.ID}}}))
	must[domain.Task](t)(s.SaveTask(bg, domain.Task{Title: "Late", Status: "next", Due: "2026-11-01", Entities: []domain.Ref{{ID: alex.ID}}}))
	cfg := must[domain.RadarConfig](t)(s.RadarConfig(bg))
	signals := must[[]domain.Signal](t)(s.Radar(bg, cfg))
	count := map[string]int{}
	for _, sig := range signals {
		count[sig.Kind]++
		if sig.Reason == "" || sig.Person.ID == "" {
			t.Fatalf("unexplained signal %+v", sig)
		}
	}
	if count["commitments"] != 1 || count["dates"] != 1 || count["active"] != 1 || count["overdue"] != 1 || count["cadence"] != 1 || count["followup"] != 0 {
		t.Fatalf("%v", count)
	}
	cfg.Disabled["active"], cfg.UpcomingDays, cfg.LeadDays = true, 1, 30
	if e := s.SaveRadarConfig(bg, cfg); e != nil {
		t.Fatal(e)
	}
	cfg = must[domain.RadarConfig](t)(s.RadarConfig(bg))
	count = map[string]int{}
	for _, sig := range must[[]domain.Signal](t)(s.Radar(bg, cfg)) {
		count[sig.Kind]++
	}
	if count["active"] != 0 || count["dates"] != 0 || count["followup"] != 1 || count["cadence"] != 0 {
		t.Fatalf("config ignored: %v", count)
	}
}

func has(items []domain.InboxItem, key string) bool {
	for _, it := range items {
		if it.Key == key {
			return true
		}
	}
	return false
}

func TestCollectionsAreLiveFilters(t *testing.T) {
	s, _ := crmStore(t)
	alex := person(t, s, "Alex Rivera")
	c := must[domain.Collection](t)(s.SaveCollection(bg, domain.Collection{Name: "Open Harbor tasks", Entity: "task", Query: "is:open project:Harbor", Group: "status", Pinned: true}))
	must[domain.Collection](t)(s.SaveCollection(bg, domain.Collection{Name: "A people list", Entity: "person", Query: "has:tasks"}))
	if _, e := s.SaveCollection(bg, domain.Collection{Name: "open harbor TASKS", Entity: "task"}); e == nil {
		t.Fatal("duplicate name accepted")
	}
	if _, e := s.SaveCollection(bg, domain.Collection{Name: "x", Entity: "widget"}); e == nil {
		t.Fatal("unknown entity accepted")
	}
	list := must[[]domain.Collection](t)(s.Collections(bg))
	if len(list) != 2 || list[0].ID != c.ID || !list[0].Pinned || list[0].Group != "status" {
		t.Fatalf("%+v", list)
	}
	run := func() int {
		return len(must[[]domain.Task](t)(s.Tasks(bg, TaskFilter{Query: q(t, list[0].Entity, list[0].Query)})))
	}
	if run() != 0 {
		t.Fatal("empty collection has rows")
	}
	project := must[domain.Project](t)(s.SaveProject(bg, domain.Project{Name: "Project Harbor"}))
	task := must[domain.Task](t)(s.SaveTask(bg, domain.Task{Title: "Draft", Status: "next", Projects: []domain.Ref{{ID: project.ID}}, Entities: []domain.Ref{{ID: alex.ID}}}))
	if run() != 1 {
		t.Fatal("collection did not pick up new data")
	}
	if rows := must[[]domain.Summary](t)(s.List(bg, "person", q(t, "person", list[1].Query), "name")); len(rows) != 1 {
		t.Fatal(rows)
	}
	must[domain.Task](t)(s.CompleteTask(bg, task.ID))
	if run() != 0 {
		t.Fatal("collection did not drop completed task")
	}
	c.Name, c.Pinned = "Renamed", false
	must[domain.Collection](t)(s.SaveCollection(bg, c))
	if e := s.DeleteCollection(bg, list[1].ID); e != nil {
		t.Fatal(e)
	}
	if list = must[[]domain.Collection](t)(s.Collections(bg)); len(list) != 1 || list[0].Name != "Renamed" {
		t.Fatal(list)
	}
}

func TestMergeArchiveAndDeleteKeepReferencesIntact(t *testing.T) {
	s, now := crmStore(t)
	keep := person(t, s, "Alex Rivera", func(c *domain.Contact) {
		c.Methods = []domain.Method{{Kind: "email", Value: "alex@example.com", Preferred: true}}
		c.Tags = []string{"friend"}
	})
	dupe := person(t, s, "A. Rivera", func(c *domain.Contact) {
		c.Methods = []domain.Method{{Kind: "email", Value: "ALEX@example.com"}, {Kind: "email", Value: "alex@work.example", Preferred: true}, {Kind: "phone", Value: "+1 555 0100"}}
		c.Tags = []string{"Friend", "client"}
		c.Notes = "Prefers mornings"
		c.Birthday = "1990-05-01"
		c.Memberships = []domain.Membership{{Organization: "Riverside Studio", Current: true}}
	})
	sam := person(t, s, "Sam Lee", func(c *domain.Contact) {
		c.Relationships = []domain.Relationship{{TargetID: dupe.ID, Type: "friend"}, {TargetID: keep.ID, Type: "friend"}}
	})
	dupe = must[domain.Contact](t)(s.Contact(bg, dupe.ID))
	dupe.Relationships = []domain.Relationship{{TargetID: keep.ID, Type: "colleague"}, {TargetID: sam.ID, Type: "client"}}
	dupe = must[[]domain.Contact](t)(s.SaveContacts(bg, []domain.Contact{dupe}, "edit"))[0]
	activity := must[domain.Activity](t)(s.SaveActivity(bg, domain.Activity{Type: "call", Title: "Intro", OccurredAt: now.AddDate(0, 0, -1), Entities: []domain.Ref{{ID: dupe.ID}, {ID: keep.ID}}}))
	task := must[domain.Task](t)(s.SaveTask(bg, domain.Task{Title: "Follow up", Status: "waiting", Due: "2026-11-01", Entities: []domain.Ref{{ID: dupe.ID}}}))
	project := must[domain.Project](t)(s.SaveProject(bg, domain.Project{Name: "Harbor", Members: []domain.Ref{{ID: dupe.ID, Role: "lead"}}}))
	pipe := must[domain.Pipeline](t)(s.SavePipeline(bg, domain.Pipeline{Name: "P", Stages: []domain.Stage{{Name: "S"}}}))
	item := must[domain.PipelineItem](t)(s.SaveItem(bg, domain.PipelineItem{PipelineID: pipe.ID, Title: "Deal", Entities: []domain.Ref{{ID: dupe.ID}}}))
	must[domain.Reminder](t)(s.SaveReminder(bg, domain.Reminder{Title: "Ping", At: now.Add(time.Hour), Entity: domain.Ref{ID: dupe.ID}}))
	if e := s.SaveDate(bg, domain.ImportantDate{Entity: domain.Ref{ID: dupe.ID}, Label: "First met", Date: "2019-03-03"}); e != nil {
		t.Fatal(e)
	}
	if e := s.SetFollowUp(bg, domain.FollowUp{Entity: domain.Ref{ID: dupe.ID}, Interval: domain.Interval{Every: 1, Unit: "month"}}); e != nil {
		t.Fatal(e)
	}
	must[[]domain.InboxItem](t)(s.Inbox(bg))

	merged := must[domain.Contact](t)(s.MergeContacts(bg, keep.ID, dupe.ID))
	if len(merged.Methods) != 3 || len(merged.Tags) != 2 || merged.Birthday != "1990-05-01" || !strings.Contains(merged.Notes, "mornings") || len(merged.Memberships) != 1 ||
		len(merged.Aliases) != 1 || merged.Aliases[0] != "A. Rivera" {
		t.Fatalf("%+v", merged)
	}
	preferred := 0
	for _, m := range merged.Methods {
		if m.Kind == "email" && m.Preferred {
			preferred++
		}
	}
	if preferred != 1 || len(merged.Relationships) != 1 || merged.Relationships[0].TargetID != sam.ID || len(merged.Incoming) != 1 {
		t.Fatalf("relationships after merge: %+v incoming %+v", merged.Relationships, merged.Incoming)
	}
	gone := must[domain.Contact](t)(s.Contact(bg, dupe.ID))
	if !gone.Deleted || !strings.Contains(string(gone.Extensions["merged_into"]), keep.ID) {
		t.Fatal("merged record is not a tombstone naming its successor")
	}
	// Nothing anywhere still points at the merged record.
	for _, table := range []string{"activity_entities", "task_entities", "project_entities", "item_entities", "reminders", "important_dates", "followup_prefs", "relationship_meta"} {
		var n int
		if e := s.db.QueryRow("SELECT count(*) FROM "+table+" WHERE entity_id=?", dupe.ID).Scan(&n); e != nil || n != 0 {
			t.Fatalf("%s still references merged contact: %d %v", table, n, e)
		}
	}
	var n int
	if e := s.db.QueryRow("SELECT count(*) FROM relationships WHERE source_id=?1 OR target_id=?1", dupe.ID).Scan(&n); e != nil || n != 0 {
		t.Fatal("relationships reference merged contact", n, e)
	}
	if a := must[domain.Activity](t)(s.Activity(bg, activity.ID)); len(a.Entities) != 1 || a.Entities[0].ID != keep.ID {
		t.Fatalf("%+v", a.Entities)
	}
	if got := must[domain.Task](t)(s.Task(bg, task.ID)); got.Entities[0].ID != keep.ID {
		t.Fatal(got.Entities)
	}
	if got := must[domain.Project](t)(s.Project(bg, project.ID)); got.Members[0].ID != keep.ID || got.Members[0].Role != "lead" {
		t.Fatal(got.Members)
	}
	if got := must[domain.PipelineItem](t)(s.Item(bg, item.ID)); got.Entities[0].ID != keep.ID {
		t.Fatal(got.Entities)
	}
	if f := must[domain.FollowUp](t)(s.FollowUp(bg, keep.ID)); f.Interval.IsZero() {
		t.Fatal("follow-up schedule lost in merge")
	}
	if list := must[[]domain.Task](t)(s.Tasks(bg, TaskFilter{Query: q(t, "task", "with:\"Alex Rivera\"")})); len(list) != 1 {
		t.Fatal("search index not updated after merge")
	}
	for _, it := range must[[]domain.InboxItem](t)(s.Inbox(bg)) {
		if strings.Contains(it.Key, dupe.ID) || it.Person.ID == dupe.ID {
			t.Fatalf("inbox still references merged contact: %+v", it)
		}
	}
	if e := s.db.QueryRow("PRAGMA foreign_key_check").Scan(new(string), new(string), new(string), new(string)); e == nil {
		t.Fatal("foreign key violations after merge")
	}
	for _, bad := range [][2]string{{keep.ID, keep.ID}, {keep.ID, dupe.ID}} {
		if _, e := s.MergeContacts(bg, bad[0], bad[1]); e == nil {
			t.Fatal("invalid merge accepted", bad)
		}
	}
	// Archiving and deleting a contact keeps its CRM links readable.
	merged.Archived = true
	merged = must[[]domain.Contact](t)(s.SaveContacts(bg, []domain.Contact{merged}, "archive"))[0]
	merged.Deleted = true
	must[[]domain.Contact](t)(s.SaveContacts(bg, []domain.Contact{merged}, "delete"))
	if got := must[domain.Task](t)(s.Task(bg, task.ID)); len(got.Entities) != 1 || got.Entities[0].Name != "Alex Rivera" {
		t.Fatal("link lost after delete", got.Entities)
	}
	if follow := must[[]domain.FollowUp](t)(s.FollowUps(bg)); len(follow) != 0 {
		t.Fatal("deleted contact still scheduled for follow-up")
	}
	for _, it := range must[[]domain.InboxItem](t)(s.Inbox(bg)) {
		if it.Kind == "followup-suggested" || it.Kind == "date" {
			t.Fatalf("deleted contact surfaced: %+v", it)
		}
	}
}

func TestMigrationFromPhaseTwoPreservesData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "phase2.db")
	original := migrations
	migrations = original[:2]
	s, e := Open(bg, path)
	migrations = original
	if e != nil {
		t.Fatal(e)
	}
	var tables int
	_ = s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='tasks'").Scan(&tables)
	if tables != 0 {
		t.Fatal("fixture is not a Phase 2 database")
	}
	c := domain.NewContact("person")
	c.DisplayName = "Phase Two"
	c.Notes = "kept"
	c.Methods = []domain.Method{{Kind: "email", Value: "two@example.com"}}
	c.Memberships = []domain.Membership{{Organization: "Old Org"}}
	saved := must[[]domain.Contact](t)(s.SaveContacts(bg, []domain.Contact{c}, "create"))[0]
	if e = s.Put(bg, "pane-widths", `{"Left":0.3,"Right":0.2}`); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = Open(bg, path)
	if e != nil {
		t.Fatal("upgrade failed:", e)
	}
	defer func() { _ = s.Close() }()
	got := must[domain.Contact](t)(s.Contact(bg, saved.ID))
	if got.Notes != "kept" || got.Revision != saved.Revision || len(got.Methods) != 1 || got.Memberships[0].Organization != "Old Org" {
		t.Fatalf("%+v", got)
	}
	if v, _ := s.Get(bg, "pane-widths"); !strings.Contains(v, "0.3") {
		t.Fatal("preferences lost")
	}
	if len(must[[]domain.History](t)(s.History(bg, saved.ID))) != 1 {
		t.Fatal("history lost")
	}
	var version int
	_ = s.db.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&version)
	if version != 3 {
		t.Fatal(version)
	}
	// New features work against the upgraded data.
	must[domain.Task](t)(s.SaveTask(bg, domain.Task{Title: "After upgrade", Entities: []domain.Ref{{ID: saved.ID}}}))
	if d := must[domain.Dossier](t)(s.Dossier(bg, saved.ID)); d.OpenTaskCount != 1 {
		t.Fatal(d)
	}
	if e = s.db.QueryRow("PRAGMA foreign_key_check").Scan(new(string), new(string), new(string), new(string)); e == nil {
		t.Fatal("foreign key violations after upgrade")
	}
	// A failing later migration rolls back completely and leaves version 3 intact.
	migrations = append(append([]string{}, original...), "CREATE TABLE half_done(id INTEGER); THIS IS NOT SQL;")
	e = s.migrate(bg)
	migrations = original
	if e == nil {
		t.Fatal("broken migration accepted")
	}
	_ = s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='half_done'").Scan(&tables)
	if tables != 0 {
		t.Fatal("partial migration committed")
	}
}
