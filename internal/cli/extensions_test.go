package cli

import (
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
