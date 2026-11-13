package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
	"github.com/allisonhere/tidecontacts/internal/transfer"
	tea "github.com/charmbracelet/bubbletea"
)

const linkHelp = "Comma-separated exact names or IDs. Use L on a record to pick from a searchable list instead."

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return transfer.SplitTags(s)
}

func dateField(value, today string) (string, error) {
	return domain.ParseWhen(value, today)
}

func (m *Model) taskForm(t domain.Task) tea.Cmd {
	title := "New task"
	if t.Revision > 0 {
		title = "Edit task"
	}
	if t.Status == "" {
		t.Status = "inbox"
	}
	const dates = "today, tomorrow, fri, +3d, +2w, YYYY-MM-DD, or empty"
	fields := []formField{
		field("title", "Title", t.Title, "What needs to happen."),
		field("status", "Status", t.Status, "Inbox is unprocessed; Waiting means someone else acts next.", domain.TaskStatuses...),
		field("priority", "Priority", domain.PriorityName(t.Priority), "", domain.Priorities...),
		field("due", "Due date", t.Due, dates),
		field("time", "Due time (HH:MM)", t.DueTime, "Optional 24-hour wall-clock time."),
		field("zone", "Time zone", t.Timezone, "Optional IANA zone such as Europe/Paris. Empty follows your local zone."),
		field("start", "Start date", t.Start, dates),
		field("repeat", "Repeat", t.Recurrence.String(), "daily, weekdays, every 2 weeks, weekly on mon wed, monthly on 31, yearly, until YYYY-MM-DD, x10, after completion"),
		field("snooze", "Snoozed until", t.SnoozedUntil, dates),
		field("people", "People and organizations", refsText(t.Entities), linkHelp),
		field("projects", "Projects", refsText(t.Projects), linkHelp),
		field("tags", "Tags", strings.Join(t.Tags, ", "), "Comma-separated."),
		textArea("description", "Description", t.Description),
	}
	return m.openForm(title, fields, func(m *Model, v map[string]string) (tea.Cmd, error) {
		today := domain.Day(clock())
		next := t
		var e error
		next.Title, next.Status, next.DueTime, next.Timezone, next.Description = v["title"], v["status"], strings.TrimSpace(v["time"]), strings.TrimSpace(v["zone"]), v["description"]
		next.Priority, _ = domain.ParsePriority(v["priority"])
		for key, dst := range map[string]*string{"due": &next.Due, "start": &next.Start, "snooze": &next.SnoozedUntil} {
			if *dst, e = dateField(v[key], today); e != nil {
				return nil, e
			}
		}
		anchor := next.Recurrence.Anchor
		if next.Recurrence, e = domain.ParseRecurrence(v["repeat"]); e != nil {
			return nil, e
		}
		if !next.Recurrence.IsZero() && t.Recurrence.String() == next.Recurrence.String() {
			next.Recurrence.Anchor = anchor
		}
		next.Entities, next.Projects, next.Tags = refsFrom(v["people"], t.Entities), refsFrom(v["projects"], t.Projects), splitList(v["tags"])
		next.Normalize()
		if e = next.Validate(); e != nil {
			return nil, e
		}
		repo := m.crm.repo
		return m.crmDo("Task saved", func(ctx context.Context) (string, error) {
			saved, e := repo.SaveTask(ctx, next)
			return saved.ID, e
		}), nil
	})
}

