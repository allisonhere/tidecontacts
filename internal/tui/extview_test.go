package tui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/ext"
	"github.com/charmbracelet/x/ansi"
)

const wsScript = `read line
id=$(printf '%s' "$line" | sed 's/.*"request_id":"\([^"]*\)".*/\1/')
reply() { printf '{"api":1,"type":"response","request_id":"%s","ok":true,"data":%s}\n' "$id" "$1"; }
case "$line" in
*'"method":"view.render"'*)
  case "$line" in
  *'"workspace":"reports"'*) reply '{"title":"Report workspace","heading":"Saved reports"}';;
  *'"item":"opp1"'*) reply '{"title":"Acme IT","dossier":true,"tabs":[{"id":"overview","title":"Overview"},{"id":"people","title":"People"}],"tab":"overview","sections":[{"title":"Deal","lines":[{"label":"Value","text":"$18,000"},{"label":"Contact","text":"Ada Fictional","target":{"kind":"contact","id":"c-1"}}]}],"actions":[{"key":"q","label":"Quit","id":"bad"}]}';;
  *) reply '{"title":"Sales","badge":"PROFESSIONAL","tabs":[{"id":"overview","title":"Overview"},{"id":"pipeline","title":"Pipeline"}],"tab":"pipeline","tiles":[{"label":"Pipeline","value":"$86,400","note":"14 opportunities"},{"label":"Weighted","value":"$47,200","tone":"ok"},{"label":"Won this month","value":"$18,500"}],"heading":"Consulting pipeline","subheading":"USD · Active","board":{"columns":[{"id":"disc","title":"Discovery","total":"$18,400","tone":"info","cards":[{"id":"opp0","title":"Nexus Systems","amount":"$12,000","progress":20,"left":"20%","right":"4 days"}]},{"id":"prop","title":"Proposal","total":"$28,000","tone":"warn","cards":[{"id":"opp1","title":"Acme IT","amount":"$18,000","progress":60,"left":"60%","right":"12 days","badge":"Follow-up overdue","badge_tone":"warn"},{"id":"opp2","title":"Orbit Digital","amount":"$10,000","progress":50,"left":"50%","right":"3 days"}]},{"id":"neg","title":"Negotiation","total":"$40,000","tone":"ok","cards":[{"id":"opp3","title":"Meridian","amount":"$25,000","progress":80}]}]},"notices":[{"tone":"warn","text":"Acme IT: proposal requires follow-up"}],"open":"open","searchable":true,"actions":[{"key":"n","label":"New","id":"new"},{"key":"e","label":"Edit","id":"edit","scope":"item"},{"key":"x","label":"Delete","id":"del","scope":"item","confirm":"Delete this opportunity?"}]}';;
  esac;;
*'"method":"action.run"'*)
  case "$line" in
  *'"action":"open"'*) reply '{"open":{"kind":"ext","tab":"overview","id":"opp1"}}';;
  *'"action":"new"'*) reply '{"form":{"id":"opp","title":"New opportunity","fields":[{"key":"title","label":"Title","type":"text","required":true},{"key":"stage","label":"Stage","type":"select","options":["Discovery","Proposal"]},{"key":"notes","label":"Notes","type":"multiline"},{"key":"contact","label":"Contact","type":"contact"}]}}';;
  *'"action":"del"'*) reply '{"notice":"Deleted","refresh":true}';;
  *) reply '{"notice":"done","refresh":true}';;
  esac;;
*'"method":"form.submit"'*)
  case "$line" in
  *'"title":"bad"'*) reply '{"errors":{"title":"Title already exists"}}';;
  *) reply '{"notice":"Created","refresh":true}';;
  esac;;
*) reply '{}';;
esac
`

func installWS(t *testing.T, root string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script extensions are Unix only")
	}
	dir := filepath.Join(root, "extensions", "sales-ext")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, ext.ManifestFile), []byte("id = \"sales-ext\"\nname = \"Sales\"\nversion = \"1.0.0\"\napi = 1\ncommand = \"run.sh\"\npermissions = [\"ui.register\"]\n[[workspaces]]\nid = \"sales\"\ntitle = \"Sales\"\n[[workspaces]]\nid = \"reports\"\ntitle = \"Reports\"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"+wsScript), 0o755)
}

