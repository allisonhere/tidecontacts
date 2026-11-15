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
		case strings.Contains(c.ID, "import") || strings.Contains(c.ID, "export"):
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
	groups[0].Entries = append(groups[0].Entries, tideui.HelpEntry{Key: "Current workspace", Description: domain.Workspaces()[m.state.Active].Description})
	return groups
}
