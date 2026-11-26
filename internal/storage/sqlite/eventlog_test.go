package sqlite

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
)

func TestEventDeliveryRetryReplayAndRetention(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "contacts.db")
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	open := func() *Store {
		s := must[*Store](t)(Open(ctx, path))
		s.SetClock(func() time.Time { return now })
		return s
	}
	s := open()
	first := must[domain.Task](t)(s.SaveTask(ctx, domain.Task{Title: "First fixture"}))
	second := must[domain.Task](t)(s.SaveTask(ctx, domain.Task{Title: "Second fixture"}))
	topics := []string{"task.created"}
	next := func(consumer string) *app.EventDelivery {
		return must[*app.EventDelivery](t)(s.NextEvent(ctx, consumer, topics))
	}
	original := next("reader")
	if original == nil || original.Event.EntityID != first.ID {
		t.Fatalf("first delivery = %+v", original)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = open()
	t.Cleanup(func() { _ = s.Close() })
	seq := original.Sequence
	if err := s.SettleEvent(ctx, "other", topics, seq, false); !errors.Is(err, app.ErrEventNotEligible) {
		t.Fatalf("foreign acknowledgment: %v", err)
	}
	for n := range eventMaxAttempts {
		d := next("reader")
		if d == nil || d.Attempts != n || d.Sequence != seq || d.Event.ID != original.Event.ID {
			t.Fatalf("attempt %d = %+v", n, d)
		}
		if err := s.SettleEvent(ctx, "reader", topics, seq, true); err != nil {
			t.Fatal(err)
		}
		if n < eventMaxAttempts-1 {
			if d := next("reader"); d != nil {
				t.Fatalf("newer same-topic event bypassed backoff: %+v", d)
			}
			now = now.Add(time.Second << n)
		}
	}
	d := next("reader")
	if d == nil || d.Event.EntityID != second.ID {
		t.Fatalf("dead letter did not release topic: %+v", d)
	}
	if err := s.SettleEvent(ctx, "reader", topics, d.Sequence, false); err != nil {
		t.Fatal(err)
	}
	dead := must[[]app.EventDelivery](t)(s.DeadEvents(ctx, "reader", topics, 0, 50))
	if len(dead) != 1 || dead[0].Event.ID != original.Event.ID || dead[0].Attempts != 5 {
		t.Fatalf("dead letters = %+v", dead)
	}
	if backupReader := next("other"); backupReader == nil || backupReader.Sequence != seq {
		t.Fatalf("second reader = %+v", backupReader)
	}
	now = now.AddDate(0, 0, 181)
	if n := must[int64](t)(s.PruneEvents(ctx)); n != 0 {
		t.Fatalf("pruned %d pinned events", n)
	}
	if err := s.ReplayEvent(ctx, "reader", topics, seq); err != nil {
		t.Fatal(err)
	}
	replay := next("reader")
	if replay == nil || replay.Attempts != 0 || replay.Event.ID != original.Event.ID {
		t.Fatalf("replay = %+v", replay)
	}
	if err := s.SettleEvent(ctx, "reader", topics, seq, false); err != nil {
		t.Fatal(err)
	}
	if d := next("reader"); d != nil {
		t.Fatalf("replay rewound the checkpoint: %+v", d)
	}
	if err := s.SettleEvent(ctx, "other", topics, seq, false); err != nil {
		t.Fatal(err)
	}
	if n := must[int64](t)(s.PruneEvents(ctx)); n != 1 {
		t.Fatalf("pruned %d; want only acknowledged event", n)
	}
	if d := next("other"); d == nil || d.Event.EntityID != second.ID {
		t.Fatalf("unacknowledged event lost: %+v", d)
	}
}

func TestEventAppendFailureRollsBackBusinessWrite(t *testing.T) {
	ctx := t.Context()
	s := testStore(t)
	if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER block_append BEFORE INSERT ON event_log BEGIN SELECT RAISE(ABORT,'fixture rejection'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveTask(ctx, domain.Task{Title: "Must not commit"}); err == nil {
		t.Fatal("business write succeeded despite journal append failure")
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("stored %d tasks after rollback: %v", n, err)
	}
}
