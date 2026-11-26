package ext

import (
	"encoding/json"
	"strings"
	"testing"

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
