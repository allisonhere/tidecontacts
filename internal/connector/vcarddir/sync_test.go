package vcarddir

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

func writeCard(t *testing.T, path, uid, name, email string) {
	t.Helper()
	card := fmt.Sprintf("BEGIN:VCARD\nVERSION:3.0\nUID:%s\nFN:%s\nEMAIL:%s\nORG:Workplace\nTITLE:Engineer\nEND:VCARD\n", uid, name, email)
	if err := os.WriteFile(path, []byte(card), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestDirectorySyncIdentityOwnershipDeletionAndRecovery(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	local := domain.NewContact("person")
	local.DisplayName = "Local Alex"
	local.Methods = []domain.Method{{Kind: "email", Value: "alex@example.org"}}
	locals, err := store.SaveContacts(ctx, []domain.Contact{local}, "create")
	if err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	path := filepath.Join(folder, "alex.vcf")
	writeCard(t, path, "alex", "External Alex", "alex@example.org")
	first, err := Sync(ctx, folder, store)
	if err != nil {
		t.Fatal(err)
	}
	if first.Added != 2 {
		t.Fatalf("expected person and owned organization: %+v", first)
	}
	rows, err := store.List(ctx, "person", search.Query{}, "name")
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	var external domain.Contact
	for _, row := range rows {
		if row.ID != locals[0].ID {
			external, err = store.Contact(ctx, row.ID)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if !external.DirectoryReadOnly() || len(external.Memberships) != 1 {
		t.Fatalf("%+v", external)
	}
	external.Source = "manual"
	external.DisplayName = "Cannot change"
	if _, err := store.SaveContacts(ctx, []domain.Contact{external}, "edit"); err == nil {
		t.Fatal("bypassed read-only source guard")
	}
	writeCard(t, path, "alex", "Alex Updated", "new@example.org")
	updated, err := Sync(ctx, folder, store)
	if err != nil || updated.Updated != 1 {
		t.Fatal(updated, err)
	}
	got, err := store.Contact(ctx, external.ID)
	if err != nil || got.DisplayName != "Alex Updated" {
		t.Fatal(got, err)
	}
	history, err := store.History(ctx, external.ID)
	if err != nil || len(history) != 2 {
		t.Fatal(history, err)
	}
	if err := os.Rename(path, filepath.Join(folder, "renamed.vcf")); err != nil {
		t.Fatal(err)
	}
	unchanged, err := Sync(ctx, folder, store)
	if err != nil || unchanged.Unchanged != 2 {
		t.Fatal(unchanged, err)
	}
	if err := os.Remove(filepath.Join(folder, "renamed.vcf")); err != nil {
		t.Fatal(err)
	}
	removed, err := Sync(ctx, folder, store)
	if err != nil || removed.Removed != 2 {
		t.Fatal(removed, err)
	}
	got, err = store.Contact(ctx, external.ID)
	if err != nil || !got.Deleted {
		t.Fatal(got, err)
	}
	localAfter, err := store.Contact(ctx, locals[0].ID)
	if err != nil || localAfter.Revision != locals[0].Revision || localAfter.Deleted {
		t.Fatal("local contact changed", err)
	}
	writeCard(t, path, "alex", "Returned Alex", "return@example.org")
	if _, err := Sync(ctx, folder, store); err != nil {
		t.Fatal(err)
	}
	got, err = store.Contact(ctx, external.ID)
	if err != nil || got.Deleted || got.DisplayName != "Returned Alex" {
		t.Fatal(got, err)
	}
	books, err := store.ContactDirectories(ctx)
	if err != nil || len(books) != 1 {
		t.Fatal(books, err)
	}
	if err := store.DisconnectContactDirectory(ctx, books[0].ID, books[0].Revision); err != nil {
		t.Fatal(err)
	}
	if reports, warning := RefreshAll(ctx, store); warning != "" || len(reports) != 0 {
		t.Fatal("disconnected book refreshed", reports, warning)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("source file modified", err)
	}
	if _, err := Sync(ctx, folder, store); err != nil {
		t.Fatal(err)
	}
	got, err = store.Contact(ctx, external.ID)
	if err != nil || got.Deleted {
		t.Fatal("reconnect did not retain identity", err)
	}
}
func TestFailedDirectoryScanNeverPrunesAndConcurrentSnapshotsConflict(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	folder := t.TempDir()
	path := filepath.Join(folder, "one.vcf")
	writeCard(t, path, "one", "One", "one@example.org")
	if _, err := Sync(ctx, folder, store); err != nil {
		t.Fatal(err)
	}
	snapshot, revision, err := Prepare(ctx, folder, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(ctx, folder, store); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyContactDirectory(ctx, snapshot, revision); err == nil {
		t.Fatal("accepted stale directory snapshot")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(folder, "broken.vcf")
	if err := os.WriteFile(broken, []byte("not a vCard"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(ctx, folder, store); err == nil {
		t.Fatal("bad file accepted")
	}
	rows, err := store.List(ctx, "person", search.Query{}, "name")
	if err != nil || len(rows) != 1 {
		t.Fatal("failed scan pruned cached contact", rows, err)
	}
	if err := os.Rename(folder, folder+"-away"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Rename(folder+"-away", folder) }()
	if _, warning := RefreshAll(ctx, store); warning == "" {
		t.Fatal("missing directory not reported")
	}
	rows, err = store.List(ctx, "person", search.Query{}, "name")
	if err != nil || len(rows) != 1 {
		t.Fatal("missing directory pruned cached contact", rows, err)
	}
}
func TestDirectorySnapshotRollbackOnInvalidContact(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	good := domain.NewContact("person")
	good.DisplayName = "Good"
	bad := domain.NewContact("person")
	bad.DisplayName = "Bad"
	bad.Methods = []domain.Method{{Kind: "email", Value: "not-email"}}
	_, err = store.ApplyContactDirectory(ctx, domain.DirectorySnapshot{Path: t.TempDir(), Cards: []domain.DirectoryCard{{Key: "good", Fingerprint: "1", Contact: good}, {Key: "bad", Fingerprint: "2", Contact: bad}}}, 0)
	if err == nil {
		t.Fatal("invalid batch accepted")
	}
	rows, err := store.List(ctx, "person", search.Query{}, "name")
	if err != nil || len(rows) != 0 {
		t.Fatal("partial snapshot saved", rows, err)
	}
	books, err := store.ContactDirectories(ctx)
	if err != nil || len(books) != 0 {
		t.Fatal("failed snapshot connected a book", books, err)
	}
}

func TestMixedVdirItems(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "contacts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	dir := t.TempDir()
	if _, err := Sync(ctx, dir, store); err == nil {
		t.Fatal("connected an empty new folder")
	}
	for filename, field := range map[string]string{"postal": "ADR:;;42 Example Street;Springfield;;;", "phone.VCF": "TEL:555-0150", "email.vcard": "EMAIL:sample@example.org", "mixed.vcf": "TEL:555-0101\nEMAIL:mixed@example.org\nADR:;;1 Example Street;;;;"} {
		data := "BEGIN:VCARD\nVERSION:3.0\nUID:" + filename + "\n" + field + "\nEND:VCARD\n"
		if err := os.WriteFile(filepath.Join(dir, filename), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "displayname"), []byte("Contacts"), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := Sync(ctx, dir, store)
	if err != nil || report.Added != 4 {
		t.Fatal(report, err)
	}
	books, err := store.ContactDirectories(ctx)
	if err != nil || len(books) != 1 || books[0].Contacts != 4 {
		t.Fatal(books, err)
	}
	report, err = Sync(ctx, dir, store)
	if err != nil || report.Unchanged != 4 {
		t.Fatal(report, err)
	}
}
