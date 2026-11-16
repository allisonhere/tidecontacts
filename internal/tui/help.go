package tui

import (
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tideui"
	"strings"
)

func (m Model) helpSections() []tideui.HelpSection {
	groups := []tideui.HelpSection{{Name: "Getting started", Entries: []tideui.HelpEntry{{Key: "ctrl+g", Description: "Open Menu: choose a workspace or action, then Enter."}, {Key: "Navigation", Description: "Menu → record list → details on wide terminals; the menu opens as a modal on smaller terminals."}}}, {Name: "Global"}, {Name: "People & organizations"}, {Name: "Relationships"}, {Name: "Activities"}, {Name: "Tasks"}, {Name: "Projects & pipelines"}, {Name: "Insights"}, {Name: "Appearance & layout"}, {Name: "Import & export"}}
	for _, c := range m.registry.Commands() {
		i := 1
		switch {
		case strings.Contains(c.ID, "theme") || strings.Contains(c.ID, "grow") || strings.Contains(c.ID, "shrink") || c.ID == "density":
			i = 8
		case strings.Contains(c.ID, "import") || strings.Contains(c.ID, "export") || c.ID == "refresh-address-books":
			i = 9
		case strings.Contains(c.ID, "relationship") || strings.Contains(c.ID, "followup"):
			i = 3
		case strings.Contains(c.ID, "activity") || strings.Contains(c.ID, "meeting") || strings.Contains(c.ID, "call") || strings.Contains(c.ID, "tidemail"):
			i = 4
		case strings.Contains(c.ID, "task") || c.ID == "complete" || c.ID == "reopen" || c.ID == "snooze" || c.ID == "reschedule":
			i = 5
		case strings.Contains(c.ID, "project") || strings.Contains(c.ID, "pipeline") || strings.Contains(c.ID, "stage") || strings.Contains(c.ID, "item"):
			i = 6
		case strings.Contains(c.ID, "radar") || strings.Contains(c.ID, "inbox") || strings.Contains(c.ID, "collection") || strings.Contains(c.ID, "reminder"):
			i = 7
		case strings.Contains(c.ID, "contact") || strings.Contains(c.ID, "profile") || c.ID == "favorite" || c.ID == "archive":
			i = 2
		}
		key := strings.Join(c.Keys, " / ")
		if key == "" {
			key = "Commands"
		}
		groups[i].Entries = append(groups[i].Entries, tideui.HelpEntry{Key: key, Description: c.Label})
	}
	groups[4].Entries = append(groups[4].Entries,
		tideui.HelpEntry{Key: "New activity", Description: "A anywhere or n in Activities opens the labeled form; meeting/call/note actions preselect Type. Space/Ctrl+O searches People & orgs or Projects."},
		tideui.HelpEntry{Key: "Quick capture", Description: "One-line activity syntax is optional: choose Quick capture activity (one line) from Commands."},
	)
	groups[5].Entries = append(groups[5].Entries,
		tideui.HelpEntry{Key: "New task", Description: "T anywhere or n in Tasks opens a labeled form; Tab moves fields, Left/Right changes status/priority."},
		tideui.HelpEntry{Key: "Link records", Description: "In task/activity People & orgs or Projects, Space/Ctrl+O opens search; Enter toggles a link; Ctrl+D removes the last link."},
		tideui.HelpEntry{Key: "Quick capture", Description: "One-line task syntax is optional: choose Quick capture task (one line) from Commands."},
	)
	groups[9].Entries = append(groups[9].Entries,
		tideui.HelpEntry{Key: "Import sources", Description: "CSV, vCard, CRM JSON, Mutt aliases and khard/vdirsyncer folders are active; dimmed coming-soon sources are disabled."},
		tideui.HelpEntry{Key: "File browser", Description: "Enter opens folders/files; Left or Backspace goes up; p enters a path; b returns to sources; . toggles hidden files/folders (initially off)."},
		tideui.HelpEntry{Key: "Connect folder", Description: "Choose khard/vdirsyncer; n browses, s selects the current folder, p enters a path; preview then Enter and y confirms."},
		tideui.HelpEntry{Key: "Folder controls", Description: "r refreshes all connections; x then y disconnects the selected folder. Edit source vCards to change read-only contacts."},
		tideui.HelpEntry{Key: "TideMail", Description: "People r and recipient Ctrl+T refresh connected folders; TideContacts need not run. Failed scans keep cached contacts."},
	)
	groups[0].Entries = append(groups[0].Entries, tideui.HelpEntry{Key: "Current workspace", Description: domain.Workspaces()[m.state.Active].Description})
	return groups
}
