package ext

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tidecontacts/internal/domain"
)

func TestAttentionAndEngagementReuseCommunityRulesBehindPermissions(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	p := domain.NewContact("person")
	p.DisplayName = "Ada Fictional"
	saved, err := store.SaveContacts(ctx, []domain.Contact{p}, "test")
	if err != nil {
		t.Fatal(err)
	}
	id := saved[0].ID
	ref := domain.Ref{Kind: "person", ID: id, Name: "Ada Fictional"}
	past := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	task, err := store.SaveTask(ctx, domain.Task{Title: "Send the report", Due: past, Status: "next", Entities: []domain.Ref{ref}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveActivity(ctx, domain.Activity{Type: "call", Title: "Kickoff", Provenance: domain.Local, Entities: []domain.Ref{ref}, OccurredAt: time.Now().AddDate(0, 0, -5)}); err != nil {
		t.Fatal(err)
	}
	all := NewPermissionSet("attention.read", "attention.write", "contacts.read", "tasks.read", "tasks.write", "activities.read")
	deny := func(drop, method string, args any) {
		t.Helper()
		set := NewPermissionSet(all.Sorted()...)
		delete(set, drop)
		if _, e := call(t, &Session{ID: "bi-ext", Repo: store, Granted: set}, method, args); e == nil || e.Code != CodePermissionDenied {
			t.Errorf("%s without %s: %v", method, drop, e)
		}
	}
	for _, perm := range []string{"attention.read", "contacts.read", "tasks.read"} {
		deny(perm, "attention.list", nil)
	}
	deny("attention.write", "attention.act", map[string]any{"key": "x", "action": "dismiss"})
	deny("tasks.write", "tasks.complete", map[string]any{"id": task.ID})
	for _, perm := range []string{"activities.read", "tasks.read", "contacts.read"} {
		deny(perm, "contacts.engagement", map[string]any{"ids": []string{id}})
	}
	s := &Session{ID: "bi-ext", Repo: store, Granted: all}
	list := func() []AttentionItem {
		t.Helper()
		raw, e := call(t, s, "attention.list", nil)
		if e != nil {
			t.Fatal(e)
		}
		var items []AttentionItem
		if err := json.Unmarshal(raw, &items); err != nil {
			t.Fatal(err)
		}
		return items
	}
	var overdue AttentionItem
	for _, it := range list() {
		if it.Kind == "task-overdue" {
			overdue = it
		}
	}
	if overdue.Key == "" || overdue.Reason == "" || overdue.SourceID != task.ID {
		t.Fatalf("no overdue entry: %+v", list())
	}
	raw, e := call(t, s, "contacts.engagement", map[string]any{"ids": []string{id, "nobody"}})
	if e != nil {
		t.Fatal(e)
	}
	var g []Engagement
	_ = json.Unmarshal(raw, &g)
	if len(g) != 2 || g[0].LastType != "call" || g[0].LastInteraction == "" || g[0].OpenTaskCount != 1 || len(g[0].Tasks) != 1 || !g[1].Missing {
		t.Fatalf("%s", raw)
	}
	if _, e := call(t, s, "contacts.engagement", map[string]any{"ids": strings.Split(strings.Repeat("x,", 300), ",")}); e == nil || e.Code != CodeInvalid {
		t.Fatalf("unbounded ids: %v", e)
	}
	// Snoozing through the host hides the entry; the Community rules own it.
	if _, e := call(t, s, "attention.act", map[string]any{"key": overdue.Key, "action": "snooze", "until": time.Now().AddDate(0, 0, 3).Format("2006-01-02")}); e != nil {
		t.Fatal(e)
	}
	for _, it := range list() {
		if it.Key == overdue.Key {
			t.Fatal("snoozed entry still open")
		}
	}
	if _, e := call(t, s, "attention.act", map[string]any{"key": overdue.Key, "action": "erase"}); e == nil {
		t.Fatal("unknown action accepted")
	}
	if _, e := call(t, s, "tasks.complete", map[string]any{"id": task.ID}); e != nil {
		t.Fatal(e)
	}
	if done, _ := store.Task(ctx, task.ID); done.Status != "completed" {
		t.Fatal(done.Status)
	}
}
