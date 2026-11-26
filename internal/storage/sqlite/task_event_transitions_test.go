package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
)

func TestTaskJournalCapturesWaitingAndReopenOnce(t *testing.T) {
	ctx := t.Context()
	s := must[*Store](t)(Open(ctx, filepath.Join(t.TempDir(), "task-events.db")))
	defer s.Close()
	task := must[domain.Task](t)(s.SaveTask(ctx, domain.Task{Title: "Review", Status: "next"}))
	task.Status = "waiting"
	task = must[domain.Task](t)(s.SaveTask(ctx, task))
	task.Title = "Same wait with updated title"
	task = must[domain.Task](t)(s.SaveTask(ctx, task))
	must[domain.Task](t)(s.CompleteTask(ctx, task.ID))
	reopened := must[domain.Task](t)(s.ReopenTask(ctx, task.ID))
	topics := []string{"task.waiting", "task.reopened"}
	for _, want := range []struct {
		typ, before, after string
		revision           int
	}{
		{"task.waiting", "next", "waiting", 2},
		{"task.reopened", "completed", reopened.Status, reopened.Revision},
	} {
		d := must[*app.EventDelivery](t)(s.NextEvent(ctx, "reader", topics))
		if d == nil || d.Event.Type != want.typ || d.Event.EntityID != task.ID || d.Event.Revision != want.revision ||
			d.Event.Payload == nil || d.Event.Payload.BeforeStatus != want.before || d.Event.Payload.AfterStatus != want.after {
			t.Fatalf("want %+v, got %+v", want, d)
		}
		if err := s.SettleEvent(ctx, "reader", topics, d.Sequence, false); err != nil {
			t.Fatal(err)
		}
	}
	if d := must[*app.EventDelivery](t)(s.NextEvent(ctx, "reader", topics)); d != nil {
		t.Fatalf("unchanged state produced an extra event: %+v", d)
	}
}
