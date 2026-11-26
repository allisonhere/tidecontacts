package ext

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type testMCPEntitlements map[string]bool

func (e testMCPEntitlements) Check(_ context.Context, capability string) (EntitlementDecision, error) {
	return EntitlementDecision{Capability: capability, Allowed: e[capability]}, nil
}

func TestProfessionalMCPRequiresApprovedProviderAndEntitlement(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	entitled := testMCPEntitlements{"pro.opportunities": true}
	m := f.manager(Options{Entitlements: entitled})
	const manifest = `mcp = true
permissions = ["mcp.register", "storage.sql"]
[storage]
sql = true
[[provides]]
id = "pro.opportunities"
version = 1
[[provides]]
id = "pro.automation"
version = 1
`
	req := MCPRequest{Action: "opportunities_search", Actor: "local-profile", Input: json.RawMessage(`{}`)}
	assertDenied := func() {
		t.Helper()
		if got := m.MCPActions(ctx); len(got) != 0 {
			t.Fatalf("unexpected tools: %v", got)
		}
		_, err := m.RunMCP(ctx, req)
		var p *ProtocolError
		if !errors.As(err, &p) || p.Code != CodePermissionDenied {
			t.Fatalf("expected permission denial, got %v", err)
		}
	}
	assertDenied() // absent

	approve := func(id, dir string, granted ...string) {
		t.Helper()
		sum, err := fileSHA256(filepath.Join(dir, "run.sh"))
		if err != nil {
			t.Fatal(err)
		}
		if err := m.update(id, func(e *stateEntry) {
			e.Enabled, e.ExecSHA256, e.Granted = true, sum, granted
		}); err != nil {
			t.Fatal(err)
		}
	}
	spoof := f.add("spoof-ext", manifest, "exit 73\n")
	approve("spoof-ext", spoof, "mcp.register", "storage.sql")
	assertDenied() // wrong provider even if entitled and approved
	if err := os.Rename(spoof, filepath.Join(f.root, "disabled-spoof")); err != nil {
		t.Fatal(err)
	}
	pro := f.add(professionalMCPProvider, manifest, `read line
case "$line" in
  *'"method":"mcp.run"'*'"actor":"local-profile"'*) ;;
  *) exit 73 ;;
esac
id=$(printf '%s' "$line" | sed 's/.*"request_id":"\([^"]*\)".*/\1/')
printf '{"api":1,"type":"response","request_id":"%s","ok":true,"data":{}}\n' "$id"
`)
	assertDenied() // installed, disabled
	approve(professionalMCPProvider, pro, "storage.sql")
	assertDenied() // registered in manifest, not approved by user
	approve(professionalMCPProvider, pro, "mcp.register", "storage.sql")
	advertised := m.MCPActions(ctx)
	if !advertised[req.Action] || advertised["playbooks_start"] {
		t.Fatalf("incorrect per-feature advertisement: %v", advertised)
	}
	if _, err := m.RunMCP(ctx, req); err != nil {
		t.Fatalf("approved action did not use dedicated RPC: %v", err)
	}
	for _, bad := range []MCPRequest{{Action: "cli.run", Actor: req.Actor}, {Action: req.Action, Actor: "mcp:spoof"}, {Action: "playbooks_start", Actor: req.Actor}} {
		if _, err := m.RunMCP(ctx, bad); err == nil {
			t.Fatalf("accepted unsupported, spoofed or unentitled request: %+v", bad)
		}
	}
	delete(entitled, "pro.opportunities")
	assertDenied() // entitlement changed after initial advertisement
	entitled["pro.opportunities"] = true
	approve(professionalMCPProvider, pro, "storage.sql")
	assertDenied() // grant revoked after initial advertisement
}

func TestMCPSessionDeniesWriteAndAdministrativeCallbacks(t *testing.T) {
	s := &Session{mcpOnly: true, Repo: newStore(t), Entitled: allowAll{},
		Granted: NewPermissionSet("contacts.read", "contacts.write", "storage.kv", "tasks.write", "events.subscribe", "events.publish")}
	for _, method := range []string{"records.rule-write", "contacts.save", "activities.save", "tasks.save", "projects.save",
		"tasks.complete", "attention.act", "events.next", "events.settle", "events.replay", "events.dead", "events.publish",
		"storage.get", "storage.put", "config.get", "filesystem.read", "cli.run"} {
		if _, e := s.Call(context.Background(), method, nil); e == nil || e.Code != CodePermissionDenied {
			t.Fatalf("callback %s was not denied: %v", method, e)
		}
	}
	if _, e := s.Call(context.Background(), "contacts.list", nil); e != nil {
		t.Fatalf("granted read callback was denied: %v", e)
	}
}
