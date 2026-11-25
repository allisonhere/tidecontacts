package transfer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadDirectoryUIDsMultiCardAndNoEmail(t *testing.T) {
	folder := t.TempDir()
	text := "BEGIN:VCARD\nVERSION:3.0\nUID:one\nFN:One\nEMAIL:one@example.org\nEMAIL:two@example.org\nEND:VCARD\nBEGIN:VCARD\nVERSION:3.0\nUID:two\nFN:No Email\nADR:;;;Town;;;\nEND:VCARD\n"
	if err := os.WriteFile(filepath.Join(folder, "both.vcf"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ReadDirectory(context.Background(), folder)
	if err != nil || len(snapshot.Cards) != 2 || len(snapshot.Cards[0].Contact.Methods) != 2 {
		t.Fatal(snapshot, err)
	}
	if snapshot.Cards[0].Key != "uid:one" || snapshot.Cards[1].Contact.DisplayName != "No Email" {
		t.Fatal(snapshot)
	}
	if err := os.WriteFile(filepath.Join(folder, "duplicate.vcf"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadDirectory(context.Background(), folder); err == nil {
		t.Fatal("duplicate UIDs accepted")
	}
}

// TestReadDirectoryDropsUnusableWebsiteInsteadOfRejectingCard guards against
// one external, unfixable field (here an ftp:// link, which a bare-domain
// scheme fix can't repair) blocking an entire connected directory's sync —
// the source account, not this app, owns the data, so the field drops
// silently rather than failing the whole card.
func TestReadDirectoryDropsUnusableWebsiteInsteadOfRejectingCard(t *testing.T) {
	folder := t.TempDir()
	text := "BEGIN:VCARD\nVERSION:3.0\nUID:one\nFN:One\nEMAIL:one@example.org\nURL:ftp://files.example.org/\nURL:example.com\nEND:VCARD\n"
	if err := os.WriteFile(filepath.Join(folder, "card.vcf"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ReadDirectory(context.Background(), folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(snapshot.Cards))
	}
	var websites []string
	for _, m := range snapshot.Cards[0].Contact.Methods {
		if m.Kind == "website" {
			websites = append(websites, m.Value)
		}
	}
	if len(websites) != 1 || websites[0] != "https://example.com" {
		t.Fatalf("expected only the fixable website to survive, got %v", websites)
	}
}
func TestReadDirectoryDoesNotFollowCardSymlinks(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(t.TempDir(), "outside.vcf")
	if err := os.WriteFile(path, []byte("BEGIN:VCARD\nVERSION:3.0\nFN:Outside\nEND:VCARD\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, filepath.Join(folder, "link.vcf")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadDirectory(context.Background(), folder); err == nil {
		t.Fatal("followed symlink")
	}
}
