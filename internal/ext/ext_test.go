package ext

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const basic = "permissions = [\"commands.register\"]\n[[commands]]\nid = \"hello\"\ntitle = \"Hello\"\n"

func TestManifestValidation(t *testing.T) {
	good := "id = \"demo-one\"\nname = \"Demo\"\nversion = \"1.0.0\"\napi = 1\ncommand = \"run\"\n"
	if _, err := ParseManifest([]byte(good)); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"bad id":              strings.Replace(good, "demo-one", "Demo_One", 1),
		"unknown key":         good + "surprise = 1\n",
		"abs command":         strings.Replace(good, `"run"`, `"/bin/sh"`, 1),
		"traversal":           strings.Replace(good, `"run"`, `"../run"`, 1),
		"backslash":           strings.Replace(good, `"run"`, `"a\\b"`, 1),
		"unknown permission":  good + "permissions = [\"root.all\"]\n",
		"core namespace":      good + "[[provides]]\nid = \"core.contacts\"\nversion = 1\n",
		"foreign namespace":   good + "[[provides]]\nid = \"other.thing\"\nversion = 1\n",
		"command w/o perm":    good + "[[commands]]\nid = \"x\"\ntitle = \"X\"\n",
		"unsupported slot":    good + "permissions = [\"ui.register\"]\n[[panels]]\nid = \"p\"\ntitle = \"P\"\nslot = \"navigation\"\n",
		"event w/o read perm": good + "permissions = [\"events.subscribe\"]\n[events]\nsubscribe = [\"contact.created\"]\n",
		"unknown event":       good + "permissions = [\"events.subscribe\"]\n[events]\nsubscribe = [\"contact.exploded\"]\n",
		"storage w/o perm":    good + "[storage]\nrequired = true\n",
		"control chars":       strings.Replace(good, "Demo", "De\\u001b[31mmo", 1),
		"bad version":         strings.Replace(good, "1.0.0", "latest", 1),
		"duplicate perm":      good + "permissions = [\"contacts.read\", \"contacts.read\"]\n",
	}
	for name, body := range cases {
		if _, err := ParseManifest([]byte(body)); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
	if _, err := ParseManifest(make([]byte, maxManifestBytes+1)); err == nil {
		t.Error("oversized manifest accepted")
	}
}

func TestDiscoveryDuplicatesInvalidAndEmpty(t *testing.T) {
	f := newFixture(t)
	m := f.manager(Options{}) // no directory at all
	if len(m.List()) != 0 {
		t.Fatal("expected no extensions")
	}
	f.add("alpha-ext", basic, okScript("{}"))
	f.add("beta-ext", basic, okScript("{}"))
	// beta-ext's manifest claims alpha-ext's id: duplicate.
	b, _ := os.ReadFile(filepath.Join(f.dir, "beta-ext", ManifestFile))
	_ = os.WriteFile(filepath.Join(f.dir, "beta-ext", ManifestFile), []byte(strings.Replace(string(b), `id = "beta-ext"`, `id = "alpha-ext"`, 1)), 0o644)
	_ = os.MkdirAll(filepath.Join(f.dir, "broken"), 0o755)
	_ = os.WriteFile(filepath.Join(f.dir, "broken", ManifestFile), []byte("id = ["), 0o644)
	_ = m.Discover(context.Background())
	got := map[string]Info{}
	for _, i := range m.List() {
		got[i.ID] = i
	}
	if got["alpha-ext"].Status != StatusDisabled || got["beta-ext"].Status != StatusInvalid || got["broken"].Status != StatusInvalid {
		t.Fatalf("unexpected statuses: %+v", got)
	}
	if !strings.Contains(got["beta-ext"].StatusDetail, "duplicate") {
		t.Fatalf("detail: %q", got["beta-ext"].StatusDetail)
	}
}

