package ext

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

func newStore(t *testing.T) *sqlite.Store {
	t.Helper()
	s, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func call(t *testing.T, s *Session, method string, args any) (json.RawMessage, *ProtocolError) {
	t.Helper()
	raw, _ := json.Marshal(args)
	out, perr := s.Call(context.Background(), method, raw)
	if perr != nil {
		return nil, perr
	}
	b, _ := json.Marshal(out)
	return b, nil
}

func TestHostDefaultDenyAndGrants(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	person := domain.NewContact("person")
	person.DisplayName = "Ada Example"
	org := domain.NewContact("organization")
	org.DisplayName = "Example Works"
	saved, err := store.SaveContacts(ctx, []domain.Contact{person, org}, "test")
	if err != nil {
		t.Fatal(err)
	}
	denied := &Session{ID: "x-ext", Repo: store} // nil grants
	for _, m := range []string{"contacts.list", "contacts.get", "contacts.save", "activities.list", "activities.save", "tasks.list", "tasks.save", "projects.list", "pipelines.list", "contacts.connections", "storage.get"} {
		args := map[string]any{}
		if m == "contacts.get" || m == "contacts.connections" {
			args["id"] = saved[0].ID
		}
		if _, e := call(t, denied, m, args); e == nil || e.Code != CodePermissionDenied {
			t.Errorf("%s: want permission_denied, got %v", m, e)
		}
	}
	read := &Session{ID: "x-ext", Repo: store, Granted: NewPermissionSet("contacts.read")}
	out, e := call(t, read, "contacts.list", map[string]any{})
	if e != nil || !json.Valid(out) {
		t.Fatalf("list: %v", e)
	}
	if _, e := call(t, read, "contacts.get", map[string]any{"id": saved[0].ID}); e != nil {
		t.Fatal(e)
	}
	// contacts.read does not reach organizations, by list or by id.
	if _, e := call(t, read, "contacts.list", map[string]any{"kind": "organization"}); e == nil || e.Code != CodePermissionDenied {
		t.Fatal("organization list allowed with contacts.read")
	}
	if out, e := call(t, read, "contacts.get", map[string]any{"id": saved[1].ID}); e == nil || len(out) != 0 {
		t.Fatal("organization record returned without organizations.read")
	}
	// Read permission does not allow writing.
	if _, e := call(t, read, "contacts.save", saved[0]); e == nil || e.Code != CodePermissionDenied {
		t.Fatal("write allowed with read grant")
	}
	// Unknown methods and fields are rejected rather than ignored.
	if _, e := call(t, read, "db.exec", map[string]any{"sql": "DROP TABLE entities"}); e == nil || e.Code != CodeUnknownMethod {
		t.Fatalf("db access: %v", e)
	}
	if _, e := call(t, read, "contacts.list", map[string]any{"sql": "x"}); e == nil || e.Code != CodeInvalid {
		t.Fatalf("unknown field: %v", e)
	}
	if _, e := call(t, &Session{ID: "x-ext"}, "contacts.list", nil); e == nil {
		t.Fatal("no repository must not panic or succeed")
	}
}

func TestHostWritesGoThroughRevisionChecks(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	c := domain.NewContact("person")
	c.DisplayName = "Grace Fictional"
	saved, _ := store.SaveContacts(ctx, []domain.Contact{c}, "test")
	w := &Session{ID: "writer-ext", Repo: store, Granted: NewPermissionSet("contacts.write", "tasks.write", "tasks.read")}
	stale := saved[0]
	current := saved[0]
	current.Notes = "first"
	if _, e := call(t, w, "contacts.save", current); e != nil {
		t.Fatalf("valid write: %v", e)
	}
	stale.Notes = "second"
	if _, e := call(t, w, "contacts.save", stale); e == nil || e.Code != CodeConflict {
		t.Fatalf("stale revision accepted: %v", e)
	}
	bad := domain.NewContact("person") // no name: domain validation must still apply
	if _, e := call(t, w, "contacts.save", bad); e == nil || e.Code != CodeInvalid {
		t.Fatalf("validation bypassed: %v", e)
	}
	if _, e := call(t, w, "tasks.save", domain.Task{Title: "Call Ada"}); e != nil {
		t.Fatal(e)
	}
}

func TestHostStorageAndConfig(t *testing.T) {
	st, _ := OpenStorage(t.TempDir(), "s-ext", 0)
	s := &Session{ID: "s-ext", Granted: NewPermissionSet("storage.extension"), Storage: st, Config: map[string]any{"days": 3}}
	if _, e := call(t, s, "storage.put", map[string]any{"key": "n", "value": "1"}); e != nil {
		t.Fatal(e)
	}
	out, _ := call(t, s, "storage.get", map[string]any{"key": "n"})
	if string(out) != `{"found":true,"value":"1"}` {
		t.Fatal(string(out))
	}
	if _, e := call(t, s, "storage.put", map[string]any{"key": "../x", "value": "1"}); e == nil {
		t.Fatal("bad key accepted")
	}
	cfg, e := call(t, &Session{ID: "s-ext", Config: map[string]any{"days": 3}}, "config.get", nil)
	if e != nil || string(cfg) != `{"days":3}` {
		t.Fatalf("config.get needs no permission: %v %s", e, cfg)
	}
	if _, e := call(t, &Session{ID: "s-ext", Storage: st}, "storage.get", map[string]any{"key": "n"}); e == nil || e.Code != CodePermissionDenied {
		t.Fatal("storage without grant")
	}
}

type denyAll struct{}

func (denyAll) Check(_ context.Context, c string) (EntitlementDecision, error) {
	return EntitlementDecision{Capability: c}, nil
}

func TestHostRechecksEntitlementOnEveryCall(t *testing.T) {
	s := &Session{ID: "p-ext", Repo: newStore(t), Granted: NewPermissionSet("contacts.read"), Entitlement: "pro.x", Entitled: denyAll{}}
	if _, e := call(t, s, "contacts.list", nil); e == nil || e.Code != CodeEntitlement {
		t.Fatalf("entitlement not enforced at the service boundary: %v", e)
	}
}

func TestProjectsSaveNeedsWriteAndValidates(t *testing.T) {
	store := newStore(t)
	denied := &Session{ID: "p-ext", Repo: store, Granted: NewPermissionSet("projects.read")}
	if _, e := call(t, denied, "projects.save", map[string]any{"Name": "X"}); e == nil || e.Code != CodePermissionDenied {
		t.Fatalf("write allowed with read grant: %v", e)
	}
	w := &Session{ID: "p-ext", Repo: store, Granted: NewPermissionSet("projects.write", "projects.read")}
	out, e := call(t, w, "projects.save", map[string]any{"Name": "Onboarding"})
	if e != nil || !strings.Contains(string(out), "Onboarding") {
		t.Fatalf("%s %v", out, e)
	}
}
