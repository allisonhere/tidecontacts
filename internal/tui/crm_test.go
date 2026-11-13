package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/config"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

var fixedNow = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) // a Friday

// crmApp opens the application on a real store with a fixed clock.
func crmApp(t *testing.T, w, h int) (Model, *sqlite.Store) {
	t.Helper()
	old := clock
	clock = func() time.Time { return fixedNow }
	t.Cleanup(func() { clock = old })
	s, e := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	s.SetClock(func() time.Time { return fixedNow })
	t.Cleanup(func() { _ = s.Close() })
	m := sized(w, h)
	n, cmd := m.Update(Ready{Config: config.Defaults(), Repo: s, Context: context.Background()})
	return deliver(n.(Model), cmd), s
}

// do sends keys one at a time and settles the resulting storage work.
func do(m Model, keys ...string) Model {
	for _, k := range keys {
		var cmd tea.Cmd
		switch k {
		case "f2":
			m, cmd = press(m, tea.KeyF2)
		case "left":
			m, cmd = press(m, tea.KeyLeft)
		case "right":
			m, cmd = press(m, tea.KeyRight)
		default:
			m, cmd = key(m, k)
		}
		if m.crm.prompt != nil && k != "enter" || m.crm.form != nil && k != "f2" {
			continue // typing: do not wait on cursor blink timers
		}
		m = deliver(m, cmd)
	}
	return m
}

func seedPeople(t *testing.T, s *sqlite.Store) (domain.Contact, domain.Contact) {
	t.Helper()
	alex := domain.NewContact("person")
	alex.DisplayName, alex.Birthday = "Alex Rivera", "--11-20"
	alex.Memberships = []domain.Membership{{Organization: "Riverside Studio", Title: "Engineer", Current: true}}
	sam := domain.NewContact("person")
	sam.DisplayName = "Sam Lee"
	sam.Relationships = []domain.Relationship{{TargetID: alex.ID, Type: "mentor"}}
	saved, e := s.SaveContacts(context.Background(), []domain.Contact{alex, sam}, "seed")
	if e != nil {
		t.Fatal(e)
	}
	return saved[0], saved[1]
}

func frame(m Model) string { return ansi.Strip(m.View()) }

