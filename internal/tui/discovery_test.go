package tui

import (
	"fmt"
	"github.com/allisonhere/tidecontacts/internal/config"
	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestDiscoveryUsesConfiguredKeys(t *testing.T) {
	c := config.Defaults()
	c.Keybindings["new-contact"] = []string{"N"}
	m := NewReady(c)
	if !strings.Contains(m.workspaceActions(), "Add person [N]") {
		t.Fatal(m.workspaceActions())
	}
	m.state.Active = wsRelationships
	if !strings.Contains(m.workspaceActions(), "Add relationship") {
		t.Fatal(m.workspaceActions())
	}
	if !strings.Contains(m.workspaceNavigation(), "[3 Relationships]") {
		t.Fatal(m.workspaceNavigation())
	}
}

func TestMenuNavigation(t *testing.T) {
	m, _ := crmApp(t, 160, 40)
	m = do(m, "ctrl+g")
	if !m.menuOpen {
		t.Fatal("menu did not open")
	}
	m = do(m, "down", "down", "enter")
	if m.state.Active != wsRelationships || m.menuOpen {
		t.Fatal("workspace navigation failed")
	}
	m = do(m, "ctrl+g", "esc")
	if m.menuOpen {
		t.Fatal("menu did not close")
	}
}

func TestThreeColumnsAt120(t *testing.T) {
	m, _ := crmApp(t, 120, 30)
	if m.mode() != tideui.ThreeColumn {
		t.Fatal("120 columns must show all three panes")
	}
	checkBounds(t, m.View(), 120, 30)
	m.width = 119
	if m.mode() == tideui.ThreeColumn {
		t.Fatal("119 columns should collapse")
	}
}

func TestMenuShortcutsAlignRight(t *testing.T) {
	m, _ := crmApp(t, 120, 30)
	rows := strings.Split(ansi.Strip(m.menuPane().Content), "\n")
	for i := 0; i < 8; i++ {
		if !strings.HasSuffix(rows[i], fmt.Sprint(i+1)) {
			t.Fatalf("shortcut not right aligned: %q", rows[i])
		}
		if ansi.StringWidth(rows[i]) != ansi.StringWidth(rows[0]) {
			t.Fatal("unequal row widths")
		}
	}
}

func TestThemePicker(t *testing.T) {
	m, _ := crmApp(t, 120, 30)
	m.cfg.TideMailThemeName = "oxocarbon-dark"
	next, cmd := m.dispatch("themes")
	m = deliver(next.(Model), cmd)
	if m.crm.picker == nil {
		t.Fatal("theme picker missing")
	}
	m.crm.picker.selected = 0
	m = do(m, "enter")
	if m.theme().Name != "oxocarbon-dark" || m.Preferences().Theme != "match-tidemail" {
		t.Fatal("match or persistence failed")
	}
}

func TestThemeLivePreviewCancelAndKeep(t *testing.T) {
	m, _ := crmApp(t, 120, 30)
	original := m.cfg.Theme
	next, cmd := m.dispatch("themes")
	m = deliver(next.(Model), cmd)
	m = do(m, "down")
	preview := m.cfg.Theme
	if preview == original {
		t.Fatal("no preview")
	}
	if m.Preferences().Theme != original {
		t.Fatal("unconfirmed preview persisted")
	}
	m = do(m, "esc")
	if m.cfg.Theme != original {
		t.Fatal("cancel did not restore")
	}
	next, cmd = m.dispatch("themes")
	m = deliver(next.(Model), cmd)
	m = do(m, "down", "enter")
	if m.cfg.Theme != preview || m.Preferences().Theme != preview {
		t.Fatal("choice not kept")
	}
	next, cmd = m.dispatch("themes")
	m = deliver(next.(Model), cmd)
	m = do(m, "catppuccin-latte")
	if m.cfg.Theme != "catppuccin-latte" {
		t.Fatal("search preview missing", m.cfg.Theme)
	}
	m = do(m, "esc")
	if m.cfg.Theme != preview {
		t.Fatal("search cancel failed")
	}
}

func TestNewFromFocusedMenuUsesHighlightedWorkspace(t *testing.T) {
	for _, width := range []int{100, 160} {
		for _, test := range []struct {
			workspace int
			title     string
		}{{wsActivities, "New activity"}, {wsTasks, "New task"}, {wsProjects, "New project"}, {wsPipelines, "New pipeline"}} {
			t.Run(fmt.Sprintf("%d/%s", width, test.title), func(t *testing.T) {
				m, _ := crmApp(t, width, 36)
				m = do(m, "ctrl+g")
				for i := 0; i < test.workspace; i++ {
					m = do(m, "down")
				}
				m = do(m, "n")
				if m.menuOpen || m.state.Active != test.workspace || m.crm.form == nil || m.crm.form.title != test.title {
					t.Fatalf("wrong creation target: ws=%d menu=%v form=%+v", m.state.Active, m.menuOpen, m.crm.form)
				}
			})
		}
	}
}
func TestNewFromMenuRespectsBindingAndOrganizationKind(t *testing.T) {
	m, _ := crmApp(t, 160, 36)
	cfg := config.Defaults()
	cfg.Keybindings["new-contact"] = []string{"N"}
	registry, err := cfg.Registry()
	if err != nil {
		t.Fatal(err)
	}
	m.registry = registry
	m = do(m, "ctrl+g", "down", "n")
	if !m.menuOpen || m.contacts.editor != nil {
		t.Fatal("unbound lowercase n created a record")
	}
	m = do(m, "N")
	if m.menuOpen || m.state.Active != wsOrganizations || m.contacts.editor == nil || m.contacts.editor.draft.Kind != "organization" {
		t.Fatal("custom New did not create an organization")
	}
}

func TestPaneNavigationRoundTrip(t *testing.T) {
	for _, width := range []int{60, 80, 110, 160} {
		m, _ := crmApp(t, width, 32)
		for _, workspace := range []string{"1", "5", "7"} {
			m = do(m, workspace)
			for focus := range m.focusCount() {
				m.focus = focus
				m = do(m, "tab")
				if m.menuOpen || m.focus != (focus+1)%m.focusCount() {
					t.Fatalf("width %d workspace %s: Tab from %d reached %d", width, workspace, focus, m.focus)
				}
				m = do(m, "shift+tab", "ctrl+g", "esc")
				if m.menuOpen || m.focus != focus {
					t.Fatalf("width %d workspace %s: lost focus %d", width, workspace, focus)
				}
			}
		}
	}
}

func TestHorizontalMenuNavigation(t *testing.T) {
	for _, width := range []int{60, 110, 160} {
		m, _ := crmApp(t, width, 32)
		m = do(m, "1", "right", "left", "left")
		if !m.menuOpen || m.menuSelected != wsPeople {
			t.Fatalf("%d: Left from list should focus People in menu", width)
		}
		m = do(m, "down", "right")
		if m.menuOpen || m.state.Active != wsOrganizations || m.focus != 0 {
			t.Fatalf("%d: Right should enter Organizations list", width)
		}
		m = do(m, "right")
		if m.focus != 1 {
			t.Fatalf("%d: Right should focus details", width)
		}
	}
}

func TestMenuWorkspacePreviewAndShortcuts(t *testing.T) {
	for _, enter := range []string{"tab", "right", "l", "enter"} {
		m, _ := crmApp(t, 160, 40)
		m = do(m, "ctrl+g", "down")
		if !m.menuOpen || m.state.Active != wsOrganizations {
			t.Fatal("highlight did not show Organizations")
		}
		m = do(m, enter)
		if m.menuOpen || m.focus != 0 || m.state.Active != wsOrganizations {
			t.Fatal(enter)
		}
	}
	m, _ := crmApp(t, 160, 40)
	m = do(m, "ctrl+g", "8")
	if m.menuOpen || m.state.Active != wsInsights {
		t.Fatal("8 did not open Insights")
	}
	for _, k := range []string{"?", ":", "b", "I"} {
		m = do(m, "ctrl+g", k)
		if m.menuOpen || k == "?" && m.overlay != help || k == ":" && m.overlay != palette || k == "b" && m.crm.prompt == nil || k == "I" && m.contacts.transfer == nil {
			t.Fatalf("menu swallowed %q", k)
		}
		m = do(m, "esc")
	}
}
