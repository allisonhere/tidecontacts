package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

// installDemo builds the real example extension into the extension directory.
func installDemo(t *testing.T) {
	t.Helper()
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("builds the demonstration extension")
	}
	dest := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "tidecontacts", "extensions", "relationship-digest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile("../../examples/extensions/relationship-digest/extension.toml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "extension.toml"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", filepath.Join(dest, "relationship-digest"), "./examples/extensions/relationship-digest")
	cmd.Dir = "../.."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build demo: %v\n%s", err, out)
	}
}

type content struct {
	Title string
	Lines []struct{ Label, Text, Level string }
}

func TestExtensionsWithoutAnyInstalled(t *testing.T) {
	h := setup(t)
	var list []map[string]any
	if err := json.Unmarshal(h.run(0, "", "extensions", "list").Data, &list); err != nil || len(list) != 0 {
		t.Fatalf("expected an empty list: %v %v", list, err)
	}
	var caps []struct {
		ID, Provider string
		Available    bool
	}
	_ = json.Unmarshal(h.run(0, "", "capabilities", "list").Data, &caps)
	if len(caps) == 0 || caps[0].Provider != "core" || !caps[0].Available {
		t.Fatalf("core capabilities missing: %+v", caps)
	}
	h.run(4, "", "extensions", "inspect", "nothing-here")
	h.run(2, "", "extensions", "bogus")
	h.run(2, "", "extensions", "enable")
	h.run(2, "", "capabilities", "enable")
}

