package demo

import (
	"context"
	"github.com/allisonhere/tidecontacts/internal/config"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
	"os"
	"path/filepath"
	"testing"
)

func TestSeedAndIsolation(t *testing.T) {
	ctx := context.Background()
	live := config.Paths{Database: filepath.Join(t.TempDir(), "live.db")}
	if err := os.WriteFile(live.Database, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := sqlite.Open(ctx, Paths(live).Database)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := Seed(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	if err := Reset(live); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(live.Database)
	if err != nil || string(data) != "untouched" {
		t.Fatal("live data changed")
	}
}