func (m *Model) activityForm(a domain.Activity) tea.Cmd {
	title := "Log activity"
	if a.Revision > 0 {
		title = "Edit activity"
	}
	now := clock()
	when := now.Format("2006-01-02 15:04")
	if !a.OccurredAt.IsZero() {
		when = a.OccurredAt.Format("2006-01-02 15:04")
	}
	if a.Visibility == "" {
		a.Visibility = "private"
	}
	fields := []formField{
		field("type", "Type", orDefault(a.Type, "note"), "email, call, meeting, conversation, message, note, event — or any single word of your own."),
		field("title", "Title", a.Title, "A short summary. Leave empty to use the first line of the notes."),
		field("when", "When", when, "YYYY-MM-DD HH:MM, or today 14:30, yesterday, fri…"),
		field("people", "People and organizations", refsText(a.Entities), linkHelp),
		field("projects", "Projects", refsText(a.Projects), linkHelp),
		field("tags", "Tags", strings.Join(a.Tags, ", "), "Comma-separated."),
		field("visibility", "Visibility", a.Visibility, "Private entries are for you; shared marks what you would be comfortable exporting.", "private", "shared"),
		textArea("body", "Notes", a.Body),
	}
	return m.openForm(title, fields, func(m *Model, v map[string]string) (tea.Cmd, error) {
		next := a
		at, e := whenFrom(v["when"], clock())
		if e != nil {
			return nil, e
		}
		next.Type, next.Title, next.Body, next.Visibility, next.OccurredAt = strings.TrimSpace(v["type"]), v["title"], v["body"], v["visibility"], at
		next.Entities, next.Projects, next.Tags = refsFrom(v["people"], a.Entities), refsFrom(v["projects"], a.Projects), splitList(v["tags"])
		check := next
		check.Normalize(clock())
		if e = check.Validate(); e != nil {
			return nil, e
		}
		repo := m.crm.repo
		return m.crmDo("Activity saved", func(ctx context.Context) (string, error) {
			saved, e := repo.SaveActivity(ctx, next)
			return saved.ID, e
		}), nil
	})
}

func (m *Model) projectForm(p domain.Project) tea.Cmd {
	title := "New project"
	if p.Revision > 0 {
		title = "Edit project"
	}
	if p.Status == "" {
		p.Status = "active"
	}
	fields := []formField{
		field("name", "Name", p.Name, ""),
		field("status", "Status", p.Status, "", domain.ProjectStatuses...),
		field("priority", "Priority", domain.PriorityName(p.Priority), "", domain.Priorities...),
		field("start", "Start date", p.Start, "today, +2w, YYYY-MM-DD, or empty"),
		field("target", "Target date", p.Target, "today, +2w, YYYY-MM-DD, or empty"),
		field("people", "Collaborators and organizations", refsText(p.Members), "Comma-separated names; add a role as Name (role). "+linkHelp),
		field("tags", "Tags", strings.Join(p.Tags, ", "), "Comma-separated."),
		field("description", "Description", p.Description, "One line on what this project is."),
		textArea("notes", "Project notes", p.Notes),
	}
	return m.openForm(title, fields, func(m *Model, v map[string]string) (tea.Cmd, error) {
		today := domain.Day(clock())
		next := p
		var e error
		next.Name, next.Status, next.Description, next.Notes = v["name"], v["status"], v["description"], v["notes"]
		next.Priority, _ = domain.ParsePriority(v["priority"])
		if next.Start, e = dateField(v["start"], today); e != nil {
			return nil, e
		}
		if next.Target, e = dateField(v["target"], today); e != nil {
			return nil, e
		}
		next.Members, next.Tags = refsFrom(v["people"], p.Members), splitList(v["tags"])
		next.Normalize()
		if e = next.Validate(); e != nil {
			return nil, e
		}
		repo := m.crm.repo
		return m.crmDo("Project saved", func(ctx context.Context) (string, error) {
			saved, e := repo.SaveProject(ctx, next)
			return saved.ID, e
		}), nil
	})
}

