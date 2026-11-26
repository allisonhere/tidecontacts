package ext

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/domain"
)

func TestManifestContributionValidation(t *testing.T) {
	base := "id = \"demo-one\"\nname = \"Demo\"\nversion = \"1.0.0\"\napi = 1\ncommand = \"run\"\n"
	bad := map[string]string{
		"workspace w/o perm":   base + "[[workspaces]]\nid = \"sales\"\ntitle = \"Sales\"\n",
		"cli w/o perm":         base + "[[cli]]\nresource = \"deals\"\n",
		"cli reserved":         base + "permissions = [\"cli.register\"]\n[[cli]]\nresource = \"contacts\"\n",
		"signals w/o perm":     base + "signals = true\n",
		"publish w/o perm":     base + "[[published_events]]\ntype = \"deal.won\"\nentity_type = \"deal\"\n",
		"publish core clash":   base + "permissions = [\"events.publish\"]\n[[published_events]]\ntype = \"contact.created\"\nentity_type = \"contact\"\n",
		"publish malformed":    base + "permissions = [\"events.publish\"]\n[[published_events]]\ntype = \"Deal Won\"\nentity_type = \"deal\"\n",
		"sql w/o perm":         base + "[storage]\nsql = true\n",
		"timeout too large":    base + "timeout_seconds = 300\n",
		"four workspaces":      base + "permissions = [\"ui.register\"]\n[[workspaces]]\nid = \"aaa\"\ntitle = \"A\"\n[[workspaces]]\nid = \"bbb\"\ntitle = \"B\"\n[[workspaces]]\nid = \"ccc\"\ntitle = \"C\"\n[[workspaces]]\nid = \"ddd\"\ntitle = \"D\"\n",
		"duplicate workspaces": base + "permissions = [\"ui.register\"]\n[[workspaces]]\nid = \"aaa\"\ntitle = \"A\"\n[[workspaces]]\nid = \"aaa\"\ntitle = \"B\"\n",
	}
	for name, body := range bad {
		if _, err := ParseManifest([]byte(body)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	good := base + "timeout_seconds = 30\npermissions = [\"ui.register\", \"cli.register\", \"signals.provide\", \"events.publish\", \"storage.sql\"]\nsignals = true\n[storage]\nsql = true\n[[workspaces]]\nid = \"sales\"\ntitle = \"Sales\"\n[[cli]]\nresource = \"deals\"\nactions = [\"list\"]\n[[published_events]]\ntype = \"deal.won\"\nentity_type = \"deal\"\n"
	if _, err := ParseManifest([]byte(good)); err != nil {
		t.Fatal(err)
	}
}

func TestSanitizeViewBoundsAndSafety(t *testing.T) {
	v := View{Title: "T\x1b[31m", Tab: "bad id", Open: "open",
		Tiles:    make([]Tile, 50),
		Actions:  []Action{{Key: "n", Label: "New", ID: "new"}, {Key: "q", Label: "Quit", ID: "quit"}, {Key: "esc", Label: "x", ID: "x"}, {Key: "n", Label: "Dup", ID: "dup"}, {Key: "ctrl+c", Label: "x", ID: "y"}, {Key: "é", Label: "x", ID: "z"}, {Key: "e", Label: "Edit", ID: "e", Scope: "weird"}},
		Board:    &Board{Columns: []Column{{ID: "c1", Title: "Stage\x00", Tone: "neon", Cards: []Card{{ID: "ok", Title: "A\x1b]0;x\x07", Progress: 400, Tone: "danger"}, {ID: "bad id", Title: "dropped"}}}, {ID: "../x", Title: "dropped"}}},
		Sections: []Section{{Lines: []SLine{{Text: "x", Target: &Target{Kind: "shell", ID: "1"}}, {Text: "y", Target: &Target{Kind: "contact", ID: "abc-1"}}, {Text: "z", Target: &Target{Kind: "contact", ID: "../etc"}}}}},
	}
	out := SanitizeView(v)
	if out.Title != "T" || out.Tab != "" || len(out.Tiles) != maxTiles {
		t.Fatalf("basic: %+v", out)
	}
	var keys []string
	for _, a := range out.Actions {
		keys = append(keys, a.Key)
		if a.Key == "e" && a.Scope != "view" {
			t.Fatal("unknown scope kept")
		}
	}
	if strings.Join(keys, "") != "ne" {
		t.Fatalf("reserved/duplicate/invalid keys survived: %v", keys)
	}
	c := out.Board.Columns
	if len(c) != 1 || c[0].Tone != "" || len(c[0].Cards) != 1 || c[0].Cards[0].Progress != -1 || strings.ContainsAny(c[0].Cards[0].Title, "\x1b\x07") || c[0].Cards[0].Tone != "danger" {
		t.Fatalf("board: %+v", c)
	}
	l := out.Sections[0].Lines
	if l[0].Target != nil || l[1].Target == nil || l[2].Target != nil {
		t.Fatalf("targets: %+v", l)
	}
}

func TestSanitizeFormAndResult(t *testing.T) {
	f := SanitizeForm(Form{ID: "opp", Title: "New\x1b[1m", Fields: []FormField{{Key: "title", Label: "Title", Type: "weird", Value: "v\x1b[0m"}, {Key: "title", Label: "dup"}, {Key: "bad key", Label: "x"}, {Key: "who", Type: "contact", Refs: []RefItem{{ID: "u-1", Name: "A\x07"}, {ID: "../x"}}}}})
	if f.Title != "New" || len(f.Fields) != 2 || f.Fields[0].Type != "text" || f.Fields[0].Value != "v" || len(f.Fields[1].Refs) != 1 || f.Fields[1].Refs[0].Name != "A" {
		t.Fatalf("%+v", f)
	}
	r := SanitizeResult(ActionResult{Notice: "ok\x1b[31m", Open: &Target{Kind: "ext", ID: "x y"}, Errors: map[string]string{"title": "bad\x1b[0m", "no way": "x"}})
	if r.Notice != "ok" || r.Open != nil || len(r.Errors) != 1 || r.Errors["title"] != "bad" {
		t.Fatalf("%+v", r)
	}
	s := SanitizeSignals([]Signal{{Key: "s1", Severity: "loud", Title: "t\x1b[1m", Facts: make([]string, 20)}, {Key: "bad key"}})
	if len(s) != 1 || s[0].Severity != "info" || s[0].Title != "t" || len(s[0].Facts) != 8 {
		t.Fatalf("%+v", s)
	}
}

func TestDevEntitlements(t *testing.T) {
	ctx := context.Background()
	for _, bad := range []string{"contacts.read", "pro.", "pro. x", "core.contacts"} {
		if _, err := ParseDevEntitlements(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	d, err := ParseDevEntitlements("pro.a, pro.b")
	if err != nil {
		t.Fatal(err)
	}
	for cap, want := range map[string]bool{"pro.a": true, "pro.b": true, "pro.c": false, "core.contacts": true, "demo.x": true} {
		got, _ := d.Check(ctx, cap)
		if got.Allowed != want {
			t.Errorf("%s: %v", cap, got)
		}
		if got.Allowed && strings.HasPrefix(cap, "pro.") && (got.Source != "development" || !strings.Contains(got.Reason, "not a license")) {
			t.Errorf("dev decision must say so: %+v", got)
		}
	}
	all, _ := ParseDevEntitlements("*")
	if g, _ := all.Check(ctx, "pro.zzz"); !g.Allowed {
		t.Fatal("* should grant any pro.")
	}
	t.Setenv(DevEntitlementsEnv, "")
	if p, note := ProviderFromEnv(); note != "" || func() bool { g, _ := p.Check(ctx, "pro.a"); return g.Allowed }() {
		t.Fatal("no env must mean community only")
	}
	t.Setenv(DevEntitlementsEnv, "pro.a")
	if _, note := ProviderFromEnv(); !strings.Contains(note, "not a license") {
		t.Fatalf("notice: %q", note)
	}
	t.Setenv(DevEntitlementsEnv, "bogus")
	if p, note := ProviderFromEnv(); !strings.Contains(note, "ignored") || func() bool { g, _ := p.Check(ctx, "pro.a"); return g.Allowed }() {
		t.Fatal("a malformed variable must not grant anything")
	}
}

func TestPartialEntitlementEnablesOnlyWhatItCovers(t *testing.T) {
	f := newFixture(t)
	f.add("multi-ext", "[[provides]]\nid = \"pro.alpha\"\nversion = 1\n[[provides]]\nid = \"pro.beta\"\nversion = 1\n[[provides]]\nid = \"multi-ext.free\"\nversion = 1\n", okScript("{}"))
	dev, _ := ParseDevEntitlements("pro.alpha")
	m := f.manager(Options{Entitlements: dev})
	if err := m.Enable(context.Background(), "multi-ext", true); err != nil {
		t.Fatal(err)
	}
	avail := map[string]bool{}
	for _, c := range m.Capabilities() {
		avail[c.ID] = c.Available
	}
	if !avail["pro.alpha"] || avail["pro.beta"] || !avail["multi-ext.free"] {
		t.Fatalf("per-capability entitlement wrong: %v", avail)
	}
}

// jsonScript answers every request with a fixed JSON value.
func jsonScript(data string) string { return okScript(data) }

func uiManifest(extra string) string {
	return "permissions = [\"ui.register\", \"commands.register\", \"cli.register\", \"signals.provide\"]\nsignals = true\n[[workspaces]]\nid = \"sales\"\ntitle = \"Sales\"\n[[cli]]\nresource = \"deals\"\nactions = [\"list\"]\n[[commands]]\nid = \"go\"\ntitle = \"Go\"\n" + extra
}

func TestManagerUICallsAndContributions(t *testing.T) {
	f := newFixture(t)
	f.add("sales-ext", uiManifest(""), okScript(`{"title":"T","tab":"overview","tiles":[{"label":"Pipeline","value":"$1"}],"actions":[{"key":"q","label":"x","id":"x"},{"key":"n","label":"New","id":"new"}]}`))
	m := f.manager(Options{})
	ctx := context.Background()
	if len(m.Workspaces()) != 0 || len(m.CLIResources()) != 0 {
		t.Fatal("contributions visible while disabled")
	}
	if _, err := m.RenderView(ctx, "sales-ext", ViewRequest{Workspace: "sales"}); err == nil {
		t.Fatal("rendered a disabled extension")
	}
	if err := m.Enable(ctx, "sales-ext", true); err != nil {
		t.Fatal(err)
	}
	if ws := m.Workspaces(); len(ws) != 1 || ws[0].Title != "Sales" {
		t.Fatalf("%+v", ws)
	}
	if r := m.CLIResources(); len(r) != 1 || r[0].Resource != "deals" {
		t.Fatalf("%+v", r)
	}
	v, err := m.RenderView(ctx, "sales-ext", ViewRequest{Workspace: "sales"})
	if err != nil || len(v.Tiles) != 1 || len(v.Actions) != 1 || v.Actions[0].Key != "n" {
		t.Fatalf("%+v %v", v, err)
	}
	if _, err := m.RenderView(ctx, "sales-ext", ViewRequest{Workspace: "undeclared"}); err == nil {
		t.Fatal("undeclared workspace rendered")
	}
	m.mu.Lock() // a revoked permission must stop the call even if the manifest still names the workspace
	m.recs["sales-ext"].entry.Granted = []string{"commands.register"}
	m.mu.Unlock()
	if _, err := m.RenderView(ctx, "sales-ext", ViewRequest{Workspace: "sales"}); err == nil || !strings.Contains(err.Error(), "ui.register") {
		t.Fatalf("permission not enforced: %v", err)
	}
	if len(m.Workspaces()) != 0 {
		t.Fatal("workspace listed without ui.register")
	}
}

func TestManagerCLISignalsAndCommandResult(t *testing.T) {
	f := newFixture(t)
	f.add("sales-ext", uiManifest(""), okScript(`{"data":[{"id":"1"}],"count":1,"etag":"e\u0001","warnings":["w"],"notice":"hi","form":{"id":"f","title":"New","fields":[{"key":"title","type":"text"}]},"key":"s1","type":"stale","severity":"warn"}`))
	m := f.manager(Options{})
	ctx := context.Background()
	if err := m.Enable(ctx, "sales-ext", true); err != nil {
		t.Fatal(err)
	}
	r, err := m.RunCLI(ctx, CLIRequest{Resource: "deals", Argv: []string{"list"}})
	if err != nil || r.Count != 1 || !strings.Contains(string(r.Data), `"id":"1"`) || strings.ContainsRune(r.ETag, 1) {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := m.RunCLI(ctx, CLIRequest{Resource: "contacts"}); err == nil {
		t.Fatal("a built-in resource was routed to an extension")
	}
	res, err := m.RunCommandResult(ctx, "sales-ext", "go", map[string]string{"context_contact": "c1"})
	if err != nil || res.Form == nil || res.Form.ID != "f" || res.Notice != "hi" {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := m.RunCommandResult(ctx, "sales-ext", "nope", nil); err == nil {
		t.Fatal("undeclared command ran")
	}
}

func TestSignalsFailureIsolated(t *testing.T) {
	f := newFixture(t)
	f.add("good-ext", uiManifest(""), okScript(`[{"key":"s1","type":"x","title":"T","severity":"danger","reason":"r"}]`))
	dir := f.add("bad-ext", uiManifest(""), okScript(`[]`))
	m := f.manager(Options{})
	ctx := context.Background()
	_ = m.Enable(ctx, "good-ext", true)
	_ = m.Enable(ctx, "bad-ext", true)
	_ = os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\nexit 1\n"), 0o755)
	sum, _ := fileSHA256(filepath.Join(dir, "run.sh"))
	_ = m.update("bad-ext", func(e *stateEntry) { e.ExecSHA256 = sum })
	_ = m.Discover(ctx)
	sigs := m.Signals(ctx)
	if len(sigs) != 1 || sigs[0].Extension != "good-ext" || sigs[0].Severity != "danger" {
		t.Fatalf("%+v", sigs)
	}
}

func TestRecordsResolveAndEvents(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	p := domain.NewContact("person")
	p.DisplayName = "Ada Fictional"
	p.Methods = []domain.Method{{Kind: "email", Value: "ada@example.com"}}
	o := domain.NewContact("organization")
	o.DisplayName = "Example Works"
	saved, err := store.SaveContacts(ctx, []domain.Contact{p, o}, "test")
	if err != nil {
		t.Fatal(err)
	}
	task, _ := store.SaveTask(ctx, domain.Task{Title: "Call Ada"})
	bus := NewBus()
	sub := bus.Subscribe(8)
	s := &Session{ID: "r-ext", Repo: store, Granted: NewPermissionSet("contacts.read", "tasks.read", "events.publish"), Bus: bus, Published: map[string]string{"deal.won": "deal"}, Entitled: CommunityEntitlements{}}
	out, e := call(t, s, "records.resolve", map[string]any{"kind": "contact", "ids": []string{saved[0].ID, saved[1].ID, "missing"}})
	if e != nil {
		t.Fatal(e)
	}
	got := string(out)
	if !strings.Contains(got, "ada@example.com") || strings.Contains(got, "Example Works") || strings.Count(got, `"missing":true`) != 2 {
		t.Fatalf("resolve leaked or mis-reported: %s", got)
	}
	if out, _ := call(t, s, "records.resolve", map[string]any{"kind": "task", "ids": []string{task.ID}}); !strings.Contains(string(out), "Call Ada") {
		t.Fatalf("%s", out)
	}
	for _, k := range []string{"organization", "activity", "project"} {
		if _, e := call(t, s, "records.resolve", map[string]any{"kind": k, "ids": []string{"x"}}); e == nil || e.Code != CodePermissionDenied {
			t.Errorf("%s resolved without permission", k)
		}
	}
	if _, e := call(t, s, "records.resolve", map[string]any{"kind": "contact", "ids": make([]string, maxResolve+1)}); e == nil {
		t.Fatal("unbounded resolve accepted")
	}
	// Events: declared types only, identifiers only, source stamped by the host.
	if _, e := call(t, s, "events.publish", map[string]any{"type": "deal.lost", "entity_id": "d1"}); e == nil || e.Code != CodePermissionDenied {
		t.Fatal("undeclared event published")
	}
	if _, e := call(t, s, "events.publish", map[string]any{"type": "contact.created", "entity_id": "d1"}); e == nil {
		t.Fatal("core event forged")
	}
	if _, e := call(t, s, "events.publish", map[string]any{"type": "deal.won", "entity_id": "d1", "revision": 3, "amount": 100}); e == nil || e.Code != CodeInvalid {
		t.Fatal("extra payload fields (financial data) must be rejected")
	}
	if _, e := call(t, s, "events.publish", map[string]any{"type": "deal.won", "entity_id": "d1", "revision": 3, "correlation_id": "c-1"}); e != nil {
		t.Fatal(e)
	}
	ev := <-sub.C
	if ev.Type != "deal.won" || ev.EntityType != "deal" || ev.Source != "extension:r-ext" || ev.Revision != 3 || ev.CorrelationID != "c-1" || !IsExtensionSource(ev.Source) {
		t.Fatalf("%+v", ev)
	}
	noPerm := &Session{ID: "r-ext", Repo: store, Published: map[string]string{"deal.won": "deal"}, Bus: bus}
	if _, e := call(t, noPerm, "events.publish", map[string]any{"type": "deal.won", "entity_id": "d1"}); e == nil || e.Code != CodePermissionDenied {
		t.Fatal("published without events.publish")
	}
	// entitlement.check reports the provider's answer without needing a permission.
	out, e = call(t, s, "entitlement.check", map[string]any{"capability": "pro.sales"})
	if e != nil || !strings.Contains(string(out), `"allowed":false`) {
		t.Fatalf("%s %v", out, e)
	}
	if _, e := call(t, s, "entitlement.check", map[string]any{"capability": "Bad Cap"}); e == nil {
		t.Fatal("malformed capability accepted")
	}
}

func TestPublishIsRateLimitedPerRequest(t *testing.T) {
	s := &Session{ID: "r-ext", Granted: NewPermissionSet("events.publish"), Published: map[string]string{"deal.won": "deal"}}
	var last *ProtocolError
	for i := 0; i < maxPublishPerCall+5; i++ {
		_, last = call(t, s, "events.publish", map[string]any{"type": "deal.won", "entity_id": "d1"})
	}
	if last == nil || last.Code != CodeQuota {
		t.Fatalf("flood not limited: %v", last)
	}
}

func TestDataDirectoryOnlyWithStorageSQL(t *testing.T) {
	f := newFixture(t)
	script := "read line\nid=$(printf '%s' \"$line\" | sed 's/.*\"request_id\":\"\\([^\"]*\\)\".*/\\1/')\nprintf '{\"api\":1,\"type\":\"response\",\"request_id\":\"%s\",\"ok\":true,\"data\":{\"lines\":[{\"text\":\"%s\"}]}}\\n' \"$id\" \"$TIDECONTACTS_EXTENSION_DATA\"\n"
	f.add("sql-ext", "permissions = [\"commands.register\", \"storage.sql\"]\n[storage]\nsql = true\n[[commands]]\nid = \"where\"\ntitle = \"Where\"\n", script)
	f.add("nosql-ext", "permissions = [\"commands.register\"]\n[[commands]]\nid = \"where\"\ntitle = \"Where\"\n", script)
	m := f.manager(Options{})
	ctx := context.Background()
	_ = m.Enable(ctx, "sql-ext", true)
	_ = m.Enable(ctx, "nosql-ext", true)
	c, err := m.RunCommand(ctx, "sql-ext", "where", nil)
	if err != nil || len(c.Lines) != 1 || c.Lines[0].Text != filepath.Join(f.data, "sql-ext") {
		t.Fatalf("data dir not provided: %+v %v", c.Lines, err)
	}
	if st, err := os.Stat(filepath.Join(f.data, "sql-ext")); err != nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("data dir not created 0700: %v %v", st, err)
	}
	if c, _ := m.RunCommand(ctx, "nosql-ext", "where", nil); len(c.Lines) != 1 || c.Lines[0].Text != "" {
		t.Fatalf("data dir leaked to an extension without storage.sql: %+v", c.Lines)
	}
	// Disabling keeps the data; only an explicit purge removes it.
	_ = os.WriteFile(filepath.Join(f.data, "sql-ext", "x.db"), []byte("data"), 0o600)
	_ = m.Disable(ctx, "sql-ext")
	if _, err := os.Stat(filepath.Join(f.data, "sql-ext", "x.db")); err != nil {
		t.Fatal("disable destroyed extension data")
	}
	if err := m.PurgeData(ctx, "sql-ext"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.data, "sql-ext")); err == nil {
		t.Fatal("purge left data behind")
	}
}

func TestRawCommandOutputCannotDriveTheTerminal(t *testing.T) {
	got := cleanOutput("a,b\r\n\x1b[2J\x07c\td\u202e\u009b1m\x00e\n")
	if want := "a,b\r\n[2Jc\td1me\n"; got != want {
		t.Fatalf("%q", got)
	}
}