func wsApp(t *testing.T, w, h int) Model {
	t.Helper()
	m, mgr, root := extApp(t, w, h)
	installWS(t, root)
	_ = mgr.Discover(ctxBackground())
	if err := mgr.Enable(ctxBackground(), "sales-ext", true); err != nil {
		t.Fatal(err)
	}
	next, cmd := m.dispatch("ext-ws:sales-ext:sales")
	return deliver(next.(Model), cmd)
}

func TestWorkspaceAppearsOnlyWhenEnabled(t *testing.T) {
	m, mgr, root := extApp(t, 140, 40)
	base := len(m.menuCommands())
	installWS(t, root)
	_ = mgr.Discover(ctxBackground())
	if len(m.menuCommands()) != base {
		t.Fatal("a disabled extension changed the menu")
	}
	_ = mgr.Enable(ctxBackground(), "sales-ext", true)
	cmds := m.menuCommands()
	if len(cmds) != base+2 || cmds[8].ID != "ext-ws:sales-ext:sales" || cmds[8].Label != "Sales" {
		t.Fatalf("menu entries missing: %+v", cmds)
	}
	m.prompt.SetValue("sales")
	found := false
	for _, c := range m.paletteMatches() {
		found = found || c.ID == "ext-ws:sales-ext:sales"
	}
	if !found {
		t.Fatal("workspace not offered by the palette")
	}
	_ = mgr.Disable(ctxBackground(), "sales-ext")
	if len(m.menuCommands()) != base {
		t.Fatal("menu entry survived disabling")
	}
}

func TestWorkspaceRendersBoardAndStaysBounded(t *testing.T) {
	m := wsApp(t, 160, 45)
	out := ansi.Strip(m.View())
	for _, want := range []string{"PROFESSIONAL", "overview", "[pipeline]", "PIPELINE", "$86,400", "14 opportunities", "WEIGHTED", "Consulting pipeline", "USD · Active", "Discovery", "Proposal", "Negotiation", "$28,000", "Nexus Systems", "Acme IT", "60%", "12 days", "Follow-up overdue", "proposal requires follow-up", "h/l", "stage", "open", "close"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Quit") {
		t.Fatal("an extension bound the reserved q key")
	}
	for _, size := range [][2]int{{160, 45}, {120, 30}, {100, 28}, {80, 24}, {60, 20}, {40, 14}, {30, 10}} {
		n := wsApp(t, size[0], size[1])
		checkBounds(t, n.View(), size[0], size[1])
		n = do(n, "l", "j", "j", "h", "enter")
		checkBounds(t, n.View(), size[0], size[1])
	}
}

func TestWorkspaceNavigationBackStackAndResume(t *testing.T) {
	m := wsApp(t, 140, 40)
	before := struct{ ws, focus int }{m.state.Active, m.focus}
	m = do(m, "l", "enter") // Proposal column, first card → open dossier
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "acme it") && !strings.Contains(out, "Acme IT") || !strings.Contains(out, "$18,000") || !strings.Contains(out, "Ada Fictional") {
		t.Fatalf("dossier not shown:\n%s", out)
	}
	if len(m.extensions.view.stack) != 1 {
		t.Fatal("dossier did not push the back stack")
	}
	seq := m.extensions.view.seq
	m = do(m, "]")
	if m.extensions.view.seq == seq {
		t.Fatal("switching tabs did not ask the extension to render")
	}
	m = do(m, "esc") // back to the board, same column and card
	v := m.extensions.view
	if v == nil || len(v.stack) != 0 || v.cur.col != 1 || v.doc.Board == nil {
		t.Fatalf("back did not restore the board: %+v", v)
	}
	sel, _ := v.selected()
	if sel.id != "opp1" {
		t.Fatalf("selection not preserved: %q", sel.id)
	}
	m = do(m, "esc") // close; main app state untouched
	if m.extensions.view != nil || m.state.Active != before.ws || m.focus != before.focus {
		t.Fatal("closing the workspace disturbed the app")
	}
	// Reopening resumes at the same column and card.
	next, cmd := m.dispatch("ext-ws:sales-ext:sales")
	m = deliver(next.(Model), cmd)
	if s, _ := m.extensions.view.selected(); m.extensions.view.cur.col != 1 || s.id != "opp1" {
		t.Fatalf("resume lost the place: col=%d sel=%q", m.extensions.view.cur.col, s.id)
	}
}