func (m *Model) itemForm(it domain.PipelineItem, p domain.Pipeline) tea.Cmd {
	title := "New item in " + p.Name
	if it.Revision > 0 {
		title = "Edit pipeline item"
	}
	stages := []string{}
	for _, g := range p.Stages {
		stages = append(stages, g.Name)
		if it.StageID == g.ID {
			it.Stage = g.Name
		}
	}
	if it.Stage == "" && len(stages) > 0 {
		it.Stage = stages[0]
	}
	fields := []formField{
		field("title", "Title", it.Title, "What is moving through this pipeline: an application, a partnership, a goal…"),
		field("stage", "Stage", it.Stage, "", stages...),
		field("next", "Next action", it.NextAction, "The one thing that moves this forward."),
		field("people", "People and organizations", refsText(it.Entities), linkHelp),
		field("projects", "Projects", refsText(it.Projects), linkHelp),
		field("tags", "Tags", strings.Join(it.Tags, ", "), "Comma-separated."),
		textArea("notes", "Notes", it.Notes),
	}
	return m.openForm(title, fields, func(m *Model, v map[string]string) (tea.Cmd, error) {
		next := it
		next.PipelineID, next.Title, next.NextAction, next.Notes = p.ID, strings.TrimSpace(v["title"]), v["next"], v["notes"]
		if next.Title == "" {
			return nil, fmt.Errorf("a pipeline item needs a title")
		}
		for _, g := range p.Stages {
			if g.Name == v["stage"] {
				next.StageID = g.ID
			}
		}
		next.Entities, next.Projects, next.Tags = refsFrom(v["people"], it.Entities), refsFrom(v["projects"], it.Projects), splitList(v["tags"])
		repo := m.crm.repo
		return m.crmDo("Item saved", func(ctx context.Context) (string, error) {
			saved, e := repo.SaveItem(ctx, next)
			return saved.ID, e
		}), nil
	})
}

func (m *Model) pipelineForm(p domain.Pipeline) tea.Cmd {
	title := "New pipeline"
	stages := "To do, Doing, Done"
	if p.ID != "" {
		title = "Edit pipeline"
		names := make([]string, len(p.Stages))
		for i, g := range p.Stages {
			names[i] = g.Name
		}
		stages = strings.Join(names, ", ")
	}
	fields := []formField{
		field("name", "Name", p.Name, "For example Job applications, Partnerships, Client onboarding or Personal goals."),
		field("description", "Description", p.Description, ""),
		field("stages", "Stages, in order", stages, "Comma-separated. Existing names keep their items; reorder freely. Use the Rename stage command to rename."),
	}
	return m.openForm(title, fields, func(m *Model, v map[string]string) (tea.Cmd, error) {
		next := p
		next.Name, next.Description, next.Stages = v["name"], v["description"], nil
		for _, name := range splitList(v["stages"]) {
			g := domain.Stage{Name: strings.TrimSpace(name)}
			for _, old := range p.Stages {
				if strings.EqualFold(old.Name, g.Name) {
					g.ID = old.ID
				}
			}
			next.Stages = append(next.Stages, g)
		}
		if strings.TrimSpace(next.Name) == "" || len(next.Stages) == 0 {
			return nil, fmt.Errorf("a pipeline needs a name and at least one stage")
		}
		repo := m.crm.repo
		return m.crmDo("Pipeline saved", func(ctx context.Context) (string, error) {
			saved, e := repo.SavePipeline(ctx, next)
			return saved.ID, e
		}), nil
	})
}

func (m *Model) collectionForm(c domain.Collection) tea.Cmd {
	pinned := "no"
	if c.Pinned {
		pinned = "yes"
	}
	fields := []formField{
		field("name", "Name", c.Name, ""),
		field("description", "Description", c.Description, ""),
		field("entity", "Record type", c.Entity, "What the collection lists.", domain.CollectionEntities...),
		field("query", "Filter", c.Query, "The same search grammar as / in that workspace, e.g. is:open project:Harbor or followup:week."),
		field("sort", "Sort", c.Sort, "People: name, organization, updated, added. Tasks: due, priority, title, updated, created."),
		field("group", "Group by", c.Group, "Tasks: status, due, priority, person, project. People: org. Empty for none."),
		field("pinned", "Pinned", pinned, "Pinned task collections also appear as views in Tasks.", "no", "yes"),
	}
	return m.openForm("Smart collection", fields, func(m *Model, v map[string]string) (tea.Cmd, error) {
		next := c
		next.Name, next.Description, next.Entity, next.Query, next.Sort, next.Group, next.Pinned = v["name"], v["description"], v["entity"], strings.TrimSpace(v["query"]), strings.TrimSpace(v["sort"]), strings.TrimSpace(v["group"]), v["pinned"] == "yes"
		if _, e := search.ParseScope(next.Entity, next.Query); e != nil {
			return nil, e
		}
		next.Normalize()
		if e := next.Validate(); e != nil {
			return nil, e
		}
		repo := m.crm.repo
		return m.crmDo("Collection saved", func(ctx context.Context) (string, error) {
			saved, e := repo.SaveCollection(ctx, next)
			return saved.ID, e
		}), nil
	})
}

