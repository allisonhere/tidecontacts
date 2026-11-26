package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

func settingsKeys(m Model, keys ...string) Model {
	for _, k := range keys {
		m, _ = key(m, k)
	}
	return m
}

func TestSettingsDisplayAndStartupKeyboard(t *testing.T) {
	m := sized(110, 32)
	m = settingsKeys(m, ":", "open settings", "enter")
	view := frame(m)
	if !strings.Contains(view, "Categories") || !strings.Contains(view, "Contact list density") {
		t.Fatalf("palette did not open the split Settings page\n%s", view)
	}
	m = settingsKeys(m, "enter", "enter", "down")
	if m.crm.picker == nil || m.cfg.Theme == "dark" || m.Preferences().Theme != "dark" {
		t.Fatal("Settings did not use the existing non-persistent live theme preview")
	}
	m = settingsKeys(m, "esc", "down", "enter", "down", "enter")
	if view := frame(m); !strings.Contains(view, "Comfortable") || m.cfg.Theme != "dark" || !m.cfg.Mouse {
		t.Fatalf("display controls did not change or theme preview was not rolled back\n%s", view)
	}
	m = settingsKeys(m, "enter", "esc", "down", "enter", "enter", "down", "down", "down", "down", "enter")
	if !strings.Contains(frame(m), "Startup workspace") || m.state.Active != wsPeople {
		t.Fatal("choosing a startup workspace closed Settings or changed the current workspace")
	}
	var saved Preferences
	data, err := json.Marshal(m.Preferences())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.DefaultWorkspace != "tasks" || saved.Density != "comfortable" || saved.Mouse == nil || *saved.Mouse {
		t.Fatalf("saved preferences lost the startup workspace or explicitly disabled mouse: %s", data)
	}
}

func TestSettingsFromMenuResizesWithoutLosingFocus(t *testing.T) {
	m := sized(160, 45)
	m, _ = press(m, tea.KeyCtrlG)
	for step := 0; step < len(m.menuCommands()) && m.menuCommands()[m.menuSelected].ID != "settings"; step++ {
		m, _ = key(m, "down")
	}
	if m.menuCommands()[m.menuSelected].ID != "settings" {
		t.Fatal("Settings is missing from the main menu")
	}
	m = settingsKeys(m, "enter", "down", "down", "enter")
	for _, size := range [][2]int{{160, 45}, {80, 24}, {44, 11}, {8, 3}, {1, 2}, {1, 1}} {
		model, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = model.(Model)
		checkBounds(t, m.View(), size[0], size[1])
		view := frame(m)
		if size[0] == 44 && (!strings.Contains(view, "Address books & import") || strings.Contains(view, "Categories")) {
			t.Fatalf("compact Settings did not preserve detail focus\n%s", view)
		}
	}
	model, _ := m.Update(tea.WindowSizeMsg{Width: 44, Height: 11})
	m = settingsKeys(model.(Model), "esc")
	if view := frame(m); !strings.Contains(view, "Categories") || strings.Contains(view, "Address books & import") {
		t.Fatalf("Esc did not restore the compact category pane\n%s", view)
	}
	m = settingsKeys(m, "esc")
	if m.settings.open || m.state.Active != wsInsights {
		t.Fatalf("leaving Settings did not return to the workspace behind the menu: %+v", m.state)
	}
}

func TestSettingsOpensExistingMCPManagerAndReturns(t *testing.T) {
	m, store := crmApp(t, 110, 32)
	m.mcp.state = store
	m = settingsKeys(m, ":", "open settings", "enter", "down", "down", "down", "down", "enter")
	m, cmd := key(m, "enter")
	m = mcpTestDeliver(t, m, cmd)
	if view := frame(m); !strings.Contains(view, "MCP profiles") || !strings.Contains(view, "TideContacts / Settings") {
		t.Fatalf("MCP management did not open over Settings\n%s", view)
	}
	m = settingsKeys(m, "esc")
	if view := frame(m); !strings.Contains(view, "Manage MCP profiles") || !strings.Contains(view, "Agents (MCP)") {
		t.Fatalf("closing the manager did not return to the selected category\n%s", view)
	}
}

func TestSettingsDoesNotSaveUntouchedTOMLDefaults(t *testing.T) {
	cfg := config.Defaults()
	cfg.Density, cfg.DefaultWorkspace, cfg.Mouse = "comfortable", "tasks", true
	m := settingsKeys(NewReady(cfg), ":", "open settings", "enter", "esc")
	if p := m.Preferences(); p.Density != "" || p.DefaultWorkspace != "" || p.Mouse != nil {
		t.Fatalf("opening Settings froze untouched config defaults: %+v", p)
	}
}