func TestIncompatibleAPIAndHost(t *testing.T) {
	f := newFixture(t)
	f.add("future-ext", "min_host = \"99.0.0\"\n", okScript("{}"))
	d := f.add("api-ext", "", okScript("{}"))
	b, _ := os.ReadFile(filepath.Join(d, ManifestFile))
	_ = os.WriteFile(filepath.Join(d, ManifestFile), []byte(strings.Replace(string(b), "api = 1", "api = 2", 1)), 0o644)
	m := f.manager(Options{})
	for _, i := range m.List() {
		if i.Status != StatusIncompatible {
			t.Errorf("%s: %s", i.ID, i.Status)
		}
	}
	if err := m.Enable(context.Background(), "api-ext", true); err == nil {
		t.Fatal("incompatible extension enabled")
	}
}

func TestEnableRequiresApprovalAndPersists(t *testing.T) {
	f := newFixture(t)
	f.add("hello-ext", basic, okScript("{}"))
	m := f.manager(Options{})
	ctx := context.Background()
	var ar *ApprovalRequiredError
	if err := m.Enable(ctx, "hello-ext", false); !errors.As(err, &ar) || len(ar.Permissions) != 1 {
		t.Fatalf("want approval error, got %v", err)
	}
	if i, _ := m.Inspect("hello-ext"); i.Status != StatusDisabled {
		t.Fatal("enabled without approval")
	}
	if err := m.Enable(ctx, "hello-ext", true); err != nil {
		t.Fatal(err)
	}
	// A fresh manager (new process) sees it enabled with the stored grant.
	m2 := f.manager(Options{})
	if i, _ := m2.Inspect("hello-ext"); i.Status != StatusEnabled || i.Permissions[0].Granted != true {
		t.Fatalf("state not persisted: %+v", i)
	}
	if err := m2.Disable(ctx, "hello-ext"); err != nil {
		t.Fatal(err)
	}
	if i, _ := m2.Inspect("hello-ext"); i.Status != StatusDisabled {
		t.Fatal("not disabled")
	}
}

func TestPermissionChangeAndExecutableChangeNeedApproval(t *testing.T) {
	f := newFixture(t)
	dir := f.add("grow-ext", basic, okScript("{}"))
	m := f.manager(Options{})
	ctx := context.Background()
	if err := m.Enable(ctx, "grow-ext", true); err != nil {
		t.Fatal(err)
	}
	// "Update" the extension: it now asks for contacts.read too.
	b, _ := os.ReadFile(filepath.Join(dir, ManifestFile))
	_ = os.WriteFile(filepath.Join(dir, ManifestFile), []byte(strings.Replace(string(b), `["commands.register"]`, `["commands.register", "contacts.read"]`, 1)), 0o644)
	_ = m.Discover(ctx)
	i, _ := m.Inspect("grow-ext")
	if i.Status != StatusNeedsApproval || len(i.NeedsApproval) != 1 || i.NeedsApproval[0] != "contacts.read" {
		t.Fatalf("expected needs-approval: %+v", i)
	}
	if _, err := m.RunCommand(ctx, "grow-ext", "hello", nil); err == nil {
		t.Fatal("ran while permissions awaited approval")
	}
	if err := m.Enable(ctx, "grow-ext", false); err == nil {
		t.Fatal("re-enabled without approval")
	}
	if err := m.Enable(ctx, "grow-ext", true); err != nil {
		t.Fatal(err)
	}
	// Swapping the binary also needs approval.
	_ = os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"+okScript(`{"lines":[]}`)+"# changed\n"), 0o755)
	_ = m.Discover(ctx)
	if i, _ := m.Inspect("grow-ext"); i.Status != StatusNeedsApproval || !strings.Contains(i.StatusDetail, "executable") {
		t.Fatalf("binary swap not detected: %+v", i)
	}
}

