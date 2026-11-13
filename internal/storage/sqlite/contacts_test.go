package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
	"path/filepath"
	"testing"
)

func testStore(t testing.TB) *Store {
	t.Helper()
	s, e := Open(context.Background(), filepath.Join(t.TempDir(), "contacts.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Close(); e != nil {
			t.Error(e)
		}
	})
	return s
}
func TestContactAggregateSearchAndHistory(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	c := domain.NewContact("person")
	c.Nickname = "Alex"
	c.Notes = "Met at café"
	c.Tags = []string{"developer", "Friend"}
	c.Methods = []domain.Method{{Kind: "email", Value: "alex@github.com", Preferred: true}, {Kind: "phone", Value: "+1 (234) 567-8900"}, {Kind: "address", Address: domain.Address{Street: "12 Main", City: "Lyon"}}}
	c.Memberships = []domain.Membership{{Organization: "Riverside Studio", Title: "Engineer", Current: true}}
	c.Fields = []domain.CustomField{{Name: "Score", Type: "number", Value: "4.5"}}
	c.Extensions = map[string]json.RawMessage{"future": json.RawMessage(`{"v":1}`)}
	saved, e := s.SaveContacts(ctx, []domain.Contact{c}, "create")
	if e != nil {
		t.Fatal(e)
	}
	got, e := s.Contact(ctx, saved[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	if got.DisplayName != "Alex" || len(got.Methods) != 3 || got.Memberships[0].Organization != "Riverside Studio" || got.Fields[0].Value != "4.5" || string(got.Extensions["future"]) != `{"v":1}` {
		t.Fatalf("roundtrip: %+v", got)
	}
	for _, q := range []string{"alex", "alx", "@github.com", "tag:developer", "org:\"riverside studio\"", "has:phone", "12345678900", "cafe"} {
		parsed, e := search.Parse(q)
		if e != nil {
			t.Fatal(e)
		}
		rows, e := s.List(ctx, "person", parsed, "name")
		if e != nil || len(rows) != 1 {
			t.Fatalf("query %q: %v %v", q, rows, e)
		}
	}
	old := got
	got.Favorite = true
	saved, e = s.SaveContacts(ctx, []domain.Contact{got}, "favorite")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SaveContacts(ctx, []domain.Contact{old}, "stale"); e == nil {
		t.Fatal("stale update accepted")
	}
	h, e := s.History(ctx, c.ID)
	if e != nil || len(h) != 2 {
		t.Fatal(h, e)
	}
	got = saved[0]
	got.Deleted = true
	if _, e = s.SaveContacts(ctx, []domain.Contact{got}, "delete"); e != nil {
		t.Fatal(e)
	}
	q, _ := search.Parse("")
	rows, _ := s.List(ctx, "person", q, "name")
	if len(rows) != 0 {
		t.Fatal("tombstone visible")
	}
	q, _ = search.Parse("is:deleted")
	rows, e = s.List(ctx, "person", q, "name")
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
}
func TestAtomicRollbackAndCyclicRelationships(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a, b := domain.NewContact("person"), domain.NewContact("organization")
	a.DisplayName = "A"
	b.DisplayName = "B"
	a.Relationships = []domain.Relationship{{TargetID: b.ID, Type: "client"}}
	b.Relationships = []domain.Relationship{{TargetID: a.ID, Type: "collaborator"}}
	if _, e := s.SaveContacts(ctx, []domain.Contact{a, b}, "batch"); e != nil {
		t.Fatal(e)
	}
	bad := domain.NewContact("person")
	bad.DisplayName = "Bad"
	bad.Relationships = []domain.Relationship{{TargetID: "missing", Type: "friend"}}
	good := domain.NewContact("person")
	good.DisplayName = "Rollback"
	if _, e := s.SaveContacts(ctx, []domain.Contact{good, bad}, "batch"); e == nil {
		t.Fatal("missing FK accepted")
	}
	if _, e := s.Contact(ctx, good.ID); e == nil {
		t.Fatal("partial transaction persisted")
	}
	a, e := s.Contact(ctx, a.ID)
	if e != nil {
		t.Fatal(e)
	}
	a.Methods = []domain.Method{{Kind: "email", Value: "a@example.com", Preferred: true}, {Kind: "email", Value: "b@example.com", Preferred: true}}
	if _, e = s.SaveContacts(ctx, []domain.Contact{a}, "invalid"); e == nil {
		t.Fatal("multiple preferred accepted")
	}
}
func TestUpgradePhaseOne(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "old.db")
	old := migrations
	migrations = old[:1]
	legacy, e := Open(context.Background(), p)
	migrations = old
	if e != nil {
		t.Fatal(e)
	}
	if e = legacy.Put(ctx, "pane-widths", "kept"); e != nil {
		t.Fatal(e)
	}
	if e = legacy.Close(); e != nil {
		t.Fatal(e)
	}
	upgraded, e := Open(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = upgraded.Close() }()
	v, e := upgraded.Get(ctx, "pane-widths")
	if v != "kept" || e != nil {
		t.Fatal(v, e)
	}
	c := domain.NewContact("person")
	c.DisplayName = "Upgraded"
	if _, e = upgraded.SaveContacts(ctx, []domain.Contact{c}, "create"); e != nil {
		t.Fatal(e)
	}
}
func BenchmarkSearchThousands(b *testing.B) {
	s := testStore(b)
	ctx := context.Background()
	batch := []domain.Contact{}
	for i := 0; i < 3000; i++ {
		c := domain.NewContact("person")
		c.DisplayName = fmt.Sprintf("Person %04d", i)
		batch = append(batch, c)
	}
	if _, e := s.SaveContacts(ctx, batch, "seed"); e != nil {
		b.Fatal(e)
	}
	q, _ := search.Parse("prsn 123")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, e := s.List(ctx, "person", q, "name"); e != nil {
			b.Fatal(e)
		}
	}
}

