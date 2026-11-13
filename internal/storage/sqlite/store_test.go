package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func TestInitializeReopenAndClose(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "db.sqlite")
	s, e := Open(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	for pragma, want := range map[string]int{"foreign_keys": 1, "busy_timeout": 5000} {
		var got int
		if e = s.db.QueryRow("PRAGMA " + pragma).Scan(&got); e != nil || got != want {
			t.Fatal(pragma, got, e)
		}
	}
	var journal string
	_ = s.db.QueryRow("PRAGMA journal_mode").Scan(&journal)
	if journal != "wal" {
		t.Fatal(journal)
	}
	if e = s.Put(ctx, "layout", "value"); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Get(ctx, "layout"); e == nil {
		t.Fatal("closed handle usable")
	}
	s, e = Open(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	v, e := s.Get(ctx, "layout")
	if e != nil || v != "value" {
		t.Fatal(v, e)
	}
	var count int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if count != len(migrations) {
		t.Fatal(count)
	}
}
func TestMigrationRollbackAndFutureVersion(t *testing.T) {
	ctx := context.Background()
	s, e := Open(ctx, filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	original := migrations
	defer func() { migrations = original }()
	migrations = append(append([]string{}, original...), `CREATE TABLE temporary_table(id INTEGER); INVALID SQL;`)
	if e = s.migrate(ctx); e == nil {
		t.Fatal("invalid migration accepted")
	}
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='temporary_table'`).Scan(&n)
	if n != 0 {
		t.Fatal("migration partially committed")
	}
	_, e = s.db.Exec(`INSERT INTO schema_migrations VALUES(99)`)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.migrate(ctx); e == nil {
		t.Fatal("future schema accepted")
	}
}
func TestCanceledOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if s, e := Open(ctx, filepath.Join(t.TempDir(), "db")); e == nil {
		_ = s.Close()
		t.Fatal("canceled open succeeded")
	}
}