func TestCommandRunsAndContentIsSanitized(t *testing.T) {
	f := newFixture(t)
	f.add("hello-ext", basic, okScript(`{"title":"Hi\u001b[31m","lines":[{"label":"a","text":"\u001b]0;pwn\u0007ok\u202e"},{"text":"x","level":"evil"}]}`))
	m := f.manager(Options{})
	ctx := context.Background()
	if err := m.Enable(ctx, "hello-ext", true); err != nil {
		t.Fatal(err)
	}
	c, err := m.RunCommand(ctx, "hello-ext", "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(c.Title+c.Lines[0].Text, "\x1b\x07\u202e") || c.Lines[0].Text != "ok" || c.Title != "Hi" || c.Lines[1].Level != "info" {
		t.Fatalf("unsanitized: %+v", c)
	}
	if _, err := m.RunCommand(ctx, "hello-ext", "nope", nil); err == nil {
		t.Fatal("undeclared command ran")
	}
	if len(m.Commands()) != 1 {
		t.Fatal("command not contributed")
	}
	_ = m.Disable(ctx, "hello-ext")
	if len(m.Commands()) != 0 {
		t.Fatal("disabled extension still contributes commands")
	}
}

func TestFailureModes(t *testing.T) {
	cases := map[string]struct{ script, want string }{
		"crash":       {"read line\nexit 3\n", "without a response"},
		"hang":        {"read line\nsleep 30\n", "timed out"},
		"garbage":     {"read line\necho not json\n", "malformed"},
		"wrong api":   {"read line\necho '{\"api\":9,\"type\":\"response\"}'\n", "unsupported api"},
		"wrong id":    {"read line\necho '{\"api\":1,\"type\":\"response\",\"request_id\":\"zzz\",\"ok\":true}'\n", "different request"},
		"bad type":    {"read line\necho '{\"api\":1,\"type\":\"exec\"}'\n", "unexpected message"},
		"oversized":   {"read line\nhead -c 3000000 /dev/zero | tr '\\0' 'a'\n", "exceeds"},
		"host flood":  {"read line\nwhile :; do echo '{\"api\":1,\"type\":\"host_call\",\"call_id\":\"1\",\"method\":\"config.get\"}'; read r; done\n", "host calls"},
		"ext error":   {"read line\nid=$(printf '%s' \"$line\" | sed 's/.*\"request_id\":\"\\([^\"]*\\)\".*/\\1/')\nprintf '{\"api\":1,\"type\":\"response\",\"request_id\":\"%s\",\"ok\":false,\"error\":%s}\\n' \"$id\" '{\"code\":\"nope\",\"message\":\"bad \\u001b[31mthing\"}'\n", "bad"},
		"no response": {"cat >/dev/null\n", "timed out"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			dir := f.add("fail-ext", basic, c.script)
			start := time.Now()
			_, err := Invoke(context.Background(), dir, "run.sh", "fail-ext", "command.run", map[string]any{}, time.Second, nil)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want %q", err, c.want)
			}
			if strings.ContainsRune(err.Error(), 0x1b) {
				t.Fatal("escape sequence leaked into error")
			}
			if time.Since(start) > 5*time.Second {
				t.Fatalf("took %s", time.Since(start))
			}
		})
	}
}

func TestHangingProcessGroupIsKilled(t *testing.T) {
	f := newFixture(t)
	marker := filepath.Join(f.root, "child-alive")
	dir := f.add("kill-ext", basic, "(sleep 1; touch "+marker+") &\nread line\nsleep 30\n")
	_, err := Invoke(context.Background(), dir, "run.sh", "kill-ext", "x", nil, 300*time.Millisecond, nil)
	if err == nil {
		t.Fatal("expected timeout")
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("child of a killed extension survived")
	}
}

func TestEnvironmentIsScrubbed(t *testing.T) {
	t.Setenv("SECRET_TOKEN", "hunter2")
	f := newFixture(t)
	dir := f.add("env-ext", basic, "read line\nid=$(printf '%s' \"$line\" | sed 's/.*\"request_id\":\"\\([^\"]*\\)\".*/\\1/')\nprintf '{\"api\":1,\"type\":\"response\",\"request_id\":\"%s\",\"ok\":true,\"data\":{\"home\":\"%s\",\"secret\":\"%s\",\"id\":\"%s\"}}\\n' \"$id\" \"$HOME\" \"$SECRET_TOKEN\" \"$TIDECONTACTS_EXTENSION_ID\"\n")
	res, err := Invoke(context.Background(), dir, "run.sh", "env-ext", "x", nil, 2*time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(res.Data), "hunter2") || !strings.Contains(string(res.Data), `"home":""`) || !strings.Contains(string(res.Data), "env-ext") {
		t.Fatalf("environment leaked: %s", res.Data)
	}
}

