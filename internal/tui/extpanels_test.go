package tui

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/ext"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// panelApp opens a workspace whose extension answers every render with
// testdata/panels.json.
func panelApp(t *testing.T, w, h int) Model {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script extensions are Unix only")
	}
	doc, err := os.ReadFile("testdata/panels.json")
	if err != nil {
		t.Fatal(err)
	}
	m, mgr, root := extApp(t, w, h)
	dir := filepath.Join(root, "extensions", "bi-ext")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, ext.ManifestFile), []byte("id = \"bi-ext\"\nname = \"Business\"\nversion = \"1.0.0\"\napi = 1\ncommand = \"run.sh\"\npermissions = [\"ui.register\"]\n[[workspaces]]\nid = \"business\"\ntitle = \"Business\"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "doc.json"), doc, 0o644)
	script := `#!/bin/sh
read line
id=$(printf '%s' "$line" | sed 's/.*"request_id":"\([^"]*\)".*/\1/')
case "$line" in
*'"item":'*) data='{"title":"Dossier","sections":[{"title":"Record","lines":[{"label":"Value","text":"$1"}]}]}';;
*'"method":"view.render"'*) data=$(cat "$(dirname "$0")/doc.json");;
*'"method":"action.run"'*) data='{"notice":"ran","refresh":true}';;
*) data='{}';;
esac
printf '{"api":1,"type":"response","request_id":"%s","ok":true,"data":%s}\n' "$id" "$data"
`
	_ = os.WriteFile(filepath.Join(dir, "run.sh"), []byte(script), 0o755)
	_ = mgr.Discover(ctxBackground())
	if err := mgr.Enable(ctxBackground(), "bi-ext", true); err != nil {
		t.Fatal(err)
	}
	next, cmd := m.dispatch("ext-ws:bi-ext:business")
	return deliver(next.(Model), cmd)
}

func shot(t *testing.T, m Model) string {
	t.Helper()
	out := ansi.Strip(m.View())
	if dir := os.Getenv("TIDECONTACTS_FRAMES"); dir != "" {
		_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%dx%d.txt", strings.ReplaceAll(t.Name(), "/", "_"), m.width, m.height)), []byte(out), 0o644)
	}
	return out
}

func has(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}
}

func TestPanelsWideLayout(t *testing.T) {
	m := panelApp(t, 160, 45)
	out := shot(t, m)
	has(t, out, "Business Pulse", "PRO", "$86,400", "54.6% weighted", "REVENUE PERFORMANCE", "SALES PIPELINE", "REQUIRES YOUR ATTENTION", "CUSTOMER CONCENTRATION", "Jun", "$28,400", "█", "Proposal awaiting response", "$18,000", "74%", "no data", "[ ] view", "panel", "keys", "│")
	checkBounds(t, m.View(), 160, 45)
	if v := m.extensions.view; v.cur.panel != "revenue" || len(v.items()) != 6 {
		t.Fatalf("focus %q items %d", v.cur.panel, len(v.items()))
	}
}

func TestPanelsStandardAndCompactLayouts(t *testing.T) {
	m := panelApp(t, 110, 32)
	has(t, shot(t, m), "REVENUE PERFORMANCE", "SALES PIPELINE", "REQUIRES YOUR ATTENTION", "$86,400")
	checkBounds(t, m.View(), 110, 32)

	c := panelApp(t, 80, 24)
	out := shot(t, c)
	has(t, out, "[command]", "$86,400", "[revenue]", "pipeline", "REVENUE PERFORMANCE", "Jun", "$8,200", "close")
	if strings.Contains(out, "REQUIRES YOUR ATTENTION") {
		t.Errorf("compact shows one panel at a time:\n%s", out)
	}
	checkBounds(t, c.View(), 80, 24)
	c = do(c, "tab", "tab")
	has(t, shot(t, c), "[attention]", "REQUIRES YOUR ATTENTION", "Vertex Labs", "$6,400")
}