func TestWorkspaceFollowsCoreRecordLinks(t *testing.T) {
	m := wsApp(t, 140, 40)
	m = do(m, "l", "enter", "j", "enter")
	// The "Contact" line targets a core person; the first selectable line is it.
	m = do(m, "enter")
	if m.extensions.view != nil {
		t.Fatalf("workspace stayed open after following a core link:\n%s", ansi.Strip(m.View()))
	}
	if len(m.crm.nav) == 0 {
		t.Fatal("the previous place was not remembered for Esc")
	}
	m = do(m, "esc")
	if m.extensions.view == nil || m.extensions.view.cur.item != "opp1" || !strings.Contains(ansi.Strip(m.View()), "Ada Fictional") {
		t.Fatalf("Esc did not return to the linked dossier:\n%s", ansi.Strip(m.View()))
	}
}

func TestWorkspacePaletteAndCrossWorkspaceReturn(t *testing.T) {
	m := do(wsApp(t, 140, 40), "l")
	m = do(m, ":")
	m = do(m, "open reports")
	if !strings.Contains(ansi.Strip(m.View()), "Open Reports") {
		t.Fatalf("workspace did not open the host palette:\n%s", ansi.Strip(m.View()))
	}
	m = do(m, "esc")
	if v := m.extensions.view; v == nil || v.cur.col != 1 || m.overlay != none {
		t.Fatal("leaving the palette lost the workspace selection")
	}
	next, cmd := m.followExtTarget(m.extensions.view, ext.Target{Kind: "ext", Workspace: "reports", ID: "saved"})
	m = deliver(next, cmd)
	if !strings.Contains(ansi.Strip(m.View()), "Saved reports") {
		t.Fatalf("did not reach the sibling workspace:\n%s", ansi.Strip(m.View()))
	}
	m = do(m, "esc")
	if v := m.extensions.view; v == nil || v.ws != "sales" || v.cur.col != 1 || v.doc.Board == nil {
		t.Fatal("Esc did not restore the workspace and selected column")
	}
}

func TestWorkspaceFormsValidationAndSubmit(t *testing.T) {
	m := wsApp(t, 140, 40)
	m = do(m, "n")
	if m.crm.form == nil || !strings.Contains(ansi.Strip(m.View()), "new opportunity") {
		t.Fatalf("form did not open:\n%s", ansi.Strip(m.View()))
	}
	if !strings.Contains(ansi.Strip(m.View()), "Stage") {
		t.Fatal("select field not rendered")
	}
	// Required field is checked before anything is sent.
	m, cmd := key(m, "ctrl+s")
	m = deliver(m, cmd)
	if m.crm.form == nil || !strings.Contains(m.crm.form.err, "Title is required") {
		t.Fatalf("required field not enforced: %+v", m.crm.form)
	}
	// Server-side validation errors keep the form open on the right field.
	m, _ = key(m, "bad")
	m, cmd = key(m, "ctrl+s")
	m = deliver(m, cmd)
	if m.crm.form == nil || !strings.Contains(m.crm.form.err, "Title already exists") || m.crm.busy {
		t.Fatalf("extension error not shown: %+v busy=%v", m.crm.form, m.crm.busy)
	}
	// A valid answer closes the form and refreshes the view.
	for i := 0; i < 3; i++ {
		m, _ = key(m, "backspace")
	}
	m, _ = key(m, "good")
	m, cmd = key(m, "ctrl+s")
	m = deliver(m, cmd)
	if m.crm.form != nil || m.extensions.view == nil || m.extensions.view.notice != "Created" {
		t.Fatalf("submit failed: form=%v notice=%q", m.crm.form != nil, m.extensions.view.notice)
	}
	m, cmd = key(m, "n")
	m = deliver(m, cmd)
	m, _ = key(m, "esc") // cancel keeps the workspace
	m, _ = key(m, "y")
	if m.extensions.view == nil {
		t.Fatal("cancelling a form closed the workspace")
	}
}

