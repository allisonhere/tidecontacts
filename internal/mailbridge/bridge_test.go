package mailbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
	"path/filepath"
	"testing"
)

func TestBookCRUDPreservesCRM(t *testing.T) {
	ctx := context.Background()
	s, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "crm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := domain.NewContact("person")
	c.DisplayName = "Alex"
	c.Biography = "private biography"
	c.Tags = []string{"friend"}
	c.Methods = []domain.Method{{Kind: "email", Value: "alex@example.com"}, {Kind: "email", Value: "second@example.com"}, {Kind: "phone", Value: "1234"}}
	saved, err := s.SaveContacts(ctx, []domain.Contact{c}, "test")
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(method string, data any) (json.RawMessage, bool) {
		t.Helper()
		raw, _ := json.Marshal(data)
		req, _ := json.Marshal(Request{API: 1, Type: "request", RequestID: "test", Method: method, Data: raw})
		var out bytes.Buffer
		if err := Serve(ctx, s, bytes.NewReader(req), &out); err != nil {
			t.Fatal(err)
		}
		var result struct {
			OK   bool            `json:"ok"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Data, result.OK
	}
	raw, ok := invoke("contacts.list", map[string]any{"query": "Alex"})
	if !ok {
		t.Fatal("list failed")
	}
	var list struct {
		Contacts []Contact `json:"contacts"`
	}
	json.Unmarshal(raw, &list)
	if len(list.Contacts) != 1 {
		t.Fatal(string(raw))
	}
	edit := list.Contacts[0]
	edit.Name = "Alex Updated"
	edit.Phone = ""
	edit.Note = "Updated in TideMail"
	edit.Organization = "Harbor"
	raw, ok = invoke("contacts.save", edit)
	if !ok {
		t.Fatal("save failed")
	}
	got, err := s.Contact(ctx, saved[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DisplayName != "Alex Updated" || got.Biography != "private biography" || len(got.Tags) != 1 || len(got.Methods) != 2 || got.Methods[1].Value != "second@example.com" {
		t.Fatalf("CRM fields lost: %+v", got)
	}
	if _, ok = invoke("contacts.save", edit); ok {
		t.Fatal("stale revision accepted")
	}
	if _, ok = invoke("contacts.save", Contact{Name: "Duplicate", Addr: "alex@example.com"}); ok {
		t.Fatal("duplicate accepted")
	}
	if _, ok = invoke("contacts.save", Contact{Name: "No email", Addr: "invalid"}); ok {
		t.Fatal("invalid email accepted")
	}
	raw, ok = invoke("contacts.search", map[string]any{"query": "Updated", "limit": 50})
	if !ok {
		t.Fatal("search failed")
	}
	if !bytes.Contains(raw, []byte("Alex Updated")) {
		t.Fatal(string(raw))
	}
}