func TestTaskCaptureLifecycleAndInputOwnership(t *testing.T) {
	m, s := crmApp(t, 120, 36)
	seedPeople(t, s)
	m = do(m, "5")
	if m.state.Active != wsTasks || m.viewName() != "Today" {
		t.Fatal(m.state.Active, m.viewName())
	}
	// Quick capture: one line, no form. Every printable key belongs to the prompt.
	m = do(m, "n")
	if m.crm.prompt == nil {
		t.Fatal("no capture prompt")
	}
	m = do(m, `Send q1 proposal? with:"Alex Rivera" due:today pri:high tag:sales`)
	if m.state.Active != wsTasks || m.overlay != none || !strings.Contains(m.crm.prompt.input.Value(), "q1 proposal?") {
		t.Fatal("keys leaked out of the prompt")
	}
	m = do(m, "enter")
	row, ok := m.listRow()
	if !ok || m.crm.err != "" || !strings.Contains(row.text, "Send q1 proposal? · Alex Rivera") || row.tag != "[>]" || !strings.Contains(row.meta, "!!!") {
		t.Fatalf("captured task missing: %q %+v", m.crm.err, row)
	}
	task := row.data.(domain.Task)
	if task.Due != "2026-11-13" || task.Status != "scheduled" || task.Tags[0] != "sales" {
		t.Fatalf("%+v", task)
	}
	// A bad capture stays in the prompt with an explanation and writes nothing.
	m = do(m, "n", "Broken due:someday", "enter")
	if m.crm.prompt == nil || m.crm.prompt.err == "" {
		t.Fatal("invalid capture accepted")
	}
	m, _ = press(m, tea.KeyEsc)
	if m.crm.prompt != nil {
		t.Fatal("esc did not close prompt")
	}
	// Priority, status, snooze and reschedule from the list.
	m = do(m, "p")
	if got, _ := s.Task(context.Background(), task.ID); got.Priority != 0 {
		t.Fatal("priority did not cycle", got.Priority)
	}
	m = do(m, ">")
	if got, _ := s.Task(context.Background(), task.ID); got.Status != "waiting" {
		t.Fatal(got.Status)
	}
	m = do(m, "S", "+1w 09:30", "enter")
	if got, _ := s.Task(context.Background(), task.ID); got.Due != "2026-11-20" || got.DueTime != "09:30" {
		t.Fatal(got.Due, got.DueTime)
	}
	if _, ok = m.listRow(); ok {
		t.Fatal("rescheduled task still in Today")
	}
	m = do(m, "]")
	if m.viewName() != "Upcoming" || !strings.Contains(frame(m), "Fri 2026-11-20") {
		t.Fatal(frame(m))
	}
	m = do(m, "z", "enter") // default: tomorrow
	if got, _ := s.Task(context.Background(), task.ID); got.SnoozedUntil != "2026-11-14" {
		t.Fatal(got.SnoozedUntil)
	}
	// Full form: fields, choice cycling, multiline description, save.
	m = do(m, "e")
	if m.crm.form == nil || m.crm.form.fields[0].input.Value() != "Send q1 proposal?" {
		t.Fatal("edit form did not open on the fresh record")
	}
	m = do(m, " again", "tab", "right")
	if m.crm.form.fields[1].input.Value() != "completed" {
		t.Fatal("choice did not cycle:", m.crm.form.fields[1].input.Value())
	}
	m = do(m, "left")
	for m.crm.form.fields[m.crm.form.selected].key != "description" {
		m, _ = press(m, tea.KeyTab)
	}
	m = do(m, "line one")
	m, _ = press(m, tea.KeyEnter)
	m = do(m, "line two", "f2")
	got, _ := s.Task(context.Background(), task.ID)
	if m.crm.form != nil || got.Title != "Send q1 proposal? again" || got.Description != "line one\nline two" || got.Status != "waiting" || len(got.Entities) != 1 {
		t.Fatalf("form save: %q %+v", m.crm.err, got)
	}
	// Cancelling a dirty form asks first and changes nothing.
	m = do(m, "e", "zzz")
	m, _ = press(m, tea.KeyEsc)
	if m.crm.form == nil || m.crm.form.confirm == "" {
		t.Fatal("no discard confirmation")
	}
	m = do(m, "y")
	if after, _ := s.Task(context.Background(), task.ID); m.crm.form != nil || after.Title != got.Title {
		t.Fatal("cancelled edit was saved")
	}
	// Complete (logs a timeline entry because a person is linked), reopen, duplicate, delete with confirmation.
	m = do(m, "]", "]")
	if m.viewName() != "All" {
		t.Fatal(m.viewName())
	}
	m = do(m, "c")
	acts, _ := s.Activities(context.Background(), app.ActivityFilter{})
	if after, _ := s.Task(context.Background(), task.ID); after.Status != "completed" || len(acts) != 1 || acts[0].Type != "task-completion" {
		t.Fatal(after.Status, acts)
	}
	m = do(m, "R", "y")
	all, _ := s.Tasks(context.Background(), app.TaskFilter{})
	if len(all) != 2 {
		t.Fatal("duplicate missing", len(all))
	}
	m = do(m, "D")
	if m.crm.confirm == nil {
		t.Fatal("delete without confirmation")
	}
	m = do(m, "n")
	if all, _ = s.Tasks(context.Background(), app.TaskFilter{}); len(all) != 2 || m.crm.confirm != nil {
		t.Fatal("declined delete removed a task")
	}
	m = do(m, "D", "y")
	if all, _ = s.Tasks(context.Background(), app.TaskFilter{}); len(all) != 1 {
		t.Fatal("confirmed delete kept the task")
	}
	// By-person grouping and filtering with the shared grammar.
	m = do(m, "]")
	if m.viewName() != "By person" || !strings.Contains(frame(m), "Alex Rivera (1)") {
		t.Fatal(frame(m))
	}
	m = do(m, "/", "nomatch")
	m, cmd := press(m, tea.KeyEnter)
	m = deliver(m, cmd)
	if _, ok = m.listRow(); ok || !strings.Contains(frame(m), "Nothing matches") {
		t.Fatal("filter not applied")
	}
	m = do(m, "/", " bogus:x")
	if !strings.Contains(m.crm.err, "unknown filter") {
		t.Fatal(m.crm.err)
	}
}

