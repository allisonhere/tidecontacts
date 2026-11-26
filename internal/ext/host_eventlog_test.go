package ext

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
)

func TestDurableEventTransportFiltersAndSettles(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	person := domain.NewContact("person")
	person.DisplayName, person.Notes = "Fixture Contact", "Private fixture note"
	if _, err := store.SaveContacts(ctx, []domain.Contact{person}, "test"); err != nil {
		t.Fatal(err)
	}
	task, err := store.SaveTask(ctx, domain.Task{Title: "Private fixture task"})
	if err != nil {
		t.Fatal(err)
	}
	s := &Session{ID: "fixture-ext", Repo: store, durableTypes: []string{"contact.created", "task.created"}, Granted: NewPermissionSet("tasks.read")}
	if _, e := call(t, s, "events.next", nil); e == nil || e.Code != CodePermissionDenied {
		t.Fatalf("read without consume grant: %v", e)
	}
	s.Granted = NewPermissionSet("tasks.read", "events.consume")
	if _, e := call(t, s, "events.next", map[string]string{"consumer": "other-ext"}); e == nil || e.Code != CodeInvalid {
		t.Fatalf("caller supplied consumer: %v", e)
	}
	raw, e := call(t, s, "events.next", nil)
	var d app.EventDelivery
	if e != nil || json.Unmarshal(raw, &d) != nil || d.Event.EntityID != task.ID || strings.Contains(string(raw), "Private") || d.Event.SchemaVersion != 2 {
		t.Fatalf("next delivery: %s (%v)", raw, e)
	}
	if _, e := call(t, s, "events.settle", map[string]int64{"sequence": d.Sequence + 1}); e == nil || e.Code != CodeConflict {
		t.Fatalf("settled an undeliverable sequence: %v", e)
	}
	for range 2 {
		if _, e := call(t, s, "events.settle", map[string]int64{"sequence": d.Sequence}); e != nil {
			t.Fatalf("acknowledge: %v", e)
		}
	}
	s.Granted = NewPermissionSet("tasks.read", "events.consume", "contacts.read")
	if raw, e := call(t, s, "events.next", nil); e != nil || !strings.Contains(string(raw), `"type":"contact.created"`) {
		t.Fatalf("adding a topic grant skipped earlier events: %s (%v)", raw, e)
	}
}
