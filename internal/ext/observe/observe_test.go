package observe

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/ext"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

var _ app.CRM = (*Store)(nil)
var _ app.Directories = (*Store)(nil)

func setup(t *testing.T) (*Store, *ext.Subscription) {
	t.Helper()
	s, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	bus := ext.NewBus()
	return Wrap(s, bus, "ui"), bus.Subscribe(64)
}

func drain(sub *ext.Subscription) []ext.Event {
	var out []ext.Event
	for {
		select {
		case e := <-sub.C:
			out = append(out, e)
		default:
			return out
		}
	}
}

func TestEventsAfterCommitOnly(t *testing.T) {
	s, sub := setup(t)
	ctx := context.Background()
	c := domain.NewContact("person")
	c.DisplayName = "Ada Fictional"
	c.Notes = "private note"
	saved, err := s.SaveContacts(ctx, []domain.Contact{c}, "edit")
	if err != nil {
		t.Fatal(err)
	}
	ev := drain(sub)
	if len(ev) != 1 || ev[0].Type != "contact.created" || ev[0].EntityID != saved[0].ID || ev[0].Revision != saved[0].Revision || ev[0].Source != "ui" {
		t.Fatalf("created: %+v", ev)
	}
	raw, _ := json.Marshal(ev[0])
	if strings.Contains(string(raw), "Ada") || strings.Contains(string(raw), "private") {
		t.Fatalf("event leaks content: %s", raw)
	}
	up := saved[0]
	up.Notes = "changed"
	if _, err := s.SaveContacts(ctx, []domain.Contact{up}, "edit"); err != nil {
		t.Fatal(err)
	}
	if ev = drain(sub); len(ev) != 1 || ev[0].Type != "contact.updated" {
		t.Fatalf("updated: %+v", ev)
	}
	// A stale save fails: no event.
	if _, err := s.SaveContacts(ctx, []domain.Contact{up}, "edit"); err == nil {
		t.Fatal("expected conflict")
	}
	// An invalid batch rolls back: no event for any member.
	good := domain.NewContact("person")
	good.DisplayName = "Valid"
	bad := domain.NewContact("person")
	if _, err := s.SaveContacts(ctx, []domain.Contact{good, bad}, "import"); err == nil {
		t.Fatal("expected validation error")
	}
	if ev = drain(sub); len(ev) != 0 {
		t.Fatalf("events for a failed transaction: %+v", ev)
	}
}

func TestImportBatchCorrelationAndSources(t *testing.T) {
	s, sub := setup(t)
	ctx := context.Background()
	var batch []domain.Contact
	for _, n := range []string{"One", "Two", "Three"} {
		c := domain.NewContact("person")
		c.DisplayName = n
		batch = append(batch, c)
	}
	if _, err := s.SaveContacts(ctx, batch, "import"); err != nil {
		t.Fatal(err)
	}
	ev := drain(sub)
	if len(ev) != 3 || ev[0].CorrelationID == "" || ev[0].CorrelationID != ev[2].CorrelationID || ev[0].Source != "import" {
		t.Fatalf("%+v", ev)
	}
	org := domain.NewContact("organization")
	org.DisplayName = "Example Works"
	if _, err := s.SaveContacts(ctx, []domain.Contact{org}, "extension:digest-ext"); err != nil {
		t.Fatal(err)
	}
	ev = drain(sub)
	if len(ev) != 1 || ev[0].Type != "organization.created" || !ext.IsExtensionSource(ev[0].Source) {
		t.Fatalf("%+v", ev)
	}
	demo := domain.NewContact("person")
	demo.DisplayName = "Seed"
	_, _ = s.SaveContacts(ctx, []domain.Contact{demo}, "demo")
	if ev = drain(sub); len(ev) != 0 {
		t.Fatalf("demo seeding must not emit: %+v", ev)
	}
}

func TestTaskAndActivityEvents(t *testing.T) {
	s, sub := setup(t)
	ctx := context.Background()
	task, err := s.SaveTask(ctx, domain.Task{Title: "Call"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	cur, _ := s.Task(ctx, task.ID)
	if _, err := s.ReopenTask(ctx, cur.ID); err != nil {
		t.Fatal(err)
	}
	a, err := s.SaveActivity(ctx, domain.Activity{Type: "note", Title: "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteActivity(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range drain(sub) {
		got = append(got, e.Type)
	}
	want := "task.created task.completed task.reopened activity.created activity.deleted"
	if strings.Join(got, " ") != want {
		t.Fatalf("got %v", got)
	}
	// Failed operations emit nothing.
	if err := s.DeleteActivity(ctx, "missing"); err == nil {
		t.Log("delete of missing activity succeeded (idempotent)")
	}
}

func TestNilBusIsPassThrough(t *testing.T) {
	raw, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	s := Wrap(raw, nil, "cli")
	if _, err := s.SaveTask(context.Background(), domain.Task{Title: "x"}); err != nil {
		t.Fatal(err)
	}
}
