package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

func taskField(t *testing.T, m Model, key string) Model {
	t.Helper()
	for i := 0; i < len(m.crm.form.fields); i++ {
		if m.crm.form.fields[m.crm.form.selected].key == key {
			return m
		}
		m = do(m, "tab")
	}
	t.Fatalf("missing task field %s", key)
	return m
}
func TestNewTaskUsesFormAndSearchableLinks(t *testing.T) {
	m, store := crmApp(t, 120, 36)
	alex, _ := seedPeople(t, store)
	project, err := store.SaveProject(context.Background(), domain.Project{Name: "Harbor"})
	if err != nil {
		t.Fatal(err)
	}
	m = do(m, "5", "n")
	if m.crm.form == nil || m.crm.prompt != nil {
		t.Fatal("new task did not open form")
	}
	m = do(m, "Call Alex")
	m = taskField(t, m, "priority")
	m = do(m, "right")
	m = taskField(t, m, "due")
	m = do(m, "tomorrow")
	m = taskField(t, m, "people")
	m = do(m, " ", "Alex")
	if m.crm.picker == nil || m.crm.form == nil {
		t.Fatal("search picker missing")
	}
	m = do(m, "enter")
	if m.crm.picker != nil || m.crm.form.fields[m.crm.form.selected].references[0].ID != alex.ID {
		t.Fatal("person selection lost")
	}
	m = taskField(t, m, "projects")
	m = do(m, " ", "Harbor", "enter")
	m = do(m, "f2")
	if m.crm.form != nil || m.crm.err != "" {
		t.Fatal("form did not save", m.crm.err)
	}
	tasks, err := store.Tasks(context.Background(), app.TaskFilter{View: "all"})
	if err != nil || len(tasks) != 1 {
		t.Fatal(tasks, err)
	}
	full, err := store.Task(context.Background(), tasks[0].ID)
	if err != nil || full.Title != "Call Alex" || full.Due != "2026-11-14" || len(full.Entities) != 1 || full.Entities[0].ID != alex.ID || len(full.Projects) != 1 || full.Projects[0].ID != project.ID {
		t.Fatal(full, err)
	}
}
func TestTaskFormKeepsContextAndCancelWritesNothing(t *testing.T) {
	m, store := crmApp(t, 100, 30)
	alex, _ := seedPeople(t, store)
	m = do(m, "1", "/", "Alex", "enter")
	m = do(m, "T")
	if m.crm.form == nil {
		t.Fatal("T did not open form")
	}
	refs := m.formReferences("people")
	if len(refs) != 1 || refs[0].ID != alex.ID {
		t.Fatal("context person lost", refs)
	}
	m = do(m, "Unsaved task")
	m = taskField(t, m, "people")
	m = do(m, " ", "esc")
	if m.crm.form == nil || m.crm.picker != nil {
		t.Fatal("picker cancel lost task")
	}
	m = do(m, "esc")
	if m.crm.form == nil || !strings.Contains(m.crm.form.confirm, "Discard") {
		t.Fatal("missing discard confirmation")
	}
	m = do(m, "y")
	tasks, err := store.Tasks(context.Background(), app.TaskFilter{View: "all"})
	if err != nil || len(tasks) != 0 {
		t.Fatal("cancel wrote task", tasks, err)
	}
}
func TestTaskFormReferenceSelectionUsesIdentityAndCanRemove(t *testing.T) {
	m, store := crmApp(t, 120, 36)
	ctx := context.Background()
	people := []domain.Contact{domain.NewContact("person"), domain.NewContact("person")}
	people[0].DisplayName = "Same Name"
	people[1].DisplayName = "Same Name"
	saved, err := store.SaveContacts(ctx, people, "create")
	if err != nil {
		t.Fatal(err)
	}
	m = do(m, "5", "n", "Identity test")
	m = taskField(t, m, "people")
	m = do(m, " ", "Same")
	if len(m.crm.picker.options) != 2 {
		t.Fatal("missing homonyms")
	}
	selectedID := m.crm.picker.options[1].ID
	m = do(m, "down", "enter", " ", "Same", "down", "enter")
	if len(m.formReferences("people")) != 0 {
		t.Fatal("same identity did not toggle off")
	}
	m = do(m, " ", "Same", "down", "enter")
	// Send the actual control key to remove the last link.
	m, _ = press(m, tea.KeyCtrlD)
	if len(m.formReferences("people")) != 0 {
		t.Fatal("remove last link failed")
	}
	m = do(m, " ", "Same", "down", "enter", "f2")
	tasks, err := store.Tasks(ctx, app.TaskFilter{View: "all"})
	if err != nil || len(tasks) != 1 {
		t.Fatal(tasks, err)
	}
	task, err := store.Task(ctx, tasks[0].ID)
	if err != nil || len(task.Entities) != 1 || task.Entities[0].ID != selectedID {
		t.Fatal("selected identity was resolved by ambiguous name", task, err, saved)
	}
}