func (m *Model) radarFormFor(cfg domain.RadarConfig) tea.Cmd {
	fields := []formField{}
	for _, def := range domain.RadarSignals {
		state := "on"
		if cfg.Disabled[def[0]] {
			state = "off"
		}
		fields = append(fields, field("signal:"+def[0], "Signal: "+def[1], state, "Every signal can be switched off.", "on", "off"))
	}
	fields = append(fields,
		field("upcoming", "Important-date window (days)", strconv.Itoa(cfg.UpcomingDays), "Dates this many days ahead appear in Radar and the People Inbox."),
		field("active", "Recently-active window (days)", strconv.Itoa(cfg.ActiveDays), ""),
		field("minimum", "Interactions needed to count as active", strconv.Itoa(cfg.ActiveMinimum), ""),
		field("lead", "Show follow-ups this many days early", strconv.Itoa(cfg.LeadDays), ""),
		field("waiting", "Waiting tasks surface in the inbox after (days)", strconv.Itoa(cfg.WaitingDays), ""),
		field("limit", "Rows per signal", strconv.Itoa(cfg.Limit), ""))
	return m.openForm("Relationship Radar settings", fields, func(m *Model, v map[string]string) (tea.Cmd, error) {
		next := domain.RadarConfig{Disabled: map[string]bool{}}
		for _, def := range domain.RadarSignals {
			if v["signal:"+def[0]] == "off" {
				next.Disabled[def[0]] = true
			}
		}
		for key, dst := range map[string]*int{"upcoming": &next.UpcomingDays, "active": &next.ActiveDays, "minimum": &next.ActiveMinimum, "lead": &next.LeadDays, "waiting": &next.WaitingDays, "limit": &next.Limit} {
			n, e := strconv.Atoi(strings.TrimSpace(v[key]))
			if e != nil || n < 0 {
				return nil, fmt.Errorf("%s must be a whole number", key)
			}
			*dst = n
		}
		repo := m.crm.repo
		return m.crmDo("Radar settings saved", func(ctx context.Context) (string, error) { return "", repo.SaveRadarConfig(ctx, next) }), nil
	})
}

// contextRef is the record the current selection is about, used to pre-link quick captures.
func (m Model) contextRef() domain.Ref {
	if m.isContacts() {
		if c := m.contacts.contact; c.ID != "" {
			return domain.Ref{Kind: c.Kind, ID: c.ID, Name: c.DisplayName}
		}
		return domain.Ref{}
	}
	if !m.isCRM() {
		return domain.Ref{}
	}
	row, ok := m.listRow()
	if !ok {
		return domain.Ref{}
	}
	switch row.kind {
	case "inbox":
		it := row.data.(domain.InboxItem)
		if it.Person.ID != "" {
			return it.Person
		}
		if it.Source.Kind == "reminder" {
			return domain.Ref{}
		}
		return it.Source
	case "signal", "followup", "task", "activity", "project", "item", "person", "organization":
		return row.ref
	case "reminder":
		r := row.data.(domain.Reminder)
		if r.Entity.ID != "" {
			return r.Entity
		}
		return r.Task
	}
	return domain.Ref{}
}

// personRef narrows the context to a person or organization, for follow-ups, dates and flags.
func (m Model) personRef() (domain.Ref, bool) {
	if ref := m.contextRef(); ref.Kind == "person" || ref.Kind == "organization" {
		return ref, true
	}
	if row, ok := m.target(); ok && m.isCRM() && (row.ref.Kind == "person" || row.ref.Kind == "organization") {
		return row.ref, true
	}
	return domain.Ref{}, false
}

func describe(ref domain.Ref) string {
	if ref.ID == "" {
		return ""
	}
	return " · linked to " + ref.Name
}

