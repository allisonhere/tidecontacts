package tui

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tidecontacts/internal/ext"
	"github.com/charmbracelet/x/ansi"
)

const extOK = `read line
id=$(printf '%s' "$line" | sed 's/.*"request_id":"\([^"]*\)".*/\1/')
printf '{"api":1,"type":"response","request_id":"%s","ok":true,"data":%s}\n' "$id" '{"title":"Hello result","lines":[{"label":"x","text":"fine","level":"ok"}]}'
`

func installExt(t *testing.T, root, id, extra, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script extensions are Unix only")
	}
	dir := filepath.Join(root, "extensions", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "id = \"" + id + "\"\nname = \"Demo " + id + "\"\nversion = \"1.0.0\"\napi = 1\ncommand = \"run.sh\"\npermissions = [\"commands.register\"]\n[[commands]]\nid = \"hello\"\ntitle = \"Say hello\"\n" + extra
	if err := os.WriteFile(filepath.Join(dir, ext.ManifestFile), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func extApp(t *testing.T, w, h int) (Model, *ext.Manager, string) {
	t.Helper()
	m, _ := crmApp(t, w, h)
	root := t.TempDir()
	mgr := ext.New(ext.Options{Dir: filepath.Join(root, "extensions"), DataDir: filepath.Join(root, "data"), StateFile: filepath.Join(root, "state.json"), Timeout: 3 * time.Second})
	t.Cleanup(mgr.Close)
	_ = mgr.Discover(context.Background())
	m.extensions.mgr = mgr
	return m, mgr, root
}

func rediscover(m Model, mgr *ext.Manager) Model {
	_ = mgr.Discover(context.Background())
	return m
}

func TestPaletteIsUnchangedWithoutExtensions(t *testing.T) {
	m, _ := crmApp(t, 120, 30)
	if got, want := len(m.paletteMatches()), len(m.registry.Filter("")); got != want {
		t.Fatalf("palette changed with no extension manager: %d vs %d", got, want)
	}
	m, _, _ = extApp(t, 120, 30) // manager present but nothing installed
	if got, want := len(m.paletteMatches()), len(m.registry.Filter("")); got != want {
		t.Fatalf("palette changed with no extensions installed: %d vs %d", got, want)
	}
	// With no manager at all the entry reports unavailability instead of crashing.
	n, _ := crmApp(t, 120, 30)
	n = do(n, "ctrl+k")
	n2, _ := n.dispatch("extensions-manage")
	if n2.(Model).extensions.modal != nil {
		t.Fatal("modal opened without a manager")
	}
}

func TestExtensionManagerModal(t *testing.T) {
	m, mgr, root := extApp(t, 120, 30)
	installExt(t, root, "hello-ext", "", extOK)
	installExt(t, root, "plain-ext", "", extOK)
	before := struct{ ws, focus, sel int }{m.state.Active, m.focus, m.selected}

	next, cmd := m.dispatch("extensions-manage")
	m = deliver(next.(Model), cmd)
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "extensions") || !strings.Contains(out, "Demo hello-ext") || !strings.Contains(out, "disabled") {
		t.Fatalf("list not rendered:\n%s", out)
	}
	checkBounds(t, m.View(), 120, 30)

	m = do(m, "down", "up", "enter")
	out = ansi.Strip(m.View())
	if m.extensions.modal.mode != "detail" || !strings.Contains(out, "commands.register") || !strings.Contains(out, "requested") {
		t.Fatalf("detail missing permissions:\n%s", out)
	}
	m = do(m, "esc")
	if m.extensions.modal == nil || m.extensions.modal.mode != "list" {
		t.Fatal("esc in detail should return to the list")
	}

	// Enabling shows the approval screen first; cancelling changes nothing.
	m = do(m, "e")
	out = ansi.Strip(m.View())
	if !strings.Contains(out, "asks for these permissions") || !strings.Contains(out, "commands.register") || !strings.Contains(out, "do not sandbox") {
		t.Fatalf("approval screen:\n%s", out)
	}
	m = do(m, "esc")
	if i, _ := mgr.Inspect("hello-ext"); i.Status != ext.StatusDisabled {
		t.Fatal("extension enabled without approval")
	}
	m = do(m, "e", "y")
	if i, _ := mgr.Inspect("hello-ext"); i.Status != ext.StatusEnabled {
		t.Fatalf("approved extension not enabled: %+v", i)
	}
	if !strings.Contains(ansi.Strip(m.View()), "enabled") {
		t.Fatal("list does not show the new status")
	}

	// The palette now offers the extension's command, and running it opens a host-rendered result.
	m = do(m, "esc")
	if m.extensions.modal != nil {
		t.Fatal("esc did not close the modal")
	}
	if after := (struct{ ws, focus, sel int }{m.state.Active, m.focus, m.selected}); after != before {
		t.Fatalf("navigation state not preserved: %v vs %v", after, before)
	}
	found := ""
	m.prompt.SetValue("hello")
	for _, c := range m.paletteMatches() {
		if strings.HasPrefix(c.ID, "ext:") {
			found = c.ID
			if !strings.Contains(c.Label, "Say hello") || len(c.Keys) != 0 {
				t.Fatalf("extension command label/keys: %+v", c)
			}
		}
	}
	if found != "ext:hello-ext:hello" {
		t.Fatalf("command not in palette: %q", found)
	}
	next, cmd = m.dispatch(found)
	m = deliver(next.(Model), cmd)
	out = ansi.Strip(m.View())
	if !strings.Contains(out, "hello result") || !strings.Contains(out, "fine") {
		t.Fatalf("result not rendered:\n%s", out)
	}
	m = do(m, "esc")
	if m.extensions.modal != nil || m.state.Active != before.ws {
		t.Fatal("focus not restored after the result closed")
	}

	// Disabling removes the contribution.
	next, cmd = m.dispatch("extensions-manage")
	m = deliver(next.(Model), cmd)
	m = do(m, "e")
	if len(mgr.Commands()) != 0 {
		t.Fatal("disabled extension still contributes commands")
	}
}

func TestExtensionFailureStatesDoNotBreakTheUI(t *testing.T) {
	m, mgr, root := extApp(t, 100, 28)
	installExt(t, root, "crash-ext", "", "read line\nexit 3\n")
	installExt(t, root, "bad-ext", "", extOK)
	_ = os.WriteFile(filepath.Join(root, "extensions", "bad-ext", ext.ManifestFile), []byte("id = \"bad-ext\"\nname = \"x\x1b[31m\"\n"), 0o644)
	m = rediscover(m, mgr)
	next, cmd := m.dispatch("extensions-manage")
	m = deliver(next.(Model), cmd)
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "invalid") || !strings.Contains(out, "disabled") {
		t.Fatalf("states:\n%s", out)
	}
	// Selecting the invalid extension: enabling is refused with a readable reason.
	m = do(m, "e")
	if m.extensions.modal.mode != "list" || m.extensions.modal.err == "" {
		// order is bad-ext, crash-ext: first row is bad-ext (invalid)
		t.Fatalf("invalid extension offered approval: %+v", m.extensions.modal)
	}
	checkBounds(t, m.View(), 100, 28)
	// A crashing extension fails activation; the host stays usable and it stays disabled.
	m = do(m, "down", "e", "y")
	if i, _ := mgr.Inspect("crash-ext"); i.Status == ext.StatusEnabled {
		t.Fatal("crashing extension enabled")
	}
	if m.extensions.modal.err == "" || !strings.Contains(ansi.Strip(m.View()), "activation failed") {
		t.Fatalf("failure not reported: %q", m.extensions.modal.err)
	}
	m = do(m, "esc")
	if m.extensions.modal != nil {
		t.Fatal("modal stuck after a failure")
	}
	m = do(m, "j") // normal navigation still works
}