func TestActivitiesDossierAndBackNavigation(t *testing.T) {
	m, s := crmApp(t, 120, 36)
	alex, _ := seedPeople(t, s)
	m = deliver(m, m.refreshContacts())
	if m.contacts.contact.ID != alex.ID {
		t.Fatal("expected Alex selected")
	}
	// Log an activity for the selected person without leaving People.
	m = do(m, "A", "Coffee type:meeting on:yesterday at:10:30 project:Harbor", "enter")
	if !strings.Contains(m.contacts.err, "project") {
		t.Fatal("unknown project should be reported:", m.contacts.err)
	}
	if _, e := s.SaveProject(context.Background(), domain.Project{Name: "Harbor"}); e != nil {
		t.Fatal(e)
	}
	m = do(m, "A", "Coffee type:meeting on:yesterday at:10:30 project:Harbor", "enter", "T", "Send recap due:tomorrow", "enter", "F", "2w", "enter", "d", "First met 2019-03-03", "enter")
	// The dossier is one keypress back from Overview.
	m = do(m, "[", "[")
	if profileViews[m.contacts.view] != "Dossier" || m.focus != 1 {
		t.Fatal(profileViews[m.contacts.view], m.focus)
	}
	out := frame(m)
	for _, want := range []string{"Follow-up", "Next: 2026-11-26", "last recorded meeting on 2026-11-12", "Open tasks (1)", "Send recap", "Connections", "Sam Lee", "Riverside Studio", "First met", "Birthday"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dossier lacks %q:\n%s", want, out)
		}
	}
	// Complete the task straight from the dossier.
	if row, ok := m.target(); !ok || row.kind != "task" {
		t.Fatalf("cursor should rest on the first task: %+v", row)
	}
	m = do(m, "c")
	if !strings.Contains(frame(m), "Open tasks (0)") || !strings.Contains(frame(m), "Recent commitments") {
		t.Fatal(frame(m))
	}
	m = do(m, "]")
	if profileViews[m.contacts.view] != "Timeline" || !strings.Contains(frame(m), "10:30 mtg Coffee") {
		t.Fatal(frame(m))
	}
	// Follow the activity, then a project from it, then come all the way back.
	for {
		row, _ := m.target()
		if row.ref.Kind == "activity" && row.text == "Coffee" {
			break
		}
		before := m.crm.dossierCursor
		if m = do(m, "j"); m.crm.dossierCursor == before {
			t.Fatal("Coffee not reachable")
		}
	}
	m = do(m, "enter")
	if m.state.Active != wsActivities || m.crm.detailTitle != "Coffee" || m.focus != 1 || len(m.crm.nav) != 1 {
		t.Fatal(m.state.Active, m.crm.detailTitle, m.focus)
	}
	for m.crm.detail[m.crm.cursor].ref.Kind != "project" {
		m = do(m, "j")
	}
	m = do(m, "enter")
	if m.state.Active != wsProjects || m.crm.detailTitle != "Harbor" || len(m.crm.nav) != 2 || !strings.Contains(frame(m), "esc ← Coffee") {
		t.Fatal(m.state.Active, m.crm.detailTitle, frame(m))
	}
	m, cmd := press(m, tea.KeyEsc)
	m = deliver(m, cmd)
	if row, _ := m.listRow(); m.state.Active != wsActivities || row.text != "Coffee · Alex Rivera" || m.focus != 1 {
		t.Fatal("back to activity lost position", m.state.Active, row.text, m.focus)
	}
	m, cmd = press(m, tea.KeyEsc)
	m = deliver(m, cmd)
	if m.state.Active != wsPeople || m.contacts.contact.ID != alex.ID || profileViews[m.contacts.view] != "Timeline" || m.focus != 1 || len(m.crm.nav) != 0 {
		t.Fatal("back to contact lost context", m.state.Active, m.contacts.view, m.focus)
	}
	// With the stack empty, Esc is the ordinary "focus the list".
	m, _ = press(m, tea.KeyEsc)
	if m.focus != 0 || m.state.Active != wsPeople {
		t.Fatal("esc with empty history")
	}

	// Timeline workspace: grouping, type views, convert to task, edit, provenance.
	if _, e := s.UpsertExternalActivity(context.Background(), domain.Activity{Type: "email", Title: "Re: invoice", SourceApp: "tidemail", ExternalRef: "<1@x>", OccurredAt: fixedNow.Add(-time.Hour)}, nil); e != nil {
		t.Fatal(e)
	}
	m = do(m, "4")
	out = frame(m)
	for _, want := range []string{"Fri 2026-11-13 · today", "11:00 mail Re: invoice", "↗ tidemail", "Thu 2026-11-12 · yesterday", "10:30 mtg Coffee · Alex Rivera", "done Completed: Send recap"} {
		if !strings.Contains(out, want) {
			t.Fatalf("timeline lacks %q:\n%s", want, out)
		}
	}
	m = do(m, "k", "e")
	if m.crm.form != nil || !strings.Contains(m.crm.err, "tidemail") {
		t.Fatal("external activity must not open for editing")
	}
	m = do(m, "]")
	if m.viewName() != "Meetings" {
		t.Fatal(m.viewName())
	}
	if row, _ := m.listRow(); row.text != "Coffee · Alex Rivera" {
		t.Fatal(row)
	}
	m = do(m, "t")
	if m.crm.prompt == nil || m.crm.prompt.input.Value() != "Follow up: Coffee" {
		t.Fatal("convert-to-task prompt")
	}
	m = do(m, "enter")
	tasks, _ := s.Tasks(context.Background(), app.TaskFilter{View: "open"})
	if len(tasks) != 1 || tasks[0].Entities[0].ID != alex.ID || tasks[0].Projects[0].Name != "Harbor" {
		t.Fatalf("%+v", tasks)
	}
	if full, _ := s.Task(context.Background(), tasks[0].ID); len(full.Activities) != 1 {
		t.Fatal("converted task lost its source activity")
	}
	m = do(m, "e")
	if m.crm.form == nil {
		t.Fatal(m.crm.err)
	}
	m = do(m, "tab", " and cake", "f2")
	if a, _ := s.Activities(context.Background(), app.ActivityFilter{Query: search.Query{Terms: []string{"cake"}}}); len(a) != 1 || len(a[0].Entities) != 1 {
		t.Fatal("activity edit lost links or text", a, m.crm.err)
	}
}