const captureHint = "Bare words are the title. Add with:\"Name\" project:Name tag:x due:fri at:14:30 pri:high every:week"

func (m *Model) quickTask(prefill string, link domain.Ref, extra func(*domain.Task)) tea.Cmd {
	return m.openPrompt("New task"+describe(link), captureHint, prefill, func(m *Model, value string) (tea.Cmd, error) {
		c, e := parseCapture(value, domain.Day(clock()))
		if e != nil {
			return nil, e
		}
		t := c.task()
		switch link.Kind {
		case "person", "organization":
			t.Entities = append(t.Entities, domain.Ref{ID: link.ID})
		case "project":
			t.Projects = append(t.Projects, domain.Ref{ID: link.ID})
		}
		if extra != nil {
			extra(&t)
		}
		t.Normalize()
		if e = t.Validate(); e != nil {
			return nil, e
		}
		repo := m.crm.repo
		note := "Task added: " + t.Title
		if t.Status == "inbox" {
			note = "Added to the task Inbox: " + t.Title
		}
		return m.crmDo(note, func(ctx context.Context) (string, error) {
			saved, e := repo.SaveTask(ctx, t)
			return saved.ID, e
		}), nil
	})
}

func (m *Model) quickActivity(kind string, link domain.Ref) tea.Cmd {
	label := map[string]string{"meeting": "Log meeting", "call": "Log call", "note": "Add note"}[kind]
	if label == "" {
		label = "Log activity"
	}
	return m.openPrompt(label+describe(link), "Bare words are the summary. Add type:call with:\"Name\" project:Name on:yesterday at:14:30 tag:x · e opens the full form afterwards", "", func(m *Model, value string) (tea.Cmd, error) {
		c, e := parseCapture(value, domain.Day(clock()))
		if e != nil {
			return nil, e
		}
		a := c.activity(clock(), kind)
		switch link.Kind {
		case "person", "organization":
			a.Entities = append(a.Entities, domain.Ref{ID: link.ID})
		case "project":
			a.Projects = append(a.Projects, domain.Ref{ID: link.ID})
		case "task":
			a.Tasks = append(a.Tasks, domain.Ref{ID: link.ID})
		}
		check := a
		check.Normalize(clock())
		if e = check.Validate(); e != nil {
			return nil, e
		}
		repo := m.crm.repo
		return m.crmDo("Logged "+check.Type+": "+check.Title, func(ctx context.Context) (string, error) {
			saved, e := repo.SaveActivity(ctx, a)
			return saved.ID, e
		}), nil
	})
}

func (m *Model) quickReminder(link domain.Ref) tea.Cmd {
	return m.openPrompt("Add reminder"+describe(link), "Title, then on:fri at:09:00 every:week with:\"Name\". Without a date it is set for tomorrow 09:00.", "", func(m *Model, value string) (tea.Cmd, error) {
		now := clock()
		c, e := parseCapture(value, domain.Day(now))
		if e != nil {
			return nil, e
		}
		day := c.on
		if day == "" {
			day = c.due
		}
		if day == "" {
			day = domain.AddDays(domain.Day(now), 1)
			if c.clock != "" {
				day = domain.Day(now)
			}
		}
		at, _ := domain.At(day, orDefault(c.clock, "09:00"), now.Location())
		r := domain.Reminder{Title: c.title, At: at, Recurrence: c.recurrence}
		switch {
		case len(c.entities) > 0:
			r.Entity = c.entities[0]
		case link.Kind == "person" || link.Kind == "organization":
			r.Entity = domain.Ref{ID: link.ID}
		case link.Kind == "task":
			r.Task = domain.Ref{ID: link.ID}
			if r.Title == "" {
				r.Title = link.Name
			}
		}
		if r.Title == "" {
			return nil, fmt.Errorf("a reminder needs a title")
		}
		repo := m.crm.repo
		return m.crmDo("Reminder set for "+at.Format("Mon 2006-01-02 15:04"), func(ctx context.Context) (string, error) {
			saved, e := repo.SaveReminder(ctx, r)
			return saved.ID, e
		}), nil
	})
}

