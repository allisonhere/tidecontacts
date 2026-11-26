package tui

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/config"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
	tea "github.com/charmbracelet/bubbletea"
)

func mcpTestApp(t *testing.T, demo bool) (Model, *sqlite.Store) {
	t.Helper()
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "mcp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg := config.Defaults()
	cfg.Demo = demo
	next, _ := sized(110, 32).Update(Ready{Config: cfg, Repo: store, MCP: store, Context: ctx})
	m := next.(Model)
	m, _ = press(m, tea.KeyCtrlK)
	m, _ = mcpTestKeys(m, "mcp profiles")
	m, cmd := press(m, tea.KeyEnter)
	return mcpTestDeliver(t, m, cmd), store
}

func mcpTestKeys(m Model, text string) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
	return next.(Model), cmd
}

func mcpTestDeliver(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected MCP IO command")
	}
	for cmd != nil {
		msg := cmd()
		switch msg.(type) {
		case mcpLoaded, mcpSaved, mcpInspected:
		default:
			t.Fatalf("unexpected MCP IO result %T", msg)
		}
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(Model)
	}
	return m
}

func TestMCPKeyboardRequiresConfirmationAndDefaultsDisabled(t *testing.T) {
	m, store := mcpTestApp(t, false)
	ctx := context.Background()
	if !strings.Contains(m.View(), "MCP is optional") {
		t.Fatal("palette did not open MCP profiles")
	}
	m, _ = mcpTestKeys(m, "n")
	m, _ = mcpTestKeys(m, "local-client")
	m, cmd := press(m, tea.KeyEnter)
	if cmd != nil || m.mcp.modal.mode != mcpConfirm {
		t.Fatal("Enter should open review without starting a save")
	}
	if lines := strings.Join(m.mcp.modal.reviewLines(), "\n"); !strings.Contains(lines, "disabled") || !strings.Contains(lines, "No CRM scopes") {
		t.Fatalf("unexpected default grant review: %s", lines)
	}
	m, cmd = press(m, tea.KeyEnter)
	if cmd != nil {
		t.Fatal("Enter on review must not confirm the grant")
	}
	m, _ = press(m, tea.KeyEsc)
	if m.mcp.modal.mode != mcpEdit {
		t.Fatal("Esc from review should return to the draft")
	}
	if _, err := store.MCPProfile(ctx, "local-client"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unconfirmed profile changed storage: %v", err)
	}
	m, _ = press(m, tea.KeyEnter)
	m, cmd = mcpTestKeys(m, "y")
	m = mcpTestDeliver(t, m, cmd)
	p, err := store.MCPProfile(ctx, "local-client")
	if err != nil || p.Enabled || len(p.Scopes) != 0 || p.Revision != 1 {
		t.Fatalf("saved profile = %+v, %v", p, err)
	}
	m, cmd = mcpTestKeys(m, "d")
	if cmd != nil || !strings.Contains(m.View(), "cannot start an MCP session") {
		t.Fatal("disabled diagnostics should explain status without starting discovery")
	}
}

func TestMCPKeyboardWriteDiscoveryAuditAndStaleRevision(t *testing.T) {
	m, store := mcpTestApp(t, true)
	ctx := context.Background()
	m, _ = mcpTestKeys(m, "n")
	m, _ = mcpTestKeys(m, "demo-client")
	m, _ = press(m, tea.KeyTab)
	m, _ = mcpTestKeys(m, " ")
	write := 2 + slices.Index(m.mcp.modal.scopes, "contacts.write")
	for field := 1; field < write; field++ {
		m, _ = press(m, tea.KeyDown)
		if m.mcp.modal.field != field+1 {
			t.Fatalf("Down landed on field %d, want %d", m.mcp.modal.field, field+1)
		}
	}
	m, _ = mcpTestKeys(m, " ")
	m, _ = press(m, tea.KeyEnter)
	if !strings.Contains(m.View(), "WRITE · contacts.write") {
		t.Fatal("write access was not visible before save")
	}
	m, cmd := mcpTestKeys(m, "y")
	m = mcpTestDeliver(t, m, cmd)
	m, cmd = mcpTestKeys(m, "d")
	m = mcpTestDeliver(t, m, cmd)
	frame := m.View()
	for _, want := range []string{"stdio discovery passed", "--demo mcp config demo-client", "contact_create · WRITE"} {
		if !strings.Contains(frame, want) {
			t.Errorf("discovery omitted %q", want)
		}
	}
	m, _ = press(m, tea.KeyEsc)
	m, cmd = mcpTestKeys(m, "a")
	m = mcpTestDeliver(t, m, cmd)
	if !strings.Contains(m.View(), "profile_update") {
		t.Fatal("workspace audit did not show the confirmed profile update")
	}
	m, _ = press(m, tea.KeyEsc)
	m, _ = press(m, tea.KeyEnter)
	m, _ = mcpTestKeys(m, " ")
	m, _ = press(m, tea.KeyEnter)
	p, err := store.MCPProfile(ctx, "demo-client")
	if err != nil || !p.Enabled || !slices.Equal(p.Scopes, []string{"contacts.write"}) {
		t.Fatalf("current profile = %+v, %v", p, err)
	}
	fresh, err := store.SaveMCPProfile(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	m, cmd = mcpTestKeys(m, "y")
	m = mcpTestDeliver(t, m, cmd)
	p, err = store.MCPProfile(ctx, "demo-client")
	if err != nil || !p.Enabled || p.Revision != fresh.Revision || !strings.Contains(m.View(), "Not saved") {
		t.Fatalf("stale confirmation overwrote or hid the conflict: %+v, %v", p, err)
	}
}
