package ext

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
)

func TestRuleWritesAreScopedRevisionCheckedAndCausal(t *testing.T) {
	ctx := t.Context()
	st := newStore(t)
	person := domain.NewContact("person")
	person.DisplayName, person.Tags, person.Notes = "Fixture Contact", []string{"keep"}, "Private notes"
	people, err := st.SaveContacts(ctx, []domain.Contact{person}, "test")
	if err != nil {
		t.Fatal(err)
	}
	s := &Session{ID: "fixture-ext", Repo: st, Granted: NewPermissionSet("contacts.read", "contacts.write")}
	body := map[string]any{"kind": "add_tag", "tag": "priority", "record_kind": "contact", "record_id": person.ID,
		"revision": people[0].Revision, "correlation_id": "run-1", "causation_id": "event-1", "automation_id": "rule-1", "depth": 1}
	if _, e := call(t, s, "records.rule-write", body); e == nil || e.Code != CodePermissionDenied {
		t.Fatalf("narrow write grant not enforced: %v", e)
	}
	s.Granted = NewPermissionSet("contacts.read", "contacts.tags.write")
	if raw, e := call(t, s, "records.rule-page", map[string]any{"kind": "contact", "limit": 1}); e != nil || !strings.Contains(string(raw), `"tags":["keep"]`) || strings.Contains(string(raw), "Private notes") {
		t.Fatalf("contact projection = %s (%v)", raw, e)
	}
	if _, e := call(t, s, "records.rule-write", body); e != nil {
		t.Fatal(e)
	}
	if _, e := call(t, s, "records.rule-write", body); e == nil || e.Code != CodeConflict {
		t.Fatalf("stale tag edit: %v", e)
	}
	got, err := st.Contact(ctx, person.ID)
	if err != nil || strings.Join(got.Tags, ",") != "keep,priority" || got.Notes != "Private notes" {
		t.Fatalf("contact tag update lost data: %+v (%v)", got.Tags, err)
	}
	s.Granted = NewPermissionSet("events.consume", "contacts.read")
	s.durableTypes = []string{"contact.updated"}
	raw, e := call(t, s, "events.next", nil)
	var d app.EventDelivery
	if e != nil || json.Unmarshal(raw, &d) != nil || d.Event.Source != "extension:fixture-ext" || d.Event.AutomationID != "rule-1" || d.Event.CorrelationID != "run-1" || d.Event.CausationID != "event-1" || d.Event.Depth != 1 {
		t.Fatalf("linked event provenance = %s (%v)", raw, e)
	}
}

