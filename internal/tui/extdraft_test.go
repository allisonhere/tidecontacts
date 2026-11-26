package tui

import (
	"strings"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/ext"
)

func TestExtensionDraftStateStaysWithItsDestinationFrame(t *testing.T) {
	m := panelApp(t, 110, 32)
	v := m.extensions.view
	v.cur.state = map[string]string{"filter": "currency:USD"}
	draft := `{"definition":"` + strings.Repeat("predicate ", 250) + `"}`
	r := ext.SanitizeResult(ext.ActionResult{Open: &ext.Target{Kind: "ext", Tab: "command", ID: "draft"}, State: map[string]string{
		"draft": draft, "invalid": "\x1b[31mx", "oversized": strings.Repeat("x", (64<<10)+1),
	}})
	m, command := m.applyExtResult(v, r)
	m = deliver(m, command)
	if got := v.cur.state; got["draft"] != draft || got["filter"] != "currency:USD" || len(got) != 2 {
		t.Fatalf("destination got %d fields, draft bytes=%d, filter=%q", len(got), len(got["draft"]), got["filter"])
	}
	m = do(m, "esc")
	if got := m.extensions.view.cur.state; len(got) != 1 || got["filter"] != "currency:USD" {
		t.Fatalf("return changed caller context: %+v", got)
	}
}