func TestProjectsPipelinesInboxAndCollections(t *testing.T) {
	m, s := crmApp(t, 150, 40)
	alex, _ := seedPeople(t, s)
	ctx := context.Background()
	// Projects: create through the form, add a milestone and a task, walk the detail views.
	m = do(m, "6", "n", "Harbor", "tab", "tab", "tab", "tab", "+2w", "tab", "Alex Rivera (lead)", "f2")
	projects, _ := s.Projects(ctx, "", search.Query{})
	if len(projects) != 1 || projects[0].Target != "2026-11-27" || projects[0].Members[0].Role != "lead" || m.crm.detailTitle != "Harbor" {
		t.Fatalf("%q %+v", m.crm.err, projects)
	}
	m = do(m, "M", "Signed brief due:fri", "enter", "T", "Draft plan pri:high", "enter")
	for i, name := range crmViews[wsProjects] {
		if m.list().view != i || !strings.Contains(frame(m), name+" · Harbor") {
			t.Fatalf("project view %s: %d", name, m.list().view)
		}
		switch name {
		case "People":
			if !strings.Contains(frame(m), "Alex Rivera") {
				t.Fatal(frame(m))
			}
		case "Tasks":
			if !strings.Contains(frame(m), "[?] Draft plan") {
				t.Fatal(frame(m))
			}
		case "Milestones":
			if !strings.Contains(frame(m), "[ ] Signed brief") {
				t.Fatal(frame(m))
			}
			m = do(m, "c")
			if p, _ := s.Project(ctx, projects[0].ID); p.Milestones[0].DoneAt != "2026-11-13" || p.Milestones[0].Due != "2026-11-20" {
				t.Fatalf("%+v", p.Milestones)
			}
		}
		m = do(m, "]")
	}
	m, _ = press(m, tea.KeyEsc)
	m = do(m, ">")
	if p, _ := s.Project(ctx, projects[0].ID); p.Status != "on-hold" {
		t.Fatal(p.Status)
	}

	// Pipelines: build one, add items, move them entirely by keyboard.
	m = do(m, "7")
	if !strings.Contains(frame(m), "No pipelines yet") {
		t.Fatal(frame(m))
	}
	m = do(m, ":", "new pipeline", "enter", "Job applications", "tab", "tab")
	for i := 0; i < 20; i++ {
		m, _ = press(m, tea.KeyBackspace)
	}
	m = do(m, "Applied, Interview, Offer", "f2")
	pipes, _ := s.Pipelines(ctx, false)
	if len(pipes) != 1 || len(pipes[0].Stages) != 3 || m.focus != 1 {
		t.Fatalf("%q %+v", m.crm.err, pipes)
	}
	m = do(m, "n", "Riverside Studio", "tab", "tab", "Send portfolio", "f2", "n", "Harbor Co", "f2")
	if len(m.crm.board[0]) != 2 {
		t.Fatal("items missing", m.crm.err)
	}
	m = do(m, "k")
	out := frame(m)
	if !strings.Contains(out, "▸ Applied") || !strings.Contains(out, "Interview") || !strings.Contains(out, "Offer") || !strings.Contains(out, "> Riverside Studio") {
		t.Fatal(out)
	}
	first, _ := m.boardItem()
	m = do(m, ">", ">")
	if it, _ := s.Item(ctx, first.ID); it.Stage != "Offer" || len(it.History) != 3 || m.crm.col != 2 {
		t.Fatalf("%+v", it)
	}
	m = do(m, ">")
	if !strings.Contains(m.feedback, "edge") {
		t.Fatal("moving past the last stage should say so")
	}
	m = do(m, "<", "h", "j")
	if it, _ := m.boardItem(); m.crm.col != 0 || it.Title != "Harbor Co" {
		t.Fatal(m.crm.col, it.Title)
	}
	m = do(m, "l", "enter")
	if m.focus != 2 || !strings.Contains(frame(m), "Stage history") || !strings.Contains(frame(m), "Applied → Interview") {
		t.Fatal(m.focus, frame(m))
	}
	m, _ = press(m, tea.KeyEsc)
	m.focus = 1
	m = do(m, ":", "add pipeline stage", "enter", "Screening", "enter")
	if pipes, _ = s.Pipelines(ctx, false); len(pipes[0].Stages) != 4 || pipes[0].Stages[2].Name != "Screening" {
		t.Fatalf("%+v", pipes[0].Stages)
	}
	// A narrow terminal shows a stage list with the focused stage's items.
	narrow, _ := m.Update(tea.WindowSizeMsg{Width: 44, Height: 20})
	m = narrow.(Model)
	m.focus = 1
	out = frame(m)
	if !strings.Contains(out, "> Interview") || !strings.Contains(out, "Riverside Studio") || strings.Contains(out, "▸") {
		t.Fatal(out)
	}
	m = do(m, "h")
	if m.crm.col != 0 || !strings.Contains(frame(m), "> Applied") {
		t.Fatal(frame(m))
	}
	wide, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = wide.(Model)

	// People Inbox: explained entries, act in place, dismissal sticks.
	if _, e := s.SaveTask(ctx, domain.Task{Title: "Overdue thing", Status: "next", Due: "2026-11-01", Entities: []domain.Ref{{ID: alex.ID}}}); e != nil {
		t.Fatal(e)
	}
	if e := s.SetFlag(ctx, alex.ID, true, "verify email"); e != nil {
		t.Fatal(e)
	}
	m = do(m, ":", "review people inbox", "enter")
	if m.state.Active != wsInsights || m.viewName() != "Inbox" {
		t.Fatal(m.state.Active, m.viewName())
	}
	out = frame(m)
	for _, want := range []string{"Overdue", "late Overdue thing", "Why this is here", "Due 2026-11-01, 12 days overdue", "date Alex Rivera", "flag Alex Rivera"} {
		if !strings.Contains(out, want) {
			t.Fatalf("inbox lacks %q:\n%s", want, out)
		}
	}
	count := func() int { items, _ := s.Inbox(ctx); return len(items) }
	before := count()
	m = do(m, "x")
	if count() != before-1 || strings.Contains(frame(m), "late Overdue thing") {
		t.Fatal("dismiss failed")
	}
	m = deliver(m, m.loadCRM())
	if strings.Contains(frame(m), "late Overdue thing") {
		t.Fatal("dismissed entry came back on refresh")
	}
	m = do(m, "v")
	if row, _ := m.listRow(); !row.muted {
		t.Fatal("review did not mark the entry")
	}
	m = do(m, "z", "enter")
	if count() != before-2 {
		t.Fatal("snooze failed", count(), before)
	}
	// Radar and its settings.
	m = do(m, "]")
	if m.viewName() != "Radar" || !strings.Contains(frame(m), "Open commitments") || !strings.Contains(frame(m), "Upcoming important dates") {
		t.Fatal(frame(m))
	}
	m = do(m, ":", "radar settings", "enter")
	if m.crm.form == nil {
		t.Fatal("no radar form")
	}
	m = do(m, "tab", "right", "f2")
	if cfg, _ := s.RadarConfig(ctx); !cfg.Disabled["commitments"] {
		t.Fatal("signal not disabled")
	}
	if strings.Contains(frame(m), "Open commitments") {
		t.Fatal("disabled signal still shown")
	}

	// Smart collections: save a live filter, see it update, open it and come back.
	m = do(m, "5", "]", "]", "]", "/", "pri:high")
	m, cmd := press(m, tea.KeyEnter)
	m = deliver(m, cmd)
	m = do(m, "C", "Urgent work", "enter", ":", "open smart collections", "enter")
	if m.viewName() != "Collections" || !strings.Contains(frame(m), "Urgent work") || !strings.Contains(frame(m), "1 result(s) right now") {
		t.Fatal(m.crm.err, frame(m))
	}
	if _, e := s.SaveTask(ctx, domain.Task{Title: "Another urgent", Priority: 3, Status: "next"}); e != nil {
		t.Fatal(e)
	}
	m = deliver(m, m.loadCRM())
	if !strings.Contains(frame(m), "2 result(s) right now") {
		t.Fatal("collection is not live")
	}
	m = do(m, "f")
	if cols, _ := s.Collections(ctx); !cols[0].Pinned {
		t.Fatal("pin")
	}
	m = do(m, "enter")
	if m.state.Active != wsTasks || m.queries[wsTasks] != "pri:high" || len(m.list().rows) < 2 {
		t.Fatal("collection did not open", m.state.Active, m.queries[wsTasks])
	}
	names := m.viewNames()
	if names[len(names)-1] != "Urgent work" {
		t.Fatal("pinned collection is not a task view", names)
	}
	m, cmd = press(m, tea.KeyEsc)
	m = deliver(m, cmd)
	if m.state.Active != wsInsights || m.viewName() != "Collections" {
		t.Fatal("back from collection")
	}
}