func TestRuleTaskPagesAreStableAndExcludeArchived(t *testing.T) {
	ctx := t.Context()
	st := newStore(t)
	for _, item := range []domain.Task{{ID: "c", Title: "Third task"}, {ID: "a", Title: "First task", Priority: 3}, {ID: "b", Title: "Archived task", Archived: true}} {
		if _, err := st.SaveTask(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	s := &Session{ID: "fixture-ext", Repo: st, Granted: NewPermissionSet("tasks.read")}
	page := func(after string) []app.RuleRecord {
		t.Helper()
		var rows []app.RuleRecord
		raw, e := call(t, s, "records.rule-page", map[string]any{"kind": "task", "after": after, "limit": 1})
		if e != nil || json.Unmarshal(raw, &rows) != nil {
			t.Fatalf("page after %q = %s (%v)", after, raw, e)
		}
		return rows
	}
	if got := page(""); len(got) != 1 || got[0].ID != "a" || got[0].Revision != 1 || got[0].Priority != 3 {
		t.Fatalf("first page = %+v", got)
	}
	if got := page("a"); len(got) != 1 || got[0].ID != "c" {
		t.Fatalf("second page = %+v", got)
	}
}

func TestRuleCreatesScopedProjectReminderAndNote(t *testing.T) {
	ctx := t.Context()
	st := newStore(t)
	org := domain.NewContact("organization")
	org.DisplayName = "Blue Harbor"
	if _, err := st.SaveContacts(ctx, []domain.Contact{org}, "test"); err != nil {
		t.Fatal(err)
	}
	s := &Session{ID: "fixture-ext", Repo: st}
	create := func(kind, permission, linkKind, linkID, due, status string) string {
		t.Helper()
		body := map[string]any{"kind": kind, "title": "Fixture " + kind, "record_kind": linkKind, "record_id": linkID,
			"due": due, "task_status": status, "correlation_id": "run-1", "causation_id": "event-1", "automation_id": "rule-1", "depth": 3}
		s.Granted = NewPermissionSet("organizations.read", "projects.read")
		if _, e := call(t, s, "records.rule-write", body); e == nil || e.Code != CodePermissionDenied {
			t.Fatalf("%s bypassed write grant: %v", kind, e)
		}
		s.Granted = NewPermissionSet(permission)
		if _, e := call(t, s, "records.rule-write", body); e == nil || e.Code != CodePermissionDenied {
			t.Fatalf("%s bypassed link read grant: %v", kind, e)
		}
		s.Granted = NewPermissionSet(permission, "organizations.read", "projects.read")
		if kind == "reminder" {
			body["due"] = "2026-02-30"
			if _, e := call(t, s, "records.rule-write", body); e == nil || e.Code != CodeInvalid {
				t.Fatalf("invalid reminder date = %v", e)
			}
			body["due"] = due
		}
		raw, e := call(t, s, "records.rule-write", body)
		var out struct{ ID string }
		if e != nil || json.Unmarshal(raw, &out) != nil || out.ID == "" {
			t.Fatalf("%s create = %s (%v)", kind, raw, e)
		}
		return out.ID
	}
	projectID := create("project", "projects.write", "organization", org.ID, "", "")
	p, err := st.Project(ctx, projectID)
	if err != nil || p.Name != "Fixture project" || len(p.Members) != 1 || p.Members[0].ID != org.ID {
		t.Fatalf("project = %+v (%v)", p, err)
	}
	id := create("reminder", "reminders.write", "organization", org.ID, "2027-01-10", "")
	reminders, err := st.Reminders(ctx, false)
	if err != nil || len(reminders) != 1 || reminders[0].ID != id || reminders[0].Title != "Fixture reminder" ||
		reminders[0].Entity.ID != org.ID || reminders[0].At.In(time.Local).Format(time.DateOnly) != "2027-01-10" {
		t.Fatalf("reminders = %+v (%v)", reminders, err)
	}
	noteID := create("note", "activities.write", "project", projectID, "", "")
	note, err := st.Activity(ctx, noteID)
	if err != nil || note.Type != "note" || note.Provenance != domain.Local || note.SourceApp != "extension:fixture-ext" ||
		len(note.Projects) != 1 || note.Projects[0].ID != projectID {
		t.Fatalf("note = %+v (%v)", note, err)
	}
	taskID := create("task", "tasks.write", "project", projectID, "2027-03-15", "waiting")
	task, err := st.Task(ctx, taskID)
	if err != nil || task.Status != "waiting" || task.Due != "2027-03-15" || len(task.Projects) != 1 || task.Projects[0].ID != projectID {
		t.Fatalf("task = %+v (%v)", task, err)
	}
	for _, tc := range []struct{ typ, id string }{
		{"project.created", projectID}, {"reminder.created", id}, {"activity.created", noteID}, {"task.created", taskID},
	} {
		d, err := st.NextEvent(ctx, "reviewer", []string{tc.typ})
		if err != nil || d == nil || d.Event.EntityID != tc.id || d.Event.Source != "extension:fixture-ext" ||
			d.Event.CorrelationID != "run-1" || d.Event.CausationID != "event-1" || d.Event.AutomationID != "rule-1" || d.Event.Depth != 3 {
			t.Fatalf("%s origin = %+v (%v)", tc.typ, d, err)
		}
	}

	p.Status = "completed"
	p, err = st.SaveProject(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	p.Name = "Renamed after completion"
	if _, err := st.SaveProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	types := []string{"project.status_changed"}
	d, err := st.NextEvent(ctx, "reviewer", types)
	if err != nil || d == nil || d.Event.EntityID != projectID || d.Event.Revision != p.Revision ||
		d.Event.Payload == nil || d.Event.Payload.BeforeStatus != "active" || d.Event.Payload.AfterStatus != "completed" {
		t.Fatalf("project completion = %+v (%v)", d, err)
	}
	if err := st.SettleEvent(ctx, "reviewer", types, d.Sequence, false); err != nil {
		t.Fatal(err)
	}
	if d, err := st.NextEvent(ctx, "reviewer", types); err != nil || d != nil {
		t.Fatalf("rename falsely reported a transition: %+v (%v)", d, err)
	}
}
