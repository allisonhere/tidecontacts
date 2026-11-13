// Package domain contains terminal-independent application vocabulary.
package domain

type Workspace struct{ ID, Name, Description string }

func Workspaces() []Workspace {
	return []Workspace{
		{"people", "People", "Contact profiles, methods, notes and each person's CRM dossier."},
		{"organizations", "Organizations", "Companies, teams and their members."},
		{"relationships", "Relationships", "Explore how people, organizations and projects connect."},
		{"activities", "Activities", "A timeline of calls, meetings, notes and other recorded interactions."},
		{"tasks", "Tasks", "Tasks, commitments, recurrence and reminders."},
		{"projects", "Projects", "Lightweight projects with people, tasks, milestones and notes."},
		{"pipelines", "Pipelines", "Optional multi-stage pipelines for any process you track."},
		{"insights", "Insights", "People Inbox, Relationship Radar, smart collections and reminders."},
	}
}
func WorkspaceIndex(id string) int {
	for i, w := range Workspaces() {
		if w.ID == id {
			return i
		}
	}
	return -1
}