func TestExecutableContainment(t *testing.T) {
	f := newFixture(t)
	dir := f.add("link-ext", basic, okScript("{}"))
	outside := filepath.Join(f.root, "outside.sh")
	_ = os.WriteFile(outside, []byte("#!/bin/sh\n"), 0o755)
	_ = os.Symlink(outside, filepath.Join(dir, "escape.sh"))
	if _, err := ResolveExecutable(dir, "escape.sh"); err == nil {
		t.Fatal("symlink escape accepted")
	}
	if _, err := ResolveExecutable(dir, "../outside.sh"); err == nil {
		t.Fatal("traversal accepted")
	}
	_ = os.WriteFile(filepath.Join(dir, "plain"), []byte("x"), 0o644)
	if _, err := ResolveExecutable(dir, "plain"); err == nil {
		t.Fatal("non-executable accepted")
	}
}

func TestFailureBreakerAndRecovery(t *testing.T) {
	f := newFixture(t)
	dir := f.add("flaky-ext", basic, okScript(`{"lines":[]}`))
	m := f.manager(Options{})
	ctx := context.Background()
	if err := m.Enable(ctx, "flaky-ext", true); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\nexit 1\n"), 0o755)
	// Executable changed → host refuses to run it, no crash, no failure count.
	_ = m.Discover(ctx)
	if _, err := m.RunCommand(ctx, "flaky-ext", "hello", nil); err == nil {
		t.Fatal("changed binary ran")
	}
	// Approve the new binary, then let it fail repeatedly.
	if err := m.Enable(ctx, "flaky-ext", true); err == nil {
		t.Fatal("activation of a crashing extension should fail")
	}
	if i, _ := m.Inspect("flaky-ext"); i.Status == StatusEnabled {
		t.Fatal("failed activation left the extension enabled")
	}
}

func TestBreakerOpensAfterRepeatedFailures(t *testing.T) {
	f := newFixture(t)
	dir := f.add("flaky-ext", basic, okScript(`{"lines":[]}`))
	m := f.manager(Options{})
	ctx := context.Background()
	if err := m.Enable(ctx, "flaky-ext", true); err != nil {
		t.Fatal(err)
	}
	// Break it in place without changing the recorded digest by faking approval of the new script.
	_ = os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\nread l\nexit 1\n"), 0o755)
	sum, _ := fileSHA256(filepath.Join(dir, "run.sh"))
	_ = m.update("flaky-ext", func(e *stateEntry) { e.ExecSHA256 = sum })
	_ = m.Discover(ctx)
	for i := 0; i < failureThreshold; i++ {
		if _, err := m.RunCommand(ctx, "flaky-ext", "hello", nil); err == nil {
			t.Fatal("expected failure")
		}
	}
	i, _ := m.Inspect("flaky-ext")
	if i.Status != StatusFailed || i.Health.ConsecutiveFailures < failureThreshold || i.Health.LastError == "" {
		t.Fatalf("breaker did not open: %+v", i)
	}
	// Host keeps working: other calls are refused fast, not executed.
	if _, err := m.RunCommand(ctx, "flaky-ext", "hello", nil); err == nil {
		t.Fatal("failed extension ran")
	}
}

