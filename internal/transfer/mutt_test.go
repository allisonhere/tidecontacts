package transfer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/search"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

func TestMuttAliases(t *testing.T) {
	input := `# address book
set editor=vim
source "do-not-follow"
alias alex Alex Example <alex@example.org>
alias -group friends -group work duo "Doe, Renée" <renee@example.org>, \
 Bob Example <bob@example.org> # met at work; tags:team
alias single 'Sam Example <sam@example.org>'
alias full "Full Name <full@example.org>"
alias hash "Has # sign" <hash@example.org>
`
	f, err := ReadMutt([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if f.Format != "mutt" || len(f.Cards) != 6 {
		t.Fatalf("%+v", f)
	}
	c := f.Cards[1]
	if c.DisplayName != "Doe, Renée" || c.Nickname != "duo" || len(c.Tags) != 2 || !strings.Contains(c.Notes, "met at work") || c.Methods[0].Value != "renee@example.org" {
		t.Fatalf("%+v", c)
	}
	if f.Cards[5].DisplayName != "Has # sign" {
		t.Fatal("quoted comment marker lost")
	}
}
func TestMuttRejectsUnsupportedAndMalformedAliases(t *testing.T) {
	for _, input := range []string{
		"alias", "alias a", "alias a other-alias", "alias a `touch /tmp/no`", "alias a $ADDRESS", "alias a a@example.org; source other", "alias -group", "alias a \"unterminated", "alias a a@example.org\\", "unalias a", "# empty\nsource aliases",
	} {
		if _, err := ReadMutt([]byte(input)); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}
func TestMuttFilePreviewUsesDuplicateReview(t *testing.T) {
	ctx := context.Background()
	repo, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = repo.Close() }()
	path := filepath.Join(t.TempDir(), "aliases")
	if err := os.WriteFile(path, []byte("alias alex Alex Example <alex@example.org>\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := ReadMuttFile(path)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := BuildPreview(ctx, repo, f)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := repo.List(ctx, "person", search.Query{}, "name")
	if err != nil || len(rows) != 0 {
		t.Fatal("preview wrote contacts", err)
	}
	if len(preview.Selected()) != 1 {
		t.Fatalf("%+v", preview)
	}
	if _, err := repo.SaveContacts(ctx, preview.Selected(), "import"); err != nil {
		t.Fatal(err)
	}
	preview, err = BuildPreview(ctx, repo, f)
	if err != nil || len(preview.Selected()) != 0 || preview.Records[0].Duplicate == "" {
		t.Fatal("duplicate not excluded", err)
	}
}
