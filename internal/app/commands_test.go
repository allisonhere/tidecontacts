package app

import (
	"github.com/allisonhere/tidecontacts/internal/domain"
	"testing"
)

func TestRegistryAndNavigation(t *testing.T) {
	r := Defaults()
	matches := r.Filter("gt ppl")
	if len(matches) == 0 || matches[0].ID != "workspace-people" {
		t.Fatal(matches)
	}
	if len(r.Filter("zzzzzzz")) != 0 {
		t.Fatal("unexpected match")
	}
	if e := r.Register(Command{ID: "quit", Label: "duplicate"}); e == nil {
		t.Fatal("duplicate allowed")
	}
	s := State{}
	for i, w := range domain.Workspaces() {
		if !s.Navigate(w.ID) || s.Active != i {
			t.Fatal(w)
		}
	}
	if s.Navigate("unknown") {
		t.Fatal("unknown route")
	}
}