func (m *Model) followUpPrompt(person domain.Ref) tea.Cmd {
	return m.openPrompt("Follow-up for "+person.Name, "A cadence (2w, monthly, quarterly, every 45 days), a date (fri, +3d, 2026-12-01), never, or none to clear", "", func(m *Model, value string) (tea.Cmd, error) {
		repo := m.crm.repo
		today := domain.Day(clock())
		interval, never, intervalErr := domain.ParseInterval(value)
		day, dateErr := domain.ParseWhen(value, today)
		if value != "" && value != "none" && intervalErr != nil && dateErr != nil {
			return nil, fmt.Errorf("use a cadence such as 2w or monthly, a date such as fri or 2026-12-01, never, or none")
		}
		return m.crmDo("Follow-up updated for "+person.Name, func(ctx context.Context) (string, error) {
			f, e := repo.FollowUp(ctx, person.ID)
			if e != nil {
				return "", e
			}
			f.Entity = person
			switch {
			case value == "" || value == "none":
				f.Interval, f.Never, f.NextDate, f.SnoozedUntil = domain.Interval{}, false, "", ""
			case never:
				f.Never = true
			case intervalErr == nil:
				f.Interval, f.Never, f.SnoozedUntil = interval, false, ""
			default:
				f.NextDate, f.Never, f.SnoozedUntil = day, false, ""
			}
			return person.ID, repo.SetFollowUp(ctx, f)
		}), nil
	})
}

func (m *Model) datePrompt(person domain.Ref) tea.Cmd {
	return m.openPrompt("Important date for "+person.Name, "Label then date: First met 2019-03-03 · Name day --07-26 (yearless dates repeat every year)", "", func(m *Model, value string) (tea.Cmd, error) {
		i := strings.LastIndex(value, " ")
		if i < 0 {
			return nil, fmt.Errorf("write a label followed by a YYYY-MM-DD or --MM-DD date")
		}
		d := domain.ImportantDate{Entity: person, Label: strings.TrimSpace(value[:i]), Date: value[i+1:], Annual: true}
		if !domain.DateValid(d.Date) || d.Label == "" {
			return nil, fmt.Errorf("write a label followed by a YYYY-MM-DD or --MM-DD date")
		}
		repo := m.crm.repo
		return m.crmDo("Date added for "+person.Name, func(ctx context.Context) (string, error) { return person.ID, repo.SaveDate(ctx, d) }), nil
	})
}

// relationshipAdd picks a target and a type, then saves through the contact aggregate
// so revision checks and history apply exactly as they do in the editor.
func (m *Model) relationshipAdd(person domain.Ref) tea.Cmd {
	return m.openPicker("Relate "+person.Name+" to…", "Type to search people and organizations", "entity", nil, func(m *Model, target domain.Ref) tea.Cmd {
		types := []domain.Ref{}
		for _, t := range []string{"friend", "family", "colleague", "collaborator", "client", "mentor", "partner", "introduced by"} {
			types = append(types, domain.Ref{Name: t})
		}
		return m.openPicker(person.Name+" → "+target.Name, "Pick a label or type a new one", "", types, func(m *Model, kind domain.Ref) tea.Cmd {
			repo := m.crm.repo
			return m.crmDo(fmt.Sprintf("%s is now %s of %s", target.Name, kind.Name, person.Name), func(ctx context.Context) (string, error) {
				if target.ID == person.ID {
					return "", fmt.Errorf("cannot relate a contact to itself")
				}
				c, e := repo.Contact(ctx, person.ID)
				if e != nil {
					return "", e
				}
				c.Relationships = append(c.Relationships, domain.Relationship{TargetID: target.ID, Type: kind.Name})
				_, e = repo.SaveContacts(ctx, []domain.Contact{c}, "relationship")
				return person.ID, e
			})
		})
	})
}

func (m *Model) confirmDo(text, note string, fn func(ctx context.Context) error) {
	m.crm.confirm = &crmConfirm{text: text, yes: func(m *Model) tea.Cmd {
		return m.crmDo(note, func(ctx context.Context) (string, error) { return "", fn(ctx) })
	}}
}