func TestWorkspaceActionsAndConfirmation(t *testing.T) {
	m := wsApp(t, 140, 40)
	m = do(m, "x")
	if m.crm.confirm == nil || !strings.Contains(ansi.Strip(m.View()), "Delete this opportunity?") {
		t.Fatalf("destructive action not confirmed:\n%s", ansi.Strip(m.View()))
	}
	m = do(m, "n") // decline
	if m.crm.confirm != nil || m.extensions.view.notice == "Deleted" {
		t.Fatal("declined action ran")
	}
	m = do(m, "x", "y")
	if m.extensions.view.notice != "Deleted" {
		t.Fatalf("confirmed action did not run: %q", m.extensions.view.notice)
	}
	// Item-scoped actions need a selection; view-scoped ones do not.
	m = do(m, "/")
	if m.crm.prompt == nil {
		t.Fatal("search prompt did not open for a searchable view")
	}
	m = do(m, "esc")
}

const sigScript = `read line
id=$(printf '%s' "$line" | sed 's/.*"request_id":"\([^"]*\)".*/\1/')
reply() { printf '{"api":1,"type":"response","request_id":"%s","ok":true,"data":%s}\n' "$id" "$1"; }
case "$line" in
*'"method":"signals.list"'*) reply '[{"key":"stale-opp1","type":"stale_opportunity","title":"Acme IT is stale","severity":"warn","reason":"In Proposal for 21 days; expected 14.","facts":["Entered Proposal 2026-11-04","Expected stage duration: 14 days"],"date":"2026-11-25","action":"Send a follow-up","target":{"kind":"ext","tab":"overview","id":"opp1"}}]';;
*'"method":"action.run"'*) reply '{"notice":"ok"}';;
*) reply '{}';;
esac
`

func TestSignalsAppearInInboxWithoutDisturbingIt(t *testing.T) {
	m, mgr, root := extApp(t, 140, 40)
	dir := filepath.Join(root, "extensions", "sales-ext")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, ext.ManifestFile), []byte("id = \"sales-ext\"\nname = \"Sales\"\nversion = \"1.0.0\"\napi = 1\ncommand = \"run.sh\"\nsignals = true\npermissions = [\"ui.register\", \"signals.provide\"]\n[[workspaces]]\nid = \"sales\"\ntitle = \"Sales\"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"+sigScript), 0o755)
	_ = mgr.Discover(ctxBackground())
	if err := mgr.Enable(ctxBackground(), "sales-ext", true); err != nil {
		t.Fatal(err)
	}
	m = do(m, "8") // Insights, opens on the Inbox
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "Acme IT is stale") || !strings.Contains(out, "Sales attention") {
		t.Fatalf("contributed signal missing from the Inbox:\n%s", out)
	}
	// Detail states why, with the recorded facts and the suggested step.
	m = do(m, "j")
	for i := 0; i < 6 && !strings.Contains(ansi.Strip(m.View()), "Recorded facts"); i++ {
		m = do(m, "j")
	}
	out = ansi.Strip(m.View())
	for _, want := range []string{"In Proposal for 21 days", "Entered Proposal 2026-11-04", "Send a follow-up"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q", want)
		}
	}
	// Disabling the extension removes its rows; the built-in Inbox is unaffected.
	_ = mgr.Disable(ctxBackground(), "sales-ext")
	m = do(m, "8")
	if strings.Contains(ansi.Strip(m.View()), "Acme IT is stale") {
		t.Fatal("signal survived disabling")
	}
}

func TestBoardOpensOnTheFirstColumnWithCards(t *testing.T) {
	v := &extView{doc: ext.View{Board: &ext.Board{Columns: []ext.Column{{ID: "a", Title: "Empty"}, {ID: "b", Title: "Empty too"}, {ID: "c", Cards: []ext.Card{{ID: "x", Title: "X"}}}}}}}
	v.settle()
	if v.cur.col != 2 {
		t.Fatalf("cursor on an empty column: %d", v.cur.col)
	}
	if sel, ok := v.selected(); !ok || sel.id != "x" {
		t.Fatalf("%+v %v", sel, ok)
	}
	// An explicit choice is respected even if that column is empty.
	v = &extView{cur: extFrame{col: 1, selID: "keep"}, doc: v.doc}
	v.settle()
	if v.cur.col != 1 {
		t.Fatalf("a deliberate cursor was moved: %d", v.cur.col)
	}
}
