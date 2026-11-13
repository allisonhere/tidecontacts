// Package app owns navigation and commands, without terminal dependencies.
package app

import (
	"fmt"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/sahilm/fuzzy"
)

type Command struct {
	ID, Label string
	Keys      []string
	// Soft default keys yield to a user's own binding for another command, so a
	// configuration written for an earlier release keeps working unchanged.
	Soft bool
}

// phase3 commands were added after users may already have customized keys.
var phase3 = []Command{
	// Phase 3: contextual keys act on the selected record of the active workspace.
	{ID: "task-new", Label: "New task (quick capture)", Keys: []string{"T"}, Soft: true}, {ID: "activity-log", Label: "Log activity (quick capture)", Keys: []string{"A"}, Soft: true},
	{ID: "complete", Label: "Complete task, milestone or inbox entry", Keys: []string{"c"}, Soft: true}, {ID: "reopen", Label: "Reopen task", Keys: []string{"R"}, Soft: true},
	{ID: "snooze", Label: "Snooze task, reminder or inbox entry", Keys: []string{"z"}, Soft: true}, {ID: "reschedule", Label: "Reschedule: set due or follow-up date", Keys: []string{"S"}, Soft: true},
	{ID: "priority", Label: "Cycle priority", Keys: []string{"p"}, Soft: true}, {ID: "stage-next", Label: "Advance status or pipeline stage", Keys: []string{">"}, Soft: true}, {ID: "stage-previous", Label: "Move status or pipeline stage back", Keys: []string{"<"}, Soft: true},
	{ID: "link", Label: "Link people, organizations or projects", Keys: []string{"L"}, Soft: true}, {ID: "convert-task", Label: "Create task from selection", Keys: []string{"t"}, Soft: true},
	{ID: "followup-schedule", Label: "Set follow-up schedule", Keys: []string{"F"}, Soft: true}, {ID: "flag", Label: "Flag or unflag contact for review", Keys: []string{"!"}, Soft: true},
	{ID: "duplicate", Label: "Duplicate task", Keys: []string{"y"}, Soft: true}, {ID: "dismiss", Label: "Dismiss, unlink or remove", Keys: []string{"x"}, Soft: true}, {ID: "reviewed", Label: "Mark inbox entry reviewed", Keys: []string{"v"}, Soft: true},
	{ID: "open-external", Label: "Open external reference", Keys: []string{"o"}, Soft: true}, {ID: "milestone-add", Label: "Add project milestone", Keys: []string{"M"}, Soft: true},
	{ID: "collection-save", Label: "Save current filter as a smart collection", Keys: []string{"C"}, Soft: true}, {ID: "date-add", Label: "Add important date", Keys: []string{"d"}, Soft: true},
	{ID: "relationship-add", Label: "Add relationship", Keys: []string{"r"}, Soft: true}, {ID: "reminder-add", Label: "Add reminder", Keys: []string{"b"}, Soft: true},
	{ID: "meeting-log", Label: "Log meeting", Keys: nil, Soft: true}, {ID: "call-log", Label: "Log call", Keys: nil, Soft: true}, {ID: "note-add", Label: "Add note", Keys: nil, Soft: true},
	{ID: "project-new", Label: "New project", Keys: nil, Soft: true}, {ID: "project-open", Label: "Open project…", Keys: nil, Soft: true}, {ID: "item-new", Label: "New pipeline item", Keys: nil, Soft: true},
	{ID: "pipeline-new", Label: "New pipeline", Keys: nil, Soft: true}, {ID: "pipeline-edit", Label: "Rename pipeline / edit stages", Keys: nil, Soft: true}, {ID: "pipeline-archive", Label: "Archive or unarchive pipeline", Keys: nil, Soft: true},
	{ID: "pipeline-move", Label: "Move item to stage…", Keys: nil, Soft: true}, {ID: "pipeline-up", Label: "Move pipeline up", Keys: nil, Soft: true}, {ID: "pipeline-down", Label: "Move pipeline down", Keys: nil, Soft: true},
	{ID: "stage-add", Label: "Add pipeline stage", Keys: nil, Soft: true}, {ID: "stage-rename", Label: "Rename pipeline stage", Keys: nil, Soft: true}, {ID: "stage-remove", Label: "Remove empty pipeline stage", Keys: nil, Soft: true},
	{ID: "stage-left", Label: "Reorder stage left", Keys: nil, Soft: true}, {ID: "stage-right", Label: "Reorder stage right", Keys: nil, Soft: true},
	{ID: "relationship-open", Label: "Explore relationships of selected contact", Keys: nil, Soft: true}, {ID: "relationship-notes", Label: "Edit relationship notes", Keys: nil, Soft: true},
	{ID: "inbox-review", Label: "Review People Inbox", Keys: nil, Soft: true}, {ID: "radar-open", Label: "Open Relationship Radar", Keys: nil, Soft: true}, {ID: "radar-settings", Label: "Relationship Radar settings", Keys: nil, Soft: true},
	{ID: "collections-open", Label: "Open smart collections", Keys: nil, Soft: true}, {ID: "reminders-open", Label: "Open reminders", Keys: nil, Soft: true}, {ID: "followups-open", Label: "Open follow-up schedules", Keys: nil, Soft: true},
	{ID: "merge-contacts", Label: "Merge marked contact into selected contact", Keys: nil, Soft: true},
}