func TestRelationshipExplorerLinkingRemindersAndMerge(t *testing.T) {
	m, s := crmApp(t, 150, 40)
	alex, sam := seedPeople(t, s)
	ctx := context.Background()
	m = do(m, "3")
	if row, _ := m.listRow(); row.id != alex.ID {
		t.Fatal(row)
	}
	out := frame(m)
	for _, want := range []string{"Tree · Alex Rivera", "├─ mentor ← Sam Lee", "└─ member of Riverside Studio"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tree lacks %q:\n%s", want, out)
		}
	}
	// Enter on a node re-roots the explorer; Esc returns.
	m = do(m, "enter", "enter")
	if row, _ := m.listRow(); row.id != sam.ID || len(m.crm.nav) != 1 {
		t.Fatal("did not re-root on Sam", row.text)
	}
	m, cmd := press(m, tea.KeyEsc)
	m = deliver(m, cmd)
	if row, _ := m.listRow(); row.id != alex.ID {
		t.Fatal("back")
	}
	// Add a relationship with the searchable picker and a custom label.
	m.focus = 0
	m = do(m, "r", "sam")
	if m.crm.picker == nil || len(m.crm.picker.options) != 1 || m.crm.picker.options[0].ID != sam.ID {
		t.Fatalf("picker: %+v", m.crm.picker)
	}
	m = do(m, "enter", "climbing partner", "enter")
	if c, _ := s.Contact(ctx, alex.ID); len(c.Relationships) != 1 || c.Relationships[0].Type != "climbing partner" {
		t.Fatalf("%q %+v", m.crm.err, c.Relationships)
	}
	for _, view := range []string{"People", "Organizations", "Projects", "Profile"} {
		m = do(m, "]")
		if !strings.Contains(frame(m), view+" · Alex Rivera") {
			t.Fatal(view)
		}
	}
	if out = frame(m); !strings.Contains(out, "climbing partner · Sam Lee") || !strings.Contains(out, "Birthday") || !strings.Contains(out, "No schedule") {
		t.Fatal(out)
	}
	m = do(m, "x")
	if m.crm.confirm == nil {
		t.Fatal("removing a relationship needs confirmation")
	}
	m = do(m, "y")
	if c, _ := s.Contact(ctx, alex.ID); len(c.Relationships) != 0 {
		t.Fatal("relationship not removed")
	}
	m = do(m, "!")
	if conn, _ := s.Connections(ctx, alex.ID); !conn.Flagged {
		t.Fatal("flag")
	}
	// Link a person to a task through the picker, then unlink from the detail pane.
	task, _ := s.SaveTask(ctx, domain.Task{Title: "Plan trip", Status: "next"})
	m = do(m, "5", "]", "]", "]", "L")
	m = do(m, "enter")
	m = do(m, "alex")
	m = do(m, "enter")
	if got, _ := s.Task(ctx, task.ID); len(got.Entities) != 1 || got.Entities[0].ID != alex.ID {
		t.Fatalf("%q %+v", m.crm.err, got.Entities)
	}
	m = do(m, "enter", "x")
	if got, _ := s.Task(ctx, task.ID); len(got.Entities) != 0 {
		t.Fatal("unlink failed", m.feedback)
	}
	// Reminders: add, see the due count, snooze, dismiss.
	m = do(m, "b", "Stretch at:11:00 every:day", "enter")
	m = deliver(m, m.loadDue())
	if m.crm.due != 1 || !strings.Contains(frame(m), "1 reminder(s) due") {
		t.Fatal(m.crm.due, m.crm.err)
	}
	m = do(m, ":", "open reminders", "enter")
	if !strings.Contains(frame(m), "bell Stretch") || !strings.Contains(frame(m), "Repeats daily") {
		t.Fatal(frame(m))
	}
	m = do(m, "c")
	list, _ := s.Reminders(ctx, false)
	if len(list) != 1 || list[0].At.Format("2006-01-02 15:04") != "2026-11-14 11:00" || m.crm.due != 0 {
		t.Fatalf("recurring reminder should advance once: %+v due=%d", list, m.crm.due)
	}
	// Merge: mark the duplicate, select the keeper, confirm.
	dupe := domain.NewContact("person")
	dupe.DisplayName = "Alexander Rivera"
	if _, e := s.SaveContacts(ctx, []domain.Contact{dupe}, "seed"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.SaveTask(ctx, domain.Task{Title: "For the duplicate", Entities: []domain.Ref{{ID: dupe.ID}}}); e != nil {
		t.Fatal(e)
	}
	m = do(m, "1", ":", "merge marked", "enter")
	if m.crm.confirm != nil || !strings.Contains(m.contacts.err, "Mark exactly one") {
		t.Fatal("merge without a marked contact")
	}
	m = do(m, "j", " ", "k", ":", "merge marked", "enter")
	if m.crm.confirm == nil || !strings.Contains(m.crm.confirm.text, "Merge “Alexander Rivera” into “Alex Rivera”") {
		t.Fatalf("%+v %s", m.crm.confirm, m.contacts.err)
	}
	m = do(m, "y")
	if rows, _ := s.List(ctx, "person", search.Query{}, "name"); len(rows) != 2 {
		t.Fatal("merge did not remove the duplicate", len(rows), m.contacts.err)
	}
	if tasks, _ := s.Tasks(ctx, app.TaskFilter{EntityID: alex.ID}); len(tasks) != 1 {
		t.Fatal("merged contact's task did not follow")
	}
}

