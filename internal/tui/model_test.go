package tui

import (
	"fmt"
	"github.com/allisonhere/tidecontacts/internal/config"
	"github.com/allisonhere/tideui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func key(m Model, k string) (Model, tea.Cmd) {
	var msg tea.KeyMsg
	special := map[string]tea.KeyType{"enter": tea.KeyEnter, "esc": tea.KeyEsc, "tab": tea.KeyTab, "shift+tab": tea.KeyShiftTab, "down": tea.KeyDown, "up": tea.KeyUp, "ctrl+c": tea.KeyCtrlC}
	if typ, ok := special[k]; ok {
		msg.Type = typ
	} else {
		msg.Type = tea.KeyRunes
		msg.Runes = []rune(k)
	}
	n, c := m.Update(msg)
	return n.(Model), c
}
func sized(w, h int) Model {
	m := NewReady(config.Defaults())
	n, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return n.(Model)
}
func TestNavigationAndFocus(t *testing.T) {
	m := sized(160, 45)
	m, _ = key(m, "j")
	if m.selected != 1 {
		t.Fatal(m.selected)
	}
	m, _ = key(m, "enter")
	if m.state.Active != 1 || m.focus != 1 {
		t.Fatal("activation")
	}
	m, _ = key(m, "tab")
	if m.focus != 2 {
		t.Fatal("next")
	}
	n, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 32})
	m = n.(Model)
	if m.focus != 1 {
		t.Fatal("hidden focus")
	}
	m, _ = key(m, "tab")
	if m.focus != 0 {
		t.Fatal("medium cycle")
	}
	for i := 1; i <= 8; i++ {
		m, _ = key(m, fmt.Sprint(i))
		if m.state.Active != i-1 || m.focus != 1 {
			t.Fatal(i)
		}
	}
	m, _ = key(m, "esc")
	if m.focus != 0 {
		t.Fatal("back")
	}
}
func TestInputPrecedenceAndPalette(t *testing.T) {
	m := sized(80, 24)
	m, _ = key(m, "/")
	for _, s := range []string{"q", "1", "j", "?", ":"} {
		var c tea.Cmd
		m, c = key(m, s)
		_ = c
	}
	if m.state.Active != 0 || !m.editing || m.search.Value() != "q1j?:" || m.overlay != none {
		t.Fatal("input leaked")
	}
	m, _ = key(m, "enter")
	m, _ = key(m, "2")
	m, _ = key(m, "1")
	if m.search.Value() != "q1j?:" {
		t.Fatal("query not restored")
	}
	m, _ = key(m, ":")
	m, _ = key(m, "gt tsk")
	m, _ = key(m, "enter")
	if m.state.Active != 4 || m.overlay != none {
		t.Fatal("palette execution")
	}
	m, _ = key(m, ":")
	m, _ = key(m, "light")
	m, _ = key(m, "enter")
	if m.cfg.Theme != "light" {
		t.Fatal("theme")
	}
	m, _ = key(m, "/")
	_, cmd := key(m, "ctrl+c")
	if cmd == nil {
		t.Fatal("safe exit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("not quit")
	}
}
func TestBoundedRendering(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 100; i++ {
		w, h := 1+rng.Intn(180), 1+rng.Intn(50)
		for _, o := range []overlay{none, palette, help} {
			m := sized(w, h)
			m.overlay = o
			m.prompt.SetValue("界é👩‍💻" + strings.Repeat("x", 200))
			out := m.View()
			if len(strings.Split(out, "\n")) > h {
				t.Fatalf("height %dx%d", w, h)
			}
			for _, line := range strings.Split(out, "\n") {
				if lipgloss.Width(line) > w {
					t.Fatalf("width %dx%d: %d", w, h, lipgloss.Width(line))
				}
			}
		}
	}
}
func TestRepresentativeFrames(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(old)
	for _, size := range [][2]int{{160, 45}, {110, 32}, {80, 24}, {60, 20}} {
		m := sized(size[0], size[1])
		frame := ansi.Strip(m.View())
		name := fmt.Sprintf("%dx%d.txt", size[0], size[1])
		path := filepath.Join("testdata", name)
		if os.Getenv("UPDATE_GOLDEN") == "1" {
			if e := os.MkdirAll("testdata", 0755); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(path, []byte(frame), 0644); e != nil {
				t.Fatal(e)
			}
		}
		expected, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if string(expected) != frame {
			t.Fatalf("snapshot mismatch %s", name)
		}
	}
}
func TestThemeAndFocusVisible(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	m := sized(160, 45)
	a := m.View()
	m.cfg.Theme = "light"
	if a == m.View() {
		t.Fatal("theme does not affect frame")
	}
	for _, name := range []string{"dark", "light", "contrast"} {
		theme := resolveTheme(name)
		if tideui.ContrastRatio(theme.Fg, theme.Bg) < 4.5 {
			t.Fatal(name)
		}
	}
	m.focus = 1
	if !strings.Contains(ansi.Strip(m.View()), "> People") {
		t.Fatal("missing noncolor focus")
	}
}

func TestPaneRatiosAndStartup(t *testing.T) {
	m := sized(160, 45)
	for i := 0; i < 100; i++ {
		next, _ := m.dispatch("left-grow")
		m = next.(Model)
		next, _ = m.dispatch("right-shrink")
		m = next.(Model)
	}
	p := m.Preferences()
	if p.Left != .35 || p.Right != .15 {
		t.Fatal(p)
	}
	c := config.Defaults()
	c.Layout.Left, c.Layout.Right = p.Left, p.Right
	if NewReady(c).Preferences() != p {
		t.Fatal("saved layout did not round trip")
	}
	for _, s := range [][2]int{{0, 0}, {-1, -2}} {
		n, _ := m.Update(tea.WindowSizeMsg{Width: s[0], Height: s[1]})
		if n.View() != "" {
			t.Fatal("nonempty zero-size frame")
		}
	}
	loading := New(nil)
	if loading.IsReady() {
		t.Fatal("loading marked ready")
	}
	_, cmd := key(loading, "ctrl+c")
	if cmd == nil {
		t.Fatal("cannot cancel loading")
	}
	n, cmd := loading.Update(Ready{Err: fmt.Errorf("database unavailable")})
	if n.(Model).Err == nil || cmd == nil {
		t.Fatal("startup error did not terminate")
	}
}
