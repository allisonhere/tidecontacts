package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

func recipientFixture(t *testing.T) harness {
	h := setup(t)
	s, err := sqlite.Open(context.Background(), h.db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	contacts := []domain.Contact{}
	add := func(name, kind string, methods ...domain.Method) domain.Contact {
		c := domain.NewContact(kind)
		c.DisplayName = name
		c.Methods = methods
		return c
	}
	c := add("Alex O'Connor", "person", domain.Method{Kind: "email", Value: "alex@example.com", Preferred: true}, domain.Method{Kind: "email", Value: "second@example.com"}, domain.Method{Kind: "phone", Value: "1234"})
	c.Notes = "private-search-word"
	c.Memberships = []domain.Membership{{Organization: "Harbor", Current: true}}
	contacts = append(contacts, c)
	c = add("Archived", "person", domain.Method{Kind: "email", Value: "archive@example.com"})
	c.Archived = true
	contacts = append(contacts, c)
	c = add("Deleted", "person", domain.Method{Kind: "email", Value: "deleted@example.com"})
	c.Deleted = true
	contacts = append(contacts, c)
	contacts = append(contacts, add("No email", "person", domain.Method{Kind: "phone", Value: "555"}), add("Shared", "person", domain.Method{Kind: "email", Value: "ALEX@example.com"}), add("Support", "organization", domain.Method{Kind: "email", Value: "support@example.com"}))
	if _, err := s.SaveContacts(context.Background(), contacts, "test"); err != nil {
		t.Fatal(err)
	}
	return h
}
func queryRun(t *testing.T, h harness, format, query string) (string, string, int) {
	t.Helper()
	var out, diag bytes.Buffer
	err := Run(context.Background(), []string{"contacts", "query", "--format", format, "--query", query, "--database", h.db}, strings.NewReader(""), &out, &diag)
	code := 0
	if err != nil {
		var e *Error
		if !errors.As(err, &e) {
			t.Fatal(err)
		}
		code = e.ExitCode
	}
	return out.String(), diag.String(), code
}
func TestMailClientLookupProtocolsAndPrivacy(t *testing.T) {
	h := recipientFixture(t)
	before, err := os.ReadFile(h.db)
	if err != nil {
		t.Fatal(err)
	}
	aerc, diag, code := queryRun(t, h, "aerc", "alex")
	if code != 0 || diag != "" {
		t.Fatal(aerc, diag, code)
	}
	if aerc != "alex@example.com\tAlex O'Connor\nsecond@example.com\tAlex O'Connor\n" {
		t.Fatal(aerc)
	}
	neo, diag, code := queryRun(t, h, "neomutt", "O'Connor")
	expected := "TideContacts: 2 matching address(es)\nalex@example.com\tAlex O'Connor\tHarbor\nsecond@example.com\tAlex O'Connor\tHarbor\n"
	if code != 0 || diag != "" || neo != expected {
		t.Fatal(neo, diag, code)
	}
	all, _, code := queryRun(t, h, "aerc", "")
	if code != 0 || strings.Count(all, "\n") != 3 || !strings.Contains(all, "support@example.com") {
		t.Fatal(all)
	}
	for _, absent := range []string{"archive@example.com", "deleted@example.com", "private-search-word", "555"} {
		if strings.Contains(all, absent) {
			t.Fatal("private/inactive data exposed", all)
		}
	}
	for _, literal := range []string{"private-search-word", "is:deleted", "$(touch /tmp/should-not-exist)", "a; echo hacked"} {
		text, diag, code := queryRun(t, h, "aerc", literal)
		if text != "" || diag != "" || code != 0 {
			t.Fatal(literal, text, diag, code)
		}
	}
	text, diag, code := queryRun(t, h, "neomutt", "no-match")
	if code != 1 || strings.Count(text, "\n") != 1 || !strings.Contains(text, "No matching addresses") {
		t.Fatal(text, diag, code)
	}
	var failure map[string]any
	if json.Unmarshal([]byte(diag), &failure) != nil {
		t.Fatal(diag)
	}
	after, err := os.ReadFile(h.db)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("lookup modified database", err)
	}
}
func TestRecipientLimitsKindsAndPositionalQueries(t *testing.T) {
	h := recipientFixture(t)
	response := h.run(0, "", "contacts", "query", "--query", "Harbor", "--limit", "1")
	if response.Meta.Count != 1 || !response.Meta.PossiblyTruncated {
		t.Fatal(response.Meta)
	}
	h.run(2, "", "contacts", "list", "--format", "aerc")
	h.run(2, "", "contacts", "query", "--format", "invalid")
	h.run(2, "", "contacts", "query", "--format", "aerc", "--limit", "100")
	h.run(2, "", "contacts", "query", "O'Connor", "--query", "alex")
	var out, diag bytes.Buffer
	err := Run(context.Background(), []string{"--database", h.db, "contacts", "query", "--format", "aerc", "--", "O'Connor"}, strings.NewReader(""), &out, &diag)
	if err != nil || strings.Count(out.String(), "\n") != 2 {
		t.Fatal(err, out.String(), diag.String())
	}
	out.Reset()
	diag.Reset()
	err = Run(context.Background(), []string{"--database", h.db, "contacts", "query", "--kind", "organization", "--format", "aerc"}, strings.NewReader(""), &out, &diag)
	if err != nil || out.String() != "support@example.com\tSupport\n" {
		t.Fatal(err, out.String())
	}
}
func TestRecipientOutputIsSingleLine(t *testing.T) {
	name := singleLine("A\tB\r\nC\x1b\u202e D")
	if strings.ContainsAny(name, "\r\n\t\x1b") || strings.ContainsRune(name, '\u202e') {
		t.Fatal(name)
	}
	var out bytes.Buffer
	if err := writeRecipients(&out, []domain.Recipient{{Email: "ok@example.com", Name: name, Organization: singleLine("Org\nSecond")}}, "neomutt"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2 || len(strings.Split(lines[1], "\t")) != 3 {
		t.Fatal(out.String())
	}
}
func TestLookupDoesNotInitializeDatabase(t *testing.T) {
	h := setup(t)
	out, _, code := queryRun(t, h, "aerc", "")
	if code != 1 || out != "" {
		t.Fatal(code, out)
	}
	if _, err := os.Stat(h.db); !os.IsNotExist(err) {
		t.Fatal("lookup created database")
	}
	out, _, code = queryRun(t, h, "neomutt", "")
	if code != 1 || strings.Count(out, "\n") != 1 {
		t.Fatal(code, out)
	}
}