func TestCapabilitiesConflictsAndDependencies(t *testing.T) {
	f := newFixture(t)
	f.add("prov-ext", "[[provides]]\nid = \"prov-ext.thing\"\nversion = 2\n", okScript("{}"))
	f.add("user-ext", "requires = [\"prov-ext.thing\", \"core.contacts\"]\n", okScript("{}"))
	f.add("lonely-ext", "requires = [\"missing.cap\"]\n", okScript("{}"))
	f.add("pro-ext", "entitlement = \"pro.sales\"\n[[provides]]\nid = \"pro.sales\"\nversion = 1\n", okScript("{}"))
	f.add("pro2-ext", "[[provides]]\nid = \"pro.sales\"\nversion = 1\n", okScript("{}"))
	m := f.manager(Options{})
	ctx := context.Background()
	caps := map[string]Capability{}
	for _, c := range m.Capabilities() {
		caps[c.ID] = c
	}
	if !caps["core.contacts"].Available || caps["core.contacts"].Provider != CoreProvider {
		t.Fatal("core capability missing")
	}
	if caps["prov-ext.thing"].Available {
		t.Fatal("disabled provider reported available")
	}
	// Dependent cannot run until the provider is active.
	if err := m.Enable(ctx, "user-ext", true); err == nil {
		t.Fatal("enabled with unmet dependency")
	}
	if err := m.Enable(ctx, "prov-ext", true); err != nil {
		t.Fatal(err)
	}
	if err := m.Enable(ctx, "user-ext", true); err != nil {
		t.Fatal(err)
	}
	if err := m.Enable(ctx, "lonely-ext", true); err == nil {
		t.Fatal("enabled with missing capability")
	}
	// pro.* is denied by the community entitlement provider.
	if err := m.Enable(ctx, "pro-ext", true); err == nil || !strings.Contains(err.Error(), "entitled") {
		t.Fatalf("pro extension enabled without entitlement: %v", err)
	}
	for _, c := range m.Capabilities() {
		if c.ID == "pro.sales" && c.Available {
			t.Fatal("pro capability available without entitlement")
		}
	}
	// Two extensions cannot both provide pro.sales.
	var conflicts int
	for _, i := range m.List() {
		if i.Status == StatusConflict {
			conflicts++
		}
	}
	if conflicts != 1 {
		t.Fatalf("conflicts=%d", conflicts)
	}
}

type allowAll struct{}

func (allowAll) Check(_ context.Context, c string) (EntitlementDecision, error) {
	return EntitlementDecision{Capability: c, Allowed: true, Source: "test"}, nil
}

func TestEntitlementProviderIsConsulted(t *testing.T) {
	f := newFixture(t)
	f.add("pro-ext", "entitlement = \"pro.sales\"\n[[provides]]\nid = \"pro.sales\"\nversion = 1\n", okScript("{}"))
	m := f.manager(Options{Entitlements: allowAll{}})
	if err := m.Enable(context.Background(), "pro-ext", true); err != nil {
		t.Fatal(err)
	}
	for _, c := range m.Capabilities() {
		if c.ID == "pro.sales" && !c.Available {
			t.Fatalf("entitled capability unavailable: %+v", c)
		}
	}
	d, _ := CommunityEntitlements{}.Check(context.Background(), "core.contacts")
	e, _ := CommunityEntitlements{}.Check(context.Background(), "pro.sales")
	if !d.Allowed || e.Allowed {
		t.Fatal("community entitlement decisions wrong")
	}
}