func TestPanelsGoldenFrames(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {110, 32}, {160, 45}} {
		w, h := size[0], size[1]
		m := panelApp(t, w, h)
		frame := strings.Split(ansi.Strip(m.View()), "\n")
		for i, line := range frame {
			frame[i] = strings.TrimRight(line, " ")
		}
		out := strings.Join(frame, "\n") + "\n"
		name := filepath.Join("testdata", fmt.Sprintf("panels-%dx%d.txt", w, h))
		if os.Getenv("UPDATE_GOLDEN") == "1" {
			if err := os.WriteFile(name, []byte(out), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if out != string(want) {
			t.Fatalf("%s changed:\n%s", name, out)
		}
	}
}

func TestPanelsReducedGlyphsRetainValues(t *testing.T) {
	m := panelApp(t, 80, 24)
	m.cfg.Theme = "vt52"
	frame := ansi.Strip(m.View())
	has(t, frame, "###", "$8,200", "$28,400")
	if strings.ContainsRune(frame, '█') {
		t.Fatal("reduced-glyph chart drew Unicode blocks")
	}
}

func TestPanelsFocusSelectionDrillDownAndReturn(t *testing.T) {
	m := panelApp(t, 160, 45)
	m = do(m, "tab", "tab", "j", "j")
	v := m.extensions.view
	if sel, _ := v.selected(); v.cur.panel != "attention" || sel.id != "sig3" || sel.target.ID != "opp3" {
		t.Fatalf("%q %+v", v.cur.panel, sel)
	}
	has(t, shot(t, m), "›! Vertex Labs")
	// Focus moves away and back: the selection is remembered.
	m = do(m, "tab", "shift+tab")
	if v := m.extensions.view; v.cur.panel != "attention" || v.cur.row != 2 {
		t.Fatalf("%q row %d", v.cur.panel, v.cur.row)
	}
	// Item actions receive the line's own id; a refresh keeps focus and row.
	m = do(m, "z")
	if v := m.extensions.view; v.cur.panel != "attention" || v.cur.row != 2 || v.notice != "ran" {
		t.Fatalf("%q row %d %q", v.cur.panel, v.cur.row, v.notice)
	}
	m = do(m, "enter")
	has(t, shot(t, m), "Dossier")
	m = do(m, "esc")
	v = m.extensions.view
	if sel, _ := v.selected(); v.cur.panel != "attention" || sel.id != "sig3" {
		t.Fatalf("return lost the place: %q %+v", v.cur.panel, sel)
	}
	// h/l move between panels too; ? lists keys and any key closes it.
	m = do(m, "h")
	if m.extensions.view.cur.panel != "pipe" {
		t.Fatal(m.extensions.view.cur.panel)
	}
	m = do(m, "?")
	has(t, shot(t, m), "move between panels", "Snooze (selected item)")
	m = do(m, "j")
	if m.extensions.view.help || m.extensions.view.cur.panel != "pipe" {
		t.Fatal("help did not close cleanly")
	}
}

// A dashboard never exceeds the terminal, whatever its size, focus or keys.
func TestPanelsStayBoundedAtAnySize(t *testing.T) {
	m := panelApp(t, 120, 36)
	r := rand.New(rand.NewSource(7))
	keys := []string{"tab", "shift+tab", "j", "k", "end", "?", "pgdown", "l"}
	for i := 0; i < 400; i++ {
		w, h := 20+r.Intn(230), 6+r.Intn(70)
		m = resize(m, w, h)
		for k := r.Intn(4); k > 0; k-- {
			m = do(m, keys[r.Intn(4)])
		}
		if i%9 == 0 {
			m.extensions.view.help = r.Intn(2) == 0
		}
		checkBounds(t, m.View(), w, h)
		if t.Failed() {
			t.Fatalf("at %dx%d focus %s", w, h, m.extensions.view.cur.panel)
		}
		m.extensions.view.help = false
	}
	// Every panel can be reached and drawn on a short terminal.
	m = resize(m, 100, 20)
	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		m = do(m, "tab")
		p := m.extensions.view.doc.Panels[m.extensions.view.focusedPanel()]
		seen[p.ID] = true
		has(t, ansi.Strip(m.View()), strings.ToUpper(p.Title))
	}
	if len(seen) != 5 {
		t.Fatalf("unreachable panels: %v", seen)
	}
}

func resize(m Model, w, h int) Model {
	n, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return n.(Model)
}
