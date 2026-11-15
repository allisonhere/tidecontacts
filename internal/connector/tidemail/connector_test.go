package tidemail

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

func TestSyncReadOnlyAndRepeatable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mail.db")
	source, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.Exec(`CREATE TABLE messages(id INTEGER PRIMARY KEY,message_id TEXT,subject TEXT,from_addr TEXT,to_addr TEXT,cc_addr TEXT,date INTEGER);
 INSERT INTO messages VALUES(1,'<one@example.org>','Hello','Alex <alex@example.org>','me@example.org','',1700000000),
 (2,'<one@example.org>','Hello','alex@example.org','me@example.org','',1700000000),
 (3,'','Missing ID','','','',1700000000);`)
	if err != nil {
		t.Fatal(err)
	}
	source.Close()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "crm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	contact := domain.NewContact("person")
	contact.Nickname = "Alex"
	contact.Methods = []domain.Method{{Kind: "email", Value: "alex@example.org"}}
	if _, err := store.SaveContacts(ctx, []domain.Contact{contact}, "create"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		r, err := Sync(ctx, path, store)
		if err != nil {
			t.Fatal(err)
		}
		if r.Imported != 1 || r.Skipped != 2 || r.LastSynced == "" {
			t.Fatalf("result: %+v", r)
		}
	}
	records, err := store.Activities(ctx, app.ActivityFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ExternalRef != "one@example.org" || records[0].Body != "" {
		t.Fatalf("activities: %+v", records)
	}
	linked, err := store.Activity(ctx, records[0].ID)
	if err != nil || len(linked.Entities) != 1 || linked.Entities[0].ID != contact.ID {
		t.Fatal("participant not linked", err)
	}
	if err := store.DeleteActivity(ctx, records[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(ctx, path, store); err != nil {
		t.Fatal(err)
	}
	deleted, err := store.Activity(ctx, records[0].ID)
	if err != nil || !deleted.Deleted {
		t.Fatal("sync revived deleted activity", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("source changed")
	}
	last, err := store.Get(ctx, "tidemail-last-synced")
	if err != nil || last == "" {
		t.Fatalf("status: %q %v", last, err)
	}
	missing := filepath.Join(t.TempDir(), "missing.db")
	if _, err = Sync(ctx, missing, store); err == nil {
		t.Fatal("missing source accepted")
	}
	if _, err = os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("unexpected file")
	}
	retained, _ := store.Get(ctx, "tidemail-last-synced")
	if retained != last {
		t.Fatal("failed sync changed successful timestamp")
	}
}