func TestExtensionModalNarrowTerminals(t *testing.T) {
	for _, size := range [][2]int{{60, 20}, {40, 14}, {30, 10}} {
		m, mgr, root := extApp(t, size[0], size[1])
		installExt(t, root, "narrow-ext-with-a-very-long-identifier", "description = \"A long description that must wrap or clip but never overflow the terminal width at all\"\n", extOK)
		m = rediscover(m, mgr)
		next, cmd := m.dispatch("extensions-manage")
		m = deliver(next.(Model), cmd)
		checkBounds(t, m.View(), size[0], size[1])
		m = do(m, "enter")
		checkBounds(t, m.View(), size[0], size[1])
		m = do(m, "esc", "e")
		checkBounds(t, m.View(), size[0], size[1])
	}
}

func TestExtensionDossierPanelRows(t *testing.T) {
	m, mgr, root := extApp(t, 140, 40)
	panel := "[[panels]]\nid = \"p\"\ntitle = \"Digest\"\nslot = \"contact.dossier\"\n"
	script := "read line\nid=$(printf '%s' \"$line\" | sed 's/.*\"request_id\":\"\\([^\"]*\\)\".*/\\1/')\nprintf '{\"api\":1,\"type\":\"response\",\"request_id\":\"%s\",\"ok\":true,\"data\":%s}\\n' \"$id\" '{\"lines\":[{\"label\":\"Seen\",\"text\":\"3 times\"}]}'\n"
	installExt(t, root, "panel-ext", strings.Replace(panel, "[[panels]]", "[[panels]]", 1), script)
	b, _ := os.ReadFile(filepath.Join(root, "extensions", "panel-ext", ext.ManifestFile))
	_ = os.WriteFile(filepath.Join(root, "extensions", "panel-ext", ext.ManifestFile), []byte(strings.Replace(string(b), `permissions = ["commands.register"]`, `permissions = ["commands.register", "ui.register"]`, 1)), 0o644)
	_ = mgr.Discover(context.Background())
	if err := mgr.Enable(context.Background(), "panel-ext", true); err != nil {
		t.Fatal(err)
	}
	m.contacts.contact.ID = "contact-1"
	m.crm.dossierSeq = 4
	cmd := m.loadExtPanels(4, "contact-1")
	if cmd == nil {
		t.Fatal("no panel load for an enabled extension")
	}
	msg := cmd()
	n, _, handled := m.updateExtensions(msg)
	if !handled || len(n.extPanelRows("contact-1")) != 2 || n.extPanelRows("someone-else") != nil {
		t.Fatalf("rows: %+v", n.extPanelRows("contact-1"))
	}
	// A stale answer is ignored.
	stale := msg.(extPanelMsg)
	stale.seq = 1
	n2, _, _ := m.updateExtensions(stale)
	if n2.extPanelRows("contact-1") != nil {
		t.Fatal("stale panel result applied")
	}
}
