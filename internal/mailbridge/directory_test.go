package mailbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/connector/vcarddir"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

func TestPluginRefreshesDirectoriesWithoutRunningTUI(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	folder := t.TempDir()
	path := filepath.Join(folder, "alex.vcf")
	write := func(name string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("BEGIN:VCARD\nVERSION:3.0\nUID:alex\nFN:"+name+"\nEMAIL:one@example.org\nEMAIL:two@example.org\nEND:VCARD\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("Original Alex")
	if _, err := vcarddir.Sync(ctx, folder, store); err != nil {
		t.Fatal(err)
	}
	write("Updated Alex")
	invoke := func(method string, input any) map[string]any {
		t.Helper()
		data, _ := json.Marshal(input)
		req, _ := json.Marshal(Request{API: 1, Type: "request", RequestID: "test", Method: method, Data: data})
		var output bytes.Buffer
		if err := Serve(ctx, store, bytes.NewReader(req), &output); err != nil {
			t.Fatal(err)
		}
		var response map[string]any
		if err := json.Unmarshal(output.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	response := invoke("contacts.search", map[string]any{"query": "Updated Alex"})
	if response["ok"] != true {
		t.Fatal(response)
	}
	data := response["data"].(map[string]any)
	if matches := data["contacts"].([]any); len(matches) != 2 {
		t.Fatal("not all emails searchable", matches)
	}
	response = invoke("contacts.list", map[string]any{"query": "Updated"})
	contacts := response["data"].(map[string]any)["contacts"].([]any)
	if len(contacts) != 1 {
		t.Fatal(contacts)
	}
	contact := contacts[0].(map[string]any)
	if contact["read_only"] != true {
		t.Fatal("missing read-only marker", contact)
	}
	contact["name"] = "Cannot Edit"
	if response := invoke("contacts.save", contact); response["ok"] != false {
		t.Fatal("directory save accepted", response)
	}
	if err := os.Rename(folder, folder+"-away"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Rename(folder+"-away", folder) }()
	response = invoke("contacts.list", map[string]any{"query": "Updated"})
	if response["ok"] != true {
		t.Fatal("cached contacts unavailable", response)
	}
	data = response["data"].(map[string]any)
	if data["warning"] == nil || len(data["contacts"].([]any)) != 1 {
		t.Fatal("no cache warning", data)
	}
}