type Registry struct{ commands []Command }

func (r *Registry) Register(c Command) error {
	if c.ID == "" || c.Label == "" {
		return fmt.Errorf("command ID and label are required")
	}
	for _, old := range r.commands {
		if old.ID == c.ID {
			return fmt.Errorf("duplicate command %q", c.ID)
		}
	}
	r.commands = append(r.commands, c)
	return nil
}
func (r Registry) Commands() []Command { return append([]Command(nil), r.commands...) }
func (r Registry) Filter(q string) []Command {
	if q == "" {
		return r.Commands()
	}
	names := make([]string, len(r.commands))
	for i, c := range r.commands {
		names[i] = c.Label
	}
	var result []Command
	for _, m := range fuzzy.Find(q, names) {
		result = append(result, r.commands[m.Index])
	}
	return result
}
func (r Registry) Key(key string) string {
	for _, c := range r.commands {
		for _, k := range c.Keys {
			if k == key {
				return c.ID
			}
		}
	}
	return ""
}
func Defaults() Registry {
	r := Registry{}
	for _, c := range []Command{
		{ID: "down", Label: "Move selection down", Keys: []string{"j", "down"}}, {ID: "up", Label: "Move selection up", Keys: []string{"k", "up"}},
		{ID: "next", Label: "Focus next pane", Keys: []string{"tab", "l", "right"}}, {ID: "previous", Label: "Focus previous pane", Keys: []string{"shift+tab", "h", "left"}},
		{ID: "activate", Label: "Open selected workspace", Keys: []string{"enter"}}, {ID: "back", Label: "Go back", Keys: []string{"esc"}},
		{ID: "search", Label: "Search workspace", Keys: []string{"/"}}, {ID: "palette", Label: "Open command palette", Keys: []string{":", "ctrl+k"}},
		{ID: "help", Label: "Show contextual help", Keys: []string{"?"}}, {ID: "context", Label: "Show context pane", Keys: []string{"i"}},
		{ID: "quit", Label: "Quit TideContacts", Keys: []string{"q"}},
		{ID: "left-grow", Label: "Widen navigation pane", Keys: []string{"shift+right"}}, {ID: "left-shrink", Label: "Narrow navigation pane", Keys: []string{"shift+left"}},
		{ID: "right-grow", Label: "Widen context pane", Keys: []string{"shift+up"}}, {ID: "right-shrink", Label: "Narrow context pane", Keys: []string{"shift+down"}},
		{ID: "mark-contact", Label: "Mark contact for export", Keys: []string{" "}},
		{ID: "new-contact", Label: "New record in this workspace", Keys: []string{"n"}}, {ID: "edit-contact", Label: "Edit selected record", Keys: []string{"e"}},
		{ID: "favorite", Label: "Toggle favorite or pinned", Keys: []string{"f"}}, {ID: "archive", Label: "Archive or unarchive", Keys: []string{"a"}},
		{ID: "restore", Label: "Restore archived/deleted contact", Keys: nil}, {ID: "delete-contact", Label: "Delete selected record", Keys: []string{"D"}}, {ID: "undo-contact", Label: "Undo last contact change", Keys: []string{"u"}},
		{ID: "profile-next", Label: "Next view", Keys: []string{"]"}}, {ID: "profile-previous", Label: "Previous view", Keys: []string{"["}},
		{ID: "sort", Label: "Cycle sort or status filter", Keys: []string{"s"}}, {ID: "density", Label: "Toggle list density", Keys: nil},
		{ID: "import-contacts", Label: "Import CSV or vCard", Keys: []string{"I"}}, {ID: "export-filtered", Label: "Export filtered contacts", Keys: []string{"E"}}, {ID: "export-selected", Label: "Export marked or selected contacts", Keys: nil}, {ID: "export-all", Label: "Export all contacts", Keys: nil},
		{ID: "page-down", Label: "Page down", Keys: []string{"pgdown"}}, {ID: "page-up", Label: "Page up", Keys: []string{"pgup"}}, {ID: "top", Label: "Go to top", Keys: []string{"home"}}, {ID: "bottom", Label: "Go to bottom", Keys: []string{"end"}},
		{ID: "theme-dark", Label: "Use dark theme", Keys: nil}, {ID: "theme-light", Label: "Use light theme", Keys: nil}, {ID: "theme-contrast", Label: "Use high-contrast theme", Keys: nil},
	} {
		_ = r.Register(c)
	}
	for _, c := range phase3 {
		_ = r.Register(c)
	}
	for i, w := range domain.Workspaces() {
		_ = r.Register(Command{ID: "workspace-" + w.ID, Label: "Go to " + w.Name, Keys: []string{fmt.Sprint(i + 1)}})
	}
	return r
}

type State struct{ Active int }

func (s *State) Navigate(id string) bool {
	i := domain.WorkspaceIndex(id)
	if i < 0 {
		return false
	}
	s.Active = i
	return true
}