func TestOrganizationRenameIncomingAndTypedFields(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	org := domain.NewContact("organization")
	org.DisplayName = "Old company"
	org.Aliases = []string{"Legacy name"}
	org.Industry = "Software"
	org.Website = "https://example.com"
	person := domain.NewContact("person")
	person.DisplayName = "Member"
	person.Memberships = []domain.Membership{{OrganizationID: org.ID, Title: "Lead", Current: true}}
	person.Relationships = []domain.Relationship{{TargetID: org.ID, Type: "client"}}
	person.Fields = []domain.CustomField{{Name: "Text", Type: "text", Value: "hello"}, {Name: "Number", Type: "number", Value: "12"}, {Name: "Date", Type: "date", Value: "2020-02-29"}, {Name: "Boolean", Type: "boolean", Value: "true"}, {Name: "URL", Type: "url", Value: "https://example.com"}, {Name: "Enum", Type: "enum", Value: "one", Options: []string{"one", "two"}}}
	if _, e := s.SaveContacts(ctx, []domain.Contact{person, org}, "create"); e != nil {
		t.Fatal(e)
	}
	got, e := s.Contact(ctx, org.ID)
	if e != nil || len(got.Members) != 1 || len(got.Incoming) != 1 {
		t.Fatal(got, e)
	}
	got.DisplayName = "New company"
	if _, e = s.SaveContacts(ctx, []domain.Contact{got}, "rename"); e != nil {
		t.Fatal(e)
	}
	q, _ := search.Parse(`org:"New company"`)
	rows, e := s.List(ctx, "person", q, "organization")
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	p, e := s.Contact(ctx, person.ID)
	if e != nil || p.Memberships[0].Organization != "New company" || len(p.Fields) != 6 {
		t.Fatal(p, e)
	}
}
func TestCancelDuringBatchRollsBack(t *testing.T) {
	s := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	a := domain.NewContact("person")
	a.DisplayName = "Canceled"
	_, e := s.SaveContactsWithProgress(ctx, []domain.Contact{a}, "import", func(int, int) { cancel() })
	if e == nil {
		t.Fatal("cancellation committed")
	}
	if _, e = s.Contact(context.Background(), a.ID); e == nil {
		t.Fatal("partial write survived cancellation")
	}
}
