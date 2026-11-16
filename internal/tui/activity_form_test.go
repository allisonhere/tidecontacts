package tui

import (
	"context"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

func TestNewActivityFormPreservesContextAndSearchesLinks(t *testing.T) {
	m, store := crmApp(t, 120, 36)
	alex, _ := seedPeople(t, store)
	project, err := store.SaveProject(context.Background(), domain.Project{Name: "Harbor"})
	if err != nil {
		t.Fatal(err)
	}
	m = deliver(m, m.refreshContacts())
	m = do(m, "A")
	if m.crm.form == nil || m.crm.form.title != "New activity" || m.crm.prompt != nil {
		t.Fatal("A did not open activity form")
	}
	m = do(m, "Catch up", "tab")
	m, _ = press(m, tea.KeyCtrlU)
	m = do(m, "meeting", "tab")
	m, _ = press(m, tea.KeyCtrlU)
	m = do(m, "yesterday 10:30")
	m = taskField(t, m, "projects")
	m = do(m, " ", "Harbor", "enter")
	m = do(m, "f2")
	if m.crm.form != nil || m.contacts.err != "" {
		t.Fatal("activity save failed", m.contacts.err)
	}
	activities, err := store.Activities(context.Background(), app.ActivityFilter{})
	if err != nil || len(activities) != 1 {
		t.Fatal(activities, err)
	}
	activity := activities[0]
	if activity.Title != "Catch up" || activity.Type != "meeting" || activity.OccurredAt.Format("2006-01-02 15:04") != "2026-11-12 10:30" || len(activity.Entities) != 1 || activity.Entities[0].ID != alex.ID || len(activity.Projects) != 1 || activity.Projects[0].ID != project.ID {
		t.Fatalf("%+v", activity)
	}
}
func TestActivityCreationRoutesUseModal(t *testing.T) {
	for _, tc := range []struct{ command, kind string }{{"activity-log", "conversation"}, {"meeting-log", "meeting"}, {"call-log", "call"}, {"note-add", "note"}} {
		t.Run(tc.command, func(t *testing.T) {
			m, _ := crmApp(t, 100, 30)
			next, cmd := m.dispatch(tc.command)
			m = deliver(next.(Model), cmd)
			if m.crm.form == nil || m.crm.prompt != nil {
				t.Fatal("creation did not open form")
			}
			if got := m.crm.form.values()["type"]; got != tc.kind {
				t.Fatalf("type %q, want %q", got, tc.kind)
			}
			m = do(m, "Unsaved", "esc", "y")
			if m.crm.form != nil {
				t.Fatal("cancel did not close form")
			}
		})
	}
}