func TestConfigValidation(t *testing.T) {
	f := newFixture(t)
	f.add("cfg-ext", "[[config]]\nkey = \"days\"\nlabel = \"Days\"\ntype = \"integer\"\ndefault = 30\nmin = 1\nmax = 365\n[[config]]\nkey = \"mode\"\nlabel = \"Mode\"\ntype = \"enum\"\noptions = [\"a\", \"b\"]\ndefault = \"a\"\n", okScript("{}"))
	m := f.manager(Options{})
	for _, bad := range [][2]string{{"days", "0"}, {"days", "x"}, {"days", "1000"}, {"mode", "c"}, {"nope", "1"}} {
		if err := m.SetConfig("cfg-ext", bad[0], bad[1]); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
	if err := m.SetConfig("cfg-ext", "days", "45"); err != nil {
		t.Fatal(err)
	}
	i, _ := m.Inspect("cfg-ext")
	if i.Config[0].Value != int64(45) || i.Config[1].Value != "a" {
		t.Fatalf("%+v", i.Config)
	}
}

func TestStorageIsolationAndQuota(t *testing.T) {
	root := t.TempDir()
	a, _ := OpenStorage(root, "alpha-ext", 1)
	b, _ := OpenStorage(root, "beta-ext", 1)
	if err := a.Put("k", "secret"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := b.Get("k"); ok {
		t.Fatal("storage leaked across extensions")
	}
	for _, k := range []string{"../x", "a/b", "", "UP", strings.Repeat("a", 41)} {
		if err := a.Put(k, "v"); err == nil {
			t.Errorf("accepted key %q", k)
		}
	}
	if _, err := OpenStorage(root, "../evil", 1); err == nil {
		t.Fatal("traversal id accepted")
	}
	if err := a.Put("big", strings.Repeat("x", 2<<10)); !errors.Is(err, ErrQuota) {
		t.Fatalf("quota: %v", err)
	}
	if err := a.Purge(); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := a.Get("k"); ok {
		t.Fatal("purge failed")
	}
	if _, ok, _ := b.Get("k"); ok {
		t.Fatal("unexpected")
	}
}

func TestDisableKeepsDataAndPurgeNeedsDisable(t *testing.T) {
	f := newFixture(t)
	f.add("data-ext", "permissions = [\"storage.extension\"]\n[storage]\nrequired = true\n", okScript("{}"))
	m := f.manager(Options{})
	ctx := context.Background()
	_ = m.Enable(ctx, "data-ext", true)
	i, _ := m.Inspect("data-ext")
	s, _ := OpenStorage(f.data, "data-ext", 0)
	_ = s.Put("k", "v")
	if err := m.PurgeData(ctx, "data-ext"); err == nil {
		t.Fatal("purged data of an enabled extension")
	}
	_ = m.Disable(ctx, "data-ext")
	if v, ok, _ := s.Get("k"); !ok || v != "v" {
		t.Fatal("disable destroyed data")
	}
	if err := m.PurgeData(ctx, "data-ext"); err != nil {
		t.Fatal(err)
	}
	_ = i
}

func TestEventBusBackpressureAndShape(t *testing.T) {
	b := NewBus()
	s := b.Subscribe(2)
	for i := 0; i < 5; i++ {
		b.Publish(NewEvent("contact.created", "id", 1, "ui", ""))
	}
	if len(s.C) != 2 || s.Dropped() != 3 {
		t.Fatalf("queue=%d dropped=%d", len(s.C), s.Dropped())
	}
	e := <-s.C
	if e.ID == "" || e.SchemaVersion != EventSchemaVersion || e.EntityType != "contact" || e.Time == "" || e.Source != "ui" {
		t.Fatalf("envelope: %+v", e)
	}
	s.Close()
	b.Publish(e) // no subscribers, no panic
	(*Bus)(nil).Publish(e)
}

func TestEventDeliveryRules(t *testing.T) {
	f := newFixture(t)
	log := filepath.Join(f.root, "events.log")
	script := `while read line; do
case "$line" in *event.deliver*) printf '%s\n' "$line" >> ` + log + `;; esac
id=$(printf '%s' "$line" | sed 's/.*"request_id":"\([^"]*\)".*/\1/')
printf '{"api":1,"type":"response","request_id":"%s","ok":true,"data":{}}\n' "$id"
done
`
	_ = script
	// one-shot processes: append per invocation
	one := `read line
case "$line" in *event.deliver*) printf '%s\n' "$line" >> ` + log + `;; esac
id=$(printf '%s' "$line" | sed 's/.*"request_id":"\([^"]*\)".*/\1/')
printf '{"api":1,"type":"response","request_id":"%s","ok":true,"data":{}}\n' "$id"
`
	f.add("sub-ext", "permissions = [\"events.subscribe\", \"contacts.read\"]\n[events]\nsubscribe = [\"contact.created\"]\n", one)
	f.add("blind-ext", "permissions = [\"events.subscribe\"]\n", one)
	bus := NewBus()
	m := f.manager(Options{Bus: bus})
	ctx := context.Background()
	m.Start()
	if err := m.Enable(ctx, "sub-ext", true); err != nil {
		t.Fatal(err)
	}
	bus.Publish(NewEvent("contact.created", "c1", 1, "ui", ""))          // delivered
	bus.Publish(NewEvent("contact.updated", "c1", 2, "ui", ""))          // not subscribed
	bus.Publish(NewEvent("contact.created", "c2", 1, "extension:x", "")) // loop guard
	deadline := time.Now().Add(3 * time.Second)
	var data []byte
	for time.Now().Before(deadline) {
		data, _ = os.ReadFile(log)
		if strings.Contains(string(data), "c1") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
	data, _ = os.ReadFile(log)
	if strings.Count(string(data), "event.deliver") != 1 || !strings.Contains(string(data), `"entity_id":"c1"`) || strings.Contains(string(data), `"entity_id":"c2"`) {
		t.Fatalf("delivery log:\n%s", data)
	}
	if strings.Contains(strings.ToLower(string(data)), "name") {
		t.Fatal("event carries personal data")
	}
	m.Close()
}

func TestCloseReapsProcesses(t *testing.T) {
	f := newFixture(t)
	dir := f.add("slow-ext", basic, "read line\nsleep 20\n")
	m := f.manager(Options{Timeout: 10 * time.Second})
	_ = m.update("slow-ext", func(e *stateEntry) {})
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer close(done)
		_, _ = Invoke(ctx, dir, "run.sh", "slow-ext", "x", nil, 10*time.Second, nil)
	}()
	time.Sleep(200 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not stop the extension")
	}
}

func TestDroppedPermissionIsRevokedImmediately(t *testing.T) {
	f := newFixture(t)
	dir := f.add("shrink-ext", "permissions = [\"commands.register\", \"contacts.read\"]\n[[commands]]\nid = \"hello\"\ntitle = \"Hello\"\n", okScript(`{"lines":[]}`))
	m := f.manager(Options{})
	ctx := context.Background()
	if err := m.Enable(ctx, "shrink-ext", true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, ManifestFile))
	_ = os.WriteFile(filepath.Join(dir, ManifestFile), []byte(strings.Replace(string(b), `["commands.register", "contacts.read"]`, `["commands.register"]`, 1)), 0o644)
	_ = m.Discover(ctx)
	i, _ := m.Inspect("shrink-ext")
	if i.Status != StatusEnabled || len(i.NeedsApproval) != 0 {
		t.Fatalf("fewer permissions should not need approval: %+v", i)
	}
	m.mu.Lock()
	eff := m.recs["shrink-ext"].effective()
	m.mu.Unlock()
	if eff.Has("contacts.read") || !eff.Has("commands.register") {
		t.Fatalf("stale grant survived: %v", eff.Sorted())
	}
}

func TestDataRootNeverCollidesWithPrograms(t *testing.T) {
	// XDG config and data directories can be the same (tests, demo, some setups).
	root := t.TempDir()
	dir, data, _ := PathsFor(filepath.Join(root, "tidecontacts", "config.toml"), filepath.Join(root, "tidecontacts", "tidecontacts.db"))
	if dir == data || strings.HasPrefix(data, dir+string(filepath.Separator)) || strings.HasPrefix(dir, data+string(filepath.Separator)) {
		t.Fatalf("program dir %s and data dir %s overlap", dir, data)
	}
	// A directory holding a program is never purged, even if misconfigured.
	prog := filepath.Join(root, "prog")
	_ = os.MkdirAll(prog, 0o755)
	_ = os.WriteFile(filepath.Join(prog, ManifestFile), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(prog, "run.sh"), []byte("x"), 0o755)
	s, _ := OpenStorage(root, "prog", 0)
	_ = s.Put("k", "v")
	if err := s.Purge(); err == nil {
		t.Fatal("purged a program directory")
	}
	if _, err := os.Stat(filepath.Join(prog, "run.sh")); err != nil {
		t.Fatal("purge deleted a program file")
	}
}
