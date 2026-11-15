package tui

import (
	"strings"
	"testing"
)

func TestSharedHelpFlow(t *testing.T) {
	m, _ := crmApp(t, 120, 35)
	m = do(m, "?")
	if m.overlay != help {
		t.Fatal("help missing")
	}
	if !strings.Contains(frame(m), "GETTING STARTED") || !strings.Contains(frame(m), "Global") {
		t.Fatal("categories missing")
	}
	m = do(m, "/", "tidemail", "enter")
	if m.helpBrowser.Searching || len(m.helpBrowser.Filtered()) == 0 {
		t.Fatal("search failed")
	}
	m = do(m, "esc")
	if m.overlay != help || m.helpBrowser.Input.Value() != "" {
		t.Fatal("filter clear failed")
	}
	m = do(m, "esc")
	if m.overlay != none {
		t.Fatal("close failed")
	}
}