func TestCRMRenderingIsBoundedAndStable(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(old)
	m, s := crmApp(t, 120, 36)
	alex, sam := seedPeople(t, s)
	ctx := context.Background()
	project, _ := s.SaveProject(ctx, domain.Project{Name: "Harbor 界 redesign with a very long name that must be clipped", Target: "2026-11-01", Members: []domain.Ref{{ID: alex.ID}},
		Milestones: []domain.Milestone{{Title: "Brief", Due: "2026-11-05"}}, Notes: strings.Repeat("long note ", 60)})
	for i := 0; i < 40; i++ {
		_, _ = s.SaveTask(ctx, domain.Task{Title: fmt.Sprintf("Task %02d 👩‍💻 %s", i, strings.Repeat("x", i*3)), Status: domain.TaskStatuses[i%4], Due: domain.AddDays("2026-11-01", i), Priority: i % 4,
			Entities: []domain.Ref{{ID: alex.ID}}, Projects: []domain.Ref{{ID: project.ID}}})
		_, _ = s.SaveActivity(ctx, domain.Activity{Type: domain.ActivityTypes[i%7], Title: fmt.Sprintf("Activity %02d", i), Body: strings.Repeat("body text ", i),
			OccurredAt: fixedNow.Add(-time.Duration(i) * 7 * time.Hour), Entities: []domain.Ref{{ID: alex.ID}, {ID: sam.ID}}, Projects: []domain.Ref{{ID: project.ID}}})
	}
	pipe, _ := s.SavePipeline(ctx, domain.Pipeline{Name: "Partnerships", Stages: []domain.Stage{{Name: "Idea"}, {Name: "Talking"}, {Name: "Drafting"}, {Name: "Signed"}, {Name: "Live"}, {Name: "Closed"}}})
	for i := 0; i < 14; i++ {
		_, _ = s.SaveItem(ctx, domain.PipelineItem{PipelineID: pipe.ID, StageID: pipe.Stages[i%6].ID, Title: fmt.Sprintf("Item %02d with a long title to clip", i), Entities: []domain.Ref{{ID: sam.ID}}})
	}
	_, _ = s.SaveReminder(ctx, domain.Reminder{Title: "Ping", At: fixedNow.Add(-time.Hour), Entity: domain.Ref{ID: alex.ID}})
	_ = s.SetFollowUp(ctx, domain.FollowUp{Entity: domain.Ref{ID: sam.ID}, NextDate: "2026-11-10"})
	_, _ = s.SaveCollection(ctx, domain.Collection{Name: "Open Harbor", Entity: "task", Query: "is:open project:Harbor", Group: "status", Pinned: true})
	sizes := [][2]int{{160, 45}, {110, 32}, {80, 24}, {60, 20}, {40, 12}, {20, 6}, {1, 1}}
	golden := map[string]bool{"160x45": true, "110x32": true, "80x24": true, "60x20": true}
	for ws := wsRelationships; ws <= wsInsights; ws++ {
		m = do(m, fmt.Sprint(ws+1))
		views := max(1, len(m.viewNames()))
		for view := 0; view < views; view++ {
			for _, size := range sizes {
				next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				m = next.(Model)
				for focus := 0; focus < 3; focus++ {
					m.focus = min(focus, m.focusCount()-1)
					for _, k := range []string{"j", "j", "k"} {
						m = do(m, k)
					}
					checkBounds(t, m.View(), size[0], size[1])
				}
				m.focus = 1
				name := fmt.Sprintf("%dx%d", size[0], size[1])
				if view == 0 && golden[name] {
					path := filepath.Join("testdata", fmt.Sprintf("crm-%s-%s.txt", domain.Workspaces()[ws].ID, name))
					out := frame(m)
					if os.Getenv("UPDATE_GOLDEN") == "1" {
						if e := os.WriteFile(path, []byte(out), 0644); e != nil {
							t.Fatal(e)
						}
					}
					if want, e := os.ReadFile(path); e != nil || string(want) != out {
						t.Fatalf("frame %s mismatch: %v", path, e)
					}
				}
			}
			next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
			m = next.(Model)
			m.focus = 0
			m = do(m, "]")
		}
	}
	// Overlays and forms stay inside the terminal at every size too.
	m = do(m, "5")
	for _, size := range sizes {
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = next.(Model)
		for _, open := range []string{"n", "L", "D", "e"} {
			m = do(m, open)
			checkBounds(t, m.View(), size[0], size[1])
			if f := m.crm.form; f != nil {
				for i := range f.fields {
					f.selected = i
					checkBounds(t, m.View(), size[0], size[1])
				}
			}
			m.crm.prompt, m.crm.picker, m.crm.confirm, m.crm.form = nil, nil, nil, nil
		}
	}
	// The contact dossier views render within bounds as well.
	m = do(m, "1")
	for _, size := range sizes {
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = next.(Model)
		for _, view := range []int{7, 8} {
			m.contacts.view, m.focus = view, 1
			m = deliver(m, m.loadDossier())
			m = do(m, "j", "j")
			checkBounds(t, m.View(), size[0], size[1])
		}
	}
}