func TestRelationshipDigestEndToEnd(t *testing.T) {
	h := setup(t)
	installDemo(t)
	var list []struct {
		ID, Status string
		Commands   []struct{ ID string }
	}
	_ = json.Unmarshal(h.run(0, "", "extensions", "list").Data, &list)
	if len(list) != 1 || list[0].ID != "relationship-digest" || list[0].Status != "disabled" {
		t.Fatalf("discovery: %+v", list)
	}
	var perms []struct {
		ID          string
		Granted     bool
		Enforcement string
	}
	_ = json.Unmarshal(h.run(0, "", "extensions", "permissions", "relationship-digest").Data, &perms)
	if len(perms) != 6 {
		t.Fatalf("permissions: %+v", perms)
	}
	for _, p := range perms {
		if p.Granted {
			t.Fatalf("%s granted before approval", p.ID)
		}
	}
	// Disabled and unapproved: nothing runs, enabling needs explicit consent.
	h.run(6, "", "extensions", "run", "relationship-digest", "digest")
	h.run(6, "", "extensions", "enable", "relationship-digest")
	h.run(0, "", "extensions", "enable", "relationship-digest", "--yes")

	ctx := context.Background()
	s, err := sqlite.Open(ctx, h.db)
	if err != nil {
		t.Fatal(err)
	}
	var people []domain.Contact
	for _, n := range []string{"Ada Fictional", "Grace Fictional", "Linus Fictional"} {
		c := domain.NewContact("person")
		c.DisplayName = n
		people = append(people, c)
	}
	saved, err := s.SaveContacts(ctx, people, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveActivity(ctx, domain.Activity{Type: "note", Title: "Coffee", OccurredAt: time.Now(), Entities: []domain.Ref{{Kind: "person", ID: saved[0].ID}}}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	var c content
	if err := json.Unmarshal(h.run(0, "", "extensions", "run", "relationship-digest", "digest").Data, &c); err != nil {
		t.Fatal(err)
	}
	var text []string
	for _, l := range c.Lines {
		text = append(text, l.Label+":"+l.Text)
	}
	joined := strings.Join(text, "|")
	if c.Title != "Relationship digest" || !strings.Contains(joined, "People:3") || !strings.Contains(joined, "2 without recent activity") || strings.Contains(joined, "Ada") || !strings.Contains(joined, "Grace Fictional") {
		t.Fatalf("digest: %s", joined)
	}
	// An operation outside the grant is refused by the host.
	_ = json.Unmarshal(h.run(0, "", "extensions", "run", "relationship-digest", "probe").Data, &c)
	if !strings.Contains(c.Lines[0].Text, "permission_denied") {
		t.Fatalf("probe: %+v", c)
	}
	// Capability discovery follows the lifecycle.
	var caps []struct {
		ID        string
		Available bool
		Provider  string
	}
	_ = json.Unmarshal(h.run(0, "", "capabilities", "list").Data, &caps)
	found := false
	for _, cp := range caps {
		if cp.ID == "relationship-digest.summary" {
			found = cp.Available && cp.Provider == "relationship-digest"
		}
	}
	if !found {
		t.Fatalf("capability not available while enabled: %+v", caps)
	}
	var cmds []struct {
		Extension, ID string
		Available     bool
	}
	_ = json.Unmarshal(h.run(0, "", "extensions", "commands").Data, &cmds)
	if len(cmds) != 2 || !cmds[0].Available {
		t.Fatalf("commands: %+v", cmds)
	}
	// Configuration is validated by the manifest schema.
	h.run(3, "", "extensions", "config", "relationship-digest", "--set", "window_days=0")
	h.run(0, "", "extensions", "config", "relationship-digest", "--set", "window_days=7")
	h.run(4, "", "extensions", "config", "relationship-digest", "--set", "nope=1")
	// Disable keeps data; the command no longer runs; core data is untouched.
	h.run(0, "", "extensions", "disable", "relationship-digest")
	h.run(1, "", "extensions", "run", "relationship-digest", "digest")
	h.run(2, "", "extensions", "purge-data", "relationship-digest")
	h.run(0, "", "extensions", "purge-data", "relationship-digest", "--yes")
	if got := h.run(0, "", "contacts", "list"); got.Meta.Count != 3 {
		t.Fatalf("core data changed: %d", got.Meta.Count)
	}
}

// scriptExt installs a shell extension contributing the "deals" CLI resource.
func scriptExt(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script extensions are Unix only")
	}
	dest := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "tidecontacts", "extensions", "deals-ext")
	_ = os.MkdirAll(dest, 0o755)
	_ = os.WriteFile(filepath.Join(dest, "extension.toml"), []byte("id = \"deals-ext\"\nname = \"Deals\"\nversion = \"1.0.0\"\napi = 1\ncommand = \"run.sh\"\npermissions = [\"cli.register\"]\n[[cli]]\nresource = \"deals\"\nactions = [\"list\"]\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dest, "run.sh"), []byte("#!/bin/sh\n"+script), 0o755)
}

const respond = "read line\nid=$(printf '%s' \"$line\" | sed 's/.*\"request_id\":\"\\([^\"]*\\)\".*/\\1/')\n"

func TestExtensionContributedCLIResource(t *testing.T) {
	h := setup(t)
	scriptExt(t, respond+"case \"$line\" in\n*cli.schema*) printf '{\"api\":1,\"type\":\"response\",\"request_id\":\"%s\",\"ok\":true,\"data\":{\"type\":\"object\"}}\\n' \"$id\";;\n*'\"argv\":[\"bad\"'*) printf '{\"api\":1,\"type\":\"response\",\"request_id\":\"%s\",\"ok\":false,\"error\":{\"code\":\"usage\",\"message\":\"nope\"}}\\n' \"$id\";;\n*'\"argv\":[\"stale\"'*) printf '{\"api\":1,\"type\":\"response\",\"request_id\":\"%s\",\"ok\":false,\"error\":{\"code\":\"conflict\",\"message\":\"changed\"}}\\n' \"$id\";;\n*) printf '{\"api\":1,\"type\":\"response\",\"request_id\":\"%s\",\"ok\":true,\"data\":{\"data\":[{\"id\":\"d1\"}],\"count\":1,\"etag\":\"abc\"}}\\n' \"$id\";;\nesac\n")
	// Not enabled yet: the resource does not exist, exactly like any unknown resource.
	h.run(2, "", "deals", "list")
	h.run(0, "", "extensions", "enable", "deals-ext", "--yes")
	got := h.run(0, "", "deals", "list", "--limit", "5")
	if got.Meta.Resource != "deals" || got.Meta.Action != "list" || got.Meta.Count != 1 || got.ETag != "abc" || !strings.Contains(string(got.Data), `"d1"`) {
		t.Fatalf("%+v %s", got.Meta, got.Data)
	}
	h.run(2, "", "deals", "bad")
	h.run(5, "", "deals", "stale")
	var schema struct{ Type string }
	_ = json.Unmarshal(h.run(0, "", "schema", "deals").Data, &schema)
	if schema.Type != "object" {
		t.Fatalf("schema: %+v", schema)
	}
	// Built-in resources are never routed to extensions, and the host's own flags still work.
	if n := h.run(0, "", "contacts", "list").Meta.Count; n != 0 {
		t.Fatal("unexpected contacts")
	}
	h.run(0, "", "extensions", "disable", "deals-ext")
	h.run(2, "", "deals", "list")
}

func TestExtensionResourceNeedsEntitlementWhenDeclared(t *testing.T) {
	h := setup(t)
	scriptExt(t, respond+"printf '{\"api\":1,\"type\":\"response\",\"request_id\":\"%s\",\"ok\":true,\"data\":{\"data\":[]}}\\n' \"$id\"\n")
	dir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "tidecontacts", "extensions", "deals-ext")
	b, _ := os.ReadFile(filepath.Join(dir, "extension.toml"))
	_ = os.WriteFile(filepath.Join(dir, "extension.toml"), append([]byte("entitlement = \"pro.deals\"\n"), b...), 0o644)
	// Entitlement is a top-level key, so it must precede tables; rewrite properly.
	_ = os.WriteFile(filepath.Join(dir, "extension.toml"), []byte("id = \"deals-ext\"\nname = \"Deals\"\nversion = \"1.0.0\"\napi = 1\ncommand = \"run.sh\"\nentitlement = \"pro.deals\"\npermissions = [\"cli.register\"]\n[[cli]]\nresource = \"deals\"\n"), 0o644)
	h.run(1, "", "extensions", "enable", "deals-ext", "--yes") // community provider denies pro.deals
	t.Setenv("TIDECONTACTS_DEV_ENTITLEMENTS", "pro.deals")
	h.run(0, "", "extensions", "enable", "deals-ext", "--yes")
	h.run(0, "", "deals", "list")
	t.Setenv("TIDECONTACTS_DEV_ENTITLEMENTS", "")
	h.run(2, "", "deals", "list") // entitlement gone: the resource is no longer offered, data untouched
}

func TestExtensionResourceAfterLeadingHostFlags(t *testing.T) {
	h := setup(t)
	scriptExt(t, respond+"printf '{\"api\":1,\"type\":\"response\",\"request_id\":\"%s\",\"ok\":true,\"data\":{\"data\":[{\"id\":\"d1\"}],\"count\":1}}\\n' \"$id\"\n")
	h.run(0, "", "extensions", "enable", "deals-ext", "--yes")
	// The harness appends --database; also exercise flags before the resource.
	if got := h.run(0, "", "deals", "list"); got.Meta.Resource != "deals" {
		t.Fatalf("%+v", got.Meta)
	}
	var out, errb bytes.Buffer
	err := Run(context.Background(), []string{"--database", h.db, "deals", "list"}, strings.NewReader(""), &out, &errb)
	if err != nil || !strings.Contains(out.String(), `"resource":"deals"`) {
		t.Fatalf("a leading --database hid the extension resource: %v %s", err, errb.String())
	}
	if !Handles([]string{"--demo", "deals", "list"}) || !Handles([]string{"--database", "/x", "deals", "list"}) {
		t.Fatal("Handles must see a resource after leading host flags")
	}
	if Handles([]string{"--demo"}) || Handles([]string{"--check"}) {
		t.Fatal("TUI flags must not be treated as CLI commands")
	}
}

func TestExtensionRawOutputRequiresOptIn(t *testing.T) {
	h := setup(t)
	scriptExt(t, respond+`printf '%s\n' '{"api":1,"type":"response","request_id":"'"$id"'","ok":true,"data":{"output":"one\u001b\n二\n"}}'`+"\n")
	h.run(0, "", "extensions", "enable", "deals-ext", "--yes")
	h.run(1, "", "deals", "list")
	var out, errb bytes.Buffer
	err := Run(context.Background(), []string{"--database", h.db, "deals", "list", "--raw"}, strings.NewReader(""), &out, &errb)
	if err != nil || out.String() != "one\n二\n" {
		t.Fatalf("raw output: %q; error: %v %s", out.String(), err, errb.String())
	}
}
