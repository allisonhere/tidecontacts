package transfer

import (
	"bytes"
	"context"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTripAndNoClobber(t *testing.T) {
	c := domain.NewContact("person")
	c.DisplayName = "Renée Example"
	c.Given = "Renée"
	c.Family = "Example"
	c.Notes = "private\nmultiline"
	c.Tags = []string{"friend"}
	c.Methods = []domain.Method{{Kind: "email", Value: "one@example.com"}, {Kind: "email", Value: "two@example.com"}, {Kind: "address", Address: domain.Address{Street: "12 Main", City: "Montréal"}}}
	c.Fields = []domain.CustomField{{Name: "custom", Type: "boolean", Value: "true"}}
	c.Normalize()
	repo, e := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = repo.Close() }()
	for _, format := range []string{"csv", "vcard"} {
		var buf bytes.Buffer
		if e := Write(&buf, format, []domain.Contact{c}); e != nil {
			t.Fatal(e)
		}
		f, e := Read(buf.Bytes())
		if e != nil {
			t.Fatal(e)
		}
		p, e := BuildPreview(context.Background(), repo, f)
		if e != nil || len(p.Records) != 1 {
			t.Fatal(p, e)
		}
		got := p.Records[0].Contact
		if got.Notes != c.Notes || len(got.Methods) != 3 || len(got.Fields) != 1 || got.ID == c.ID {
			t.Fatalf("%s roundtrip %+v", format, got)
		}
		path := filepath.Join(t.TempDir(), "export")
		if e = ExportFile(path, format, []domain.Contact{c}); e != nil {
			t.Fatal(e)
		}
		before, _ := os.ReadFile(path)
		if e = ExportFile(path, format, nil); e == nil {
			t.Fatal("clobber succeeded")
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(before, after) {
			t.Fatal("existing export changed")
		}
	}
}
func TestCSVMappingDuplicatesAndVCard3(t *testing.T) {
	ctx := context.Background()
	repo, e := sqlite.Open(ctx, filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = repo.Close() }()
	f, e := Read([]byte("First Name,Last Name,Email,Company\nAlex,Smith,alex@example.com,River\nInvalid,Email,broken,River\nAlex,Smith,alex@example.com,River\n"))
	if e != nil {
		t.Fatal(e)
	}
	p, e := BuildPreview(ctx, repo, f)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Selected()) != 1 || p.Records[1].Error == "" || p.Records[2].Duplicate == "" {
		t.Fatal(p)
	}
	if _, e = repo.SaveContacts(ctx, p.Selected(), "import"); e != nil {
		t.Fatal(e)
	}
	again, e := BuildPreview(ctx, repo, f)
	if e != nil || len(again.Selected()) != 0 {
		t.Fatal(again, e)
	}
	vcf := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:No Email\r\nN:Email;No;;;\r\nTEL;TYPE=CELL:123456\r\nNOTE:First\\nSecond\r\nX-UNKNOWN:keep\r\nEND:VCARD\r\n"
	f, e = Read([]byte(vcf))
	if e != nil {
		t.Fatal(e)
	}
	p, e = BuildPreview(ctx, repo, f)
	if e != nil || len(p.Selected()) != 1 {
		t.Fatal(p, e)
	}
	var out bytes.Buffer
	if e = Write(&out, "vcard", p.Selected()); e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(out.Bytes(), []byte("X-UNKNOWN:keep")) {
		t.Fatal(out.String())
	}
}