// TestListQueriesStayFastAtScale measures instead of assuming: 10,000 contacts,
// 5,000 tasks and 5,000 activities must keep the interactive queries quick.
func TestListQueriesStayFastAtScale(t *testing.T) {
	if testing.Short() {
		t.Skip("scale test")
	}
	m, s := crmApp(t, 120, 36)
	ctx := context.Background()
	batch := make([]domain.Contact, 10000)
	for i := range batch {
		batch[i] = domain.NewContact("person")
		batch[i].DisplayName = fmt.Sprintf("Person %05d", i)
	}
	if _, e := s.SaveContacts(ctx, batch, "seed"); e != nil {
		t.Fatal(e)
	}
	project, _ := s.SaveProject(ctx, domain.Project{Name: "Scale"})
	for i := 0; i < 5000; i++ {
		who := []domain.Ref{{ID: batch[i%200].ID}}
		if _, e := s.SaveTask(ctx, domain.Task{Title: fmt.Sprintf("Task %d", i), Status: domain.TaskStatuses[i%4], Due: domain.AddDays("2026-10-01", i%90), Entities: who, Projects: []domain.Ref{{ID: project.ID}}}); e != nil {
			t.Fatal(e)
		}
		if _, e := s.SaveActivity(ctx, domain.Activity{Type: "call", Title: fmt.Sprintf("Call %d", i), OccurredAt: fixedNow.Add(-time.Duration(i) * time.Hour), Entities: who}); e != nil {
			t.Fatal(e)
		}
		if i < 300 {
			_ = s.SetFollowUp(ctx, domain.FollowUp{Entity: who[0], Interval: domain.Interval{Every: 2, Unit: "week"}})
		}
	}
	timed := func(name string, limit time.Duration, fn func()) {
		start := time.Now()
		fn()
		if took := time.Since(start); took > limit {
			t.Errorf("%s took %v (limit %v)", name, took, limit)
		} else {
			t.Logf("%s: %v", name, took)
		}
	}
	timed("timeline page", 500*time.Millisecond, func() { _, _ = s.Activities(ctx, app.ActivityFilter{Limit: 200}) })
	timed("person timeline", 250*time.Millisecond, func() { _, _ = s.Activities(ctx, app.ActivityFilter{EntityID: batch[7].ID, Limit: 200}) })
	timed("today tasks", 500*time.Millisecond, func() { _, _ = s.Tasks(ctx, app.TaskFilter{View: "today"}) })
	timed("task search", 500*time.Millisecond, func() { _, _ = s.Tasks(ctx, app.TaskFilter{Query: search.Query{Terms: []string{"task 49"}}}) })
	timed("dossier", 250*time.Millisecond, func() { _, _ = s.Dossier(ctx, batch[7].ID) })
	timed("inbox", 1500*time.Millisecond, func() { _, _ = s.Inbox(ctx) })
	timed("radar", 1500*time.Millisecond, func() { cfg, _ := s.RadarConfig(ctx); _, _ = s.Radar(ctx, cfg) })
	m = do(m, "5", "]", "]", "]")
	timed("render 2,500-row task list", 100*time.Millisecond, func() { _ = m.View() })
	if len(m.list().rows) < 1000 {
		t.Fatal("expected a large list", len(m.list().rows))
	}
}
