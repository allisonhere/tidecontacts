package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCompletionConnectionIsReadOnly(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "book.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	if err := r.Put(ctx, "must-not-write", "1"); err == nil {
		t.Fatal("read-only connection accepted write")
	}
}
