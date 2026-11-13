package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/allisonhere/tidecontacts/internal/domain"
)

// crmRow is one line of a CRM list or detail pane. Rows are plain data built off
// the UI loop; View only styles and clips them.
type crmRow struct {
	kind  string // header, text, or a record kind: task, activity, project, item, person, …
	id    string
	tag   string // short fixed-width type or state indicator; never colour alone
	text  string
	meta  string
	muted bool
	ref   domain.Ref // where Enter leads
	data  any
}

func (r crmRow) selectable() bool { return r.kind != "header" && r.kind != "text" }

func head(s string) crmRow { return crmRow{kind: "header", text: s} }
func txt(s string) crmRow  { return crmRow{kind: "text", text: s} }
func dim(s string) crmRow  { return crmRow{kind: "text", text: s, muted: true} }
func lnk(ref domain.Ref, tag, text, meta string) crmRow {
	return crmRow{kind: "link", id: ref.ID, tag: tag, text: text, meta: meta, ref: ref}
}

func refTag(kind string) string {
	return map[string]string{"person": "who ", "organization": "org ", "project": "proj", "task": "task", "activity": "act ", "item": "item", "reminder": "rem "}[kind]
}

func refLinks(rows []crmRow, title string, refs []domain.Ref) []crmRow {
	if len(refs) == 0 {
		return rows
	}
	rows = append(rows, head(title))
	for _, r := range refs {
		rows = append(rows, lnk(r, refTag(r.Kind), r.Name, r.Role))
	}
	return rows
}

var taskTags = map[string]string{"inbox": "[?]", "next": "[ ]", "scheduled": "[>]", "waiting": "[w]", "completed": "[x]", "canceled": "[-]"}
var activityTags = map[string]string{"email": "mail", "call": "call", "meeting": "mtg ", "conversation": "conv", "message": "msg ", "note": "note", "event": "evnt", "task-completion": "done", "project-update": "proj"}

func activityTag(typ string) string {
	if t, ok := activityTags[typ]; ok {
		return t
	}
	return fmt.Sprintf("%-4.4s", typ)
}

func bang(p int) string { return strings.Repeat("!", max(0, p)) }

func taskRow(t domain.Task, today string) crmRow {
	meta := []string{}
	if p := bang(t.Priority); p != "" {
		meta = append(meta, p)
	}
	if t.SnoozedUntil > today {
		meta = append(meta, "zz "+t.SnoozedUntil[5:])
	}
	if !t.Recurrence.IsZero() {
		meta = append(meta, "↻")
	}
	if t.Due != "" {
		due := domain.Relative(t.Due, today)
		if !t.Open() {
			due = t.Due
		}
		if t.DueTime != "" {
			due += " " + t.DueTime
		}
		meta = append(meta, due)
	}
	text := t.Title
	if who := refsText(t.Entities); who != "" {
		text += " · " + who
	}
	return crmRow{kind: "task", id: t.ID, tag: taskTags[t.Status], text: text, meta: strings.Join(meta, " "), muted: !t.Open(), ref: domain.Ref{Kind: "task", ID: t.ID, Name: t.Title}, data: t}
}

// taskRows groups tasks the way each view promises: by urgency for Today, by date for
// Upcoming, by person or project for the relationship views, and by state otherwise.
func taskRows(tasks []domain.Task, view, today string) []crmRow {
	rows := []crmRow{}
	group := ""
	add := func(label string, t domain.Task) {
		if label != group {
			group = label
			rows = append(rows, head(label))
		}
		rows = append(rows, taskRow(t, today))
	}
	switch view {
	case "person", "project":
		buckets := map[string][]domain.Task{}
		for _, t := range tasks {
			refs := t.Projects
			if view == "person" {
				refs = t.Entities
			}
			if len(refs) == 0 {
				buckets["~"] = append(buckets["~"], t)
			}
			for _, r := range refs {
				buckets[r.Name] = append(buckets[r.Name], t)
			}
		}
		names := make([]string, 0, len(buckets))
		for n := range buckets {
			names = append(names, n)
		}
		sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
		for _, n := range names {
			label := n
			if n == "~" {
				label = "No " + view + " linked"
			}
			for _, t := range buckets[n] {
				add(fmt.Sprintf("%s (%d)", label, len(buckets[n])), t)
			}
		}
	default:
		for _, t := range tasks {
			label := ""
			switch view {
			case "today":
				label = "Today"
				if t.Due != "" && t.Due < today {
					label = "Overdue"
				} else if t.Due != today {
					label = "Starting"
				}
			case "upcoming":
				day := t.Due
				if day == "" {
					day = t.Start
				}
				if t.SnoozedUntil > day {
					day = t.SnoozedUntil
				}
				label = dayLabel(day, today)
			case "group:status", "":
				label = strings.ToUpper(t.Status[:1]) + t.Status[1:]
				if view == "" && t.Open() {
					label = "Open"
				}
			case "group:due":
				label = "No date"
				if t.Due != "" {
					label = dayLabel(t.Due, today)
				}
			case "group:priority":
				label = "Priority: " + domain.PriorityName(t.Priority)
			}
			if label == "" {
				rows = append(rows, taskRow(t, today))
			} else {
				add(label, t)
			}
		}
	}
	return rows
}

func dayLabel(day, today string) string {
	t, ok := domain.ParseDay(day)
	if !ok {
		return day
	}
	label := t.Format("Mon 2006-01-02")
	switch domain.DaysBetween(today, day) {
	case 0:
		label += " · today"
	case 1:
		label += " · tomorrow"
	case -1:
		label += " · yesterday"
	}
	return label
}

func provenanceMark(a domain.Activity) string {
	switch a.Provenance {
	case domain.External:
		return "↗ " + a.SourceApp
	case domain.Imported:
		return "⇣ " + a.SourceApp
	}
	return ""
}

func activityRow(a domain.Activity) crmRow {
	text := a.Title
	if who := refsText(a.Entities); who != "" {
		text += " · " + who
	}
	return crmRow{kind: "activity", id: a.ID, tag: a.OccurredAt.Format("15:04") + " " + activityTag(a.Type), text: text, meta: provenanceMark(a),
		ref: domain.Ref{Kind: "activity", ID: a.ID, Name: a.Title}, data: a}
}

// activityRows groups a timeline page by local calendar day.
func activityRows(list []domain.Activity, today string) []crmRow {
	rows := []crmRow{}
	day := ""
	for _, a := range list {
		if d := domain.Day(a.OccurredAt); d != day {
			day = d
			rows = append(rows, head(dayLabel(d, today)))
		}
		rows = append(rows, activityRow(a))
	}
	return rows
}

func projectRows(list []domain.Project, today string) []crmRow {
	rows := []crmRow{}
	status := ""
	for _, p := range list {
		if p.Status != status {
			status = p.Status
			rows = append(rows, head(strings.ToUpper(status[:1])+status[1:]))
		}
		meta := fmt.Sprintf("%d open", p.OpenTasks)
		if p.Target != "" {
			meta += " · " + domain.Relative(p.Target, today)
		}
		rows = append(rows, crmRow{kind: "project", id: p.ID, tag: bang(p.Priority), text: p.Name, meta: meta, muted: p.Status == "completed" || p.Status == "archived",
			ref: domain.Ref{Kind: "project", ID: p.ID, Name: p.Name}, data: p})
	}
	return rows
}

var inboxTags = map[string]string{"reminder": "bell", "task-overdue": "late", "followup-due": "f/up", "followup-suggested": "f/up", "task-waiting": "wait", "task-inbox": "new ", "date": "date", "flagged": "flag"}

func inboxRows(items []domain.InboxItem) []crmRow {
	rows := []crmRow{}
	group := ""
	for _, it := range items {
		label := []string{"Later", "Upcoming and unprocessed", "Due now", "Overdue"}[min(max(it.Urgency, 0), 3)]
		if it.Status == "reviewed" {
			label = "Reviewed"
		}
		if label != group {
			group = label
			rows = append(rows, head(label))
		}
		rows = append(rows, crmRow{kind: "inbox", id: it.Key, tag: inboxTags[it.Kind], text: it.Title, meta: domain.InboxKinds[it.Kind], muted: it.Status == "reviewed", ref: it.Source, data: it})
	}
	return rows
}

func radarRows(signals []domain.Signal, cfg domain.RadarConfig) []crmRow {
	rows := []crmRow{}
	for _, def := range domain.RadarSignals {
		if cfg.Disabled[def[0]] {
			continue
		}
		n := 0
		for _, s := range signals {
			if s.Kind != def[0] {
				continue
			}
			if n == 0 {
				rows = append(rows, head(def[1]))
			}
			n++
			rows = append(rows, crmRow{kind: "signal", id: def[0] + ":" + s.Person.ID + ":" + s.Date, text: s.Person.Name, meta: s.Date, ref: s.Person, data: s})
		}
	}
	return rows
}

func collectionRows(list []domain.Collection) []crmRow {
	rows := []crmRow{}
	for _, c := range list {
		tag := "    "
		if c.Pinned {
			tag = "pin "
		}
		rows = append(rows, crmRow{kind: "collection", id: c.ID, tag: tag, text: c.Name, meta: c.Entity, data: c})
	}
	return rows
}

func reminderRows(list []domain.Reminder, now time.Time) []crmRow {
	rows := []crmRow{}
	group := ""
	for _, m := range list {
		label := "Upcoming"
		if !m.Due().After(now) {
			label = "Due"
		}
		if label != group {
			group = label
			rows = append(rows, head(label))
		}
		meta := m.Due().Format("2006-01-02 15:04")
		if !m.Recurrence.IsZero() {
			meta = "↻ " + meta
		}
		rows = append(rows, crmRow{kind: "reminder", id: m.ID, tag: "bell", text: m.Title, meta: meta, ref: domain.Ref{Kind: "reminder", ID: m.ID, Name: m.Title}, data: m})
	}
	return rows
}

func followRows(list []domain.FollowUp, today string) []crmRow {
	rows := []crmRow{}
	group := ""
	for _, f := range list {
		label := "Scheduled"
		switch {
		case f.Never:
			label = "Never remind"
		case f.Due != "" && f.Due <= today:
			label = "Due"
		}
		if label != group {
			group = label
			rows = append(rows, head(label))
		}
		meta := f.Interval.String()
		if f.Due != "" {
			meta = strings.TrimSpace(meta + " · " + domain.Relative(f.Due, today))
		}
		rows = append(rows, crmRow{kind: "followup", id: f.Entity.ID, tag: "f/up", text: f.Entity.Name, meta: meta, muted: f.Never, ref: f.Entity, data: f})
	}
	return rows
}

func summaryRows(list []domain.Summary) []crmRow {
	rows := make([]crmRow, 0, len(list))
	for _, c := range list {
		rows = append(rows, crmRow{kind: c.Kind, id: c.ID, tag: refTag(c.Kind), text: c.Name, meta: c.Organization, ref: domain.Ref{Kind: c.Kind, ID: c.ID, Name: c.Name}, data: c})
	}
	return rows
}

func itemRow(it domain.PipelineItem, today string) crmRow {
	meta := ""
	if len(it.StageEnteredAt) >= 10 {
		meta = fmt.Sprintf("%dd", max(0, domain.DaysBetween(it.StageEnteredAt[:10], today)))
	}
	return crmRow{kind: "item", id: it.ID, text: it.Title, meta: meta, ref: domain.Ref{Kind: "item", ID: it.ID, Name: it.Title}, data: it}
}

func stampDay(s string) string {
	if len(s) >= 16 {
		return s[:10] + " " + s[11:16] + " UTC"
	}
	return s
}

func body(rows []crmRow, title, text string) []crmRow {
	if strings.TrimSpace(text) == "" {
		return rows
	}
	rows = append(rows, head(title))
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		rows = append(rows, txt(line))
	}
	return rows
}

func taskDetail(t domain.Task, today string) []crmRow {
	rows := []crmRow{txt(fmt.Sprintf("%s %s · priority %s", taskTags[t.Status], t.Status, domain.PriorityName(t.Priority)))}
	if t.Due != "" {
		line := "Due " + t.Due + " " + t.DueTime
		if t.Open() {
			line += " (" + domain.Relative(t.Due, today) + ")"
		}
		if t.Timezone != "" {
			line += " " + t.Timezone
		}
		rows = append(rows, txt(line))
	}
	if t.Start != "" {
		rows = append(rows, txt("Starts "+t.Start))
	}
	if t.SnoozedUntil > today {
		rows = append(rows, txt("Snoozed until "+t.SnoozedUntil))
	}
	if !t.Recurrence.IsZero() {
		rows = append(rows, txt(fmt.Sprintf("Repeats %s (occurrence %d)", t.Recurrence.String(), t.Occurrence+1)))
	}
	if t.CompletedAt != "" {
		rows = append(rows, txt("Completed "+stampDay(t.CompletedAt)))
	}
	rows = body(rows, "Description", t.Description)
	rows = refLinks(rows, "People and organizations", t.Entities)
	rows = refLinks(rows, "Projects", t.Projects)
	rows = refLinks(rows, "Related activity", t.Activities)
	if len(t.Tags) > 0 {
		rows = append(rows, head("Tags"), txt(strings.Join(t.Tags, ", ")))
	}
	return append(rows, txt(""), dim("Created "+stampDay(t.CreatedAt)+" · updated "+stampDay(t.UpdatedAt)))
}

func activityDetail(a domain.Activity) []crmRow {
	zone := ""
	if a.Timezone != "" && a.Timezone != a.OccurredAt.Location().String() {
		zone = " (recorded in " + a.Timezone + ")"
	}
	rows := []crmRow{txt(fmt.Sprintf("%s · %s%s", a.Type, a.OccurredAt.Format("Mon 2006-01-02 15:04 MST"), zone))}
	switch a.Provenance {
	case domain.Local:
		rows = append(rows, dim("Recorded here · "+a.Visibility))
	case domain.Imported:
		rows = append(rows, txt("Imported from "+a.SourceApp+" · reference "+a.ExternalRef), dim("Read-only: the content belongs to its source."))
	default:
		rows = append(rows, txt("Reference to "+a.SourceApp+" · "+a.ExternalRef), dim("Read-only: the record lives in "+a.SourceApp+"."))
	}
	if a.ExternalURL != "" {
		rows = append(rows, txt("Link: "+a.ExternalURL), dim("o opens the link"))
	}
	rows = body(rows, "Notes", a.Body)
	rows = refLinks(rows, "People and organizations", a.Entities)
	rows = refLinks(rows, "Projects", a.Projects)
	rows = refLinks(rows, "Tasks", a.Tasks)
	if len(a.Tags) > 0 {
		rows = append(rows, head("Tags"), txt(strings.Join(a.Tags, ", ")))
	}
	return append(rows, txt(""), dim("Created "+stampDay(a.CreatedAt)+" · updated "+stampDay(a.UpdatedAt)))
}

func milestoneRows(p domain.Project, today string) []crmRow {
	rows := []crmRow{}
	for _, ms := range p.Milestones {
		tag, meta := "[ ]", ""
		if ms.DoneAt != "" {
			tag, meta = "[x]", "done "+ms.DoneAt
		} else if ms.Due != "" {
			meta = domain.Relative(ms.Due, today)
		}
		rows = append(rows, crmRow{kind: "milestone", id: ms.ID, tag: tag, text: ms.Title, meta: meta, muted: ms.DoneAt != "", data: ms})
	}
	if len(rows) == 0 {
		rows = append(rows, dim("No milestones yet — press M to add one."))
	}
	return rows
}

func meter(done, total, width int) string {
	if total <= 0 {
		return ""
	}
	filled := done * width / total
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + fmt.Sprintf(" %d/%d", done, total)
}

func projectDetail(p domain.Project, sub int, tasks []domain.Task, acts []domain.Activity, today string) []crmRow {
	rows := []crmRow{}
	switch sub {
	case 0:
		rows = append(rows, txt(fmt.Sprintf("%s · priority %s", p.Status, domain.PriorityName(p.Priority))))
		if p.Start != "" || p.Target != "" {
			line := "Start " + orDash(p.Start) + " · target " + orDash(p.Target)
			if p.Target != "" && p.Status != "completed" && p.Status != "archived" {
				line += " (" + domain.Relative(p.Target, today) + ")"
			}
			rows = append(rows, txt(line))
		}
		if total := p.OpenTasks + p.DoneTasks; total > 0 {
			rows = append(rows, txt("Tasks      "+meter(p.DoneTasks, total, 16)))
		}
		done := 0
		for _, ms := range p.Milestones {
			if ms.DoneAt != "" {
				done++
			}
		}
		if len(p.Milestones) > 0 {
			rows = append(rows, txt("Milestones "+meter(done, len(p.Milestones), 16)))
		}
		rows = body(rows, "Description", p.Description)
		rows = refLinks(rows, "People and organizations", p.Members)
		if len(p.Tags) > 0 {
			rows = append(rows, head("Tags"), txt(strings.Join(p.Tags, ", ")))
		}
		if len(acts) > 0 {
			rows = append(rows, head("Latest activity"))
			for _, a := range acts[:min(3, len(acts))] {
				rows = append(rows, lnk(domain.Ref{Kind: "activity", ID: a.ID, Name: a.Title}, a.OccurredAt.Format("01-02")+" "+activityTag(a.Type), a.Title, ""))
			}
		}
		rows = append(rows, txt(""), dim("Created "+stampDay(p.CreatedAt)+" · updated "+stampDay(p.UpdatedAt)))
	case 1:
		for _, r := range p.Members {
			rows = append(rows, lnk(r, refTag(r.Kind), r.Name, r.Role))
		}
		if len(rows) == 0 {
			rows = append(rows, dim("No collaborators yet — press L to link people or organizations."))
		} else {
			rows = append(rows, txt(""), dim("L link · x unlink selected"))
		}
	case 2:
		rows = taskRows(tasks, "group:status", today)
		if len(rows) == 0 {
			rows = append(rows, dim("No tasks yet — press T to add one to this project."))
		}
	case 3:
		for _, a := range acts {
			rows = append(rows, lnk(domain.Ref{Kind: "activity", ID: a.ID, Name: a.Title}, a.OccurredAt.Format("2006-01-02")+" "+activityTag(a.Type), a.Title, refsText(a.Entities)))
		}
		if len(rows) == 0 {
			rows = append(rows, dim("No recorded activity — press A to log one for this project."))
		}
	case 4:
		rows = milestoneRows(p, today)
		rows = append(rows, txt(""), dim("M add · c toggle done · S set date · x remove"))
	case 5:
		rows = body(rows, "Project notes", p.Notes)
		if len(rows) == 0 {
			rows = append(rows, dim("No project notes — press e to write some."))
		}
		for _, a := range acts {
			if a.Type == "note" {
				rows = append(rows, head(a.OccurredAt.Format("2006-01-02")+" · "+a.Title))
				for _, line := range strings.Split(a.Body, "\n") {
					rows = append(rows, txt(line))
				}
			}
		}
	}
	return rows
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func itemDetail(it domain.PipelineItem, today string) []crmRow {
	rows := []crmRow{txt(it.Pipeline + " › " + it.Stage)}
	if len(it.StageEnteredAt) >= 10 {
		rows = append(rows, txt(fmt.Sprintf("In this stage since %s (%d days)", it.StageEnteredAt[:10], max(0, domain.DaysBetween(it.StageEnteredAt[:10], today)))))
	}
	if it.NextAction != "" {
		rows = append(rows, head("Next action"), txt(it.NextAction))
	}
	rows = body(rows, "Notes", it.Notes)
	rows = refLinks(rows, "People and organizations", it.Entities)
	rows = refLinks(rows, "Projects", it.Projects)
	rows = refLinks(rows, "Tasks", it.Tasks)
	rows = refLinks(rows, "Activities", it.Activities)
	if len(it.Tags) > 0 {
		rows = append(rows, head("Tags"), txt(strings.Join(it.Tags, ", ")))
	}
	if len(it.History) > 0 {
		rows = append(rows, head("Stage history"))
		for i := len(it.History) - 1; i >= 0; i-- {
			h := it.History[i]
			from := h.From
			if from == "" {
				from = "created"
			}
			rows = append(rows, txt(fmt.Sprintf("%s %s → %s", h.At[:min(10, len(h.At))], from, h.To)))
		}
	}
	return rows
}

func followLines(f domain.FollowUp, today string) []crmRow {
	rows := []crmRow{head("Follow-up")}
	switch {
	case f.Never:
		rows = append(rows, txt("Never remind"))
	case f.Due != "":
		rows = append(rows, txt("Next: "+f.Due+" ("+domain.Relative(f.Due, today)+")"), txt("Why: "+f.Reason))
	default:
		rows = append(rows, dim("No schedule — press F to set a cadence or a date."))
	}
	if f.LastInteraction != "" {
		rows = append(rows, txt(fmt.Sprintf("Last recorded interaction: %s on %s", f.LastType, f.LastInteraction)))
	} else {
		rows = append(rows, dim("No interaction recorded yet."))
	}
	return rows
}

func dateLines(dates []domain.ImportantDate, today string) []crmRow {
	rows := []crmRow{head("Important dates")}
	for _, d := range dates {
		meta := d.Date
		if d.Next != "" && d.Next >= today {
			meta = d.Next + " · " + domain.Relative(d.Next, today)
		}
		row := crmRow{kind: "date", id: d.ID, tag: "date", text: d.Label, meta: meta, data: d}
		if d.Builtin {
			row = txt(fmt.Sprintf("%s: %s (from the contact record)", d.Label, meta))
		}
		rows = append(rows, row)
	}
	if len(dates) == 0 {
		rows = append(rows, dim("None — press d to add one."))
	}
	return rows
}

// treeRows draws the hierarchical relationship list with Unicode line drawing.
func treeRows(c domain.Connections) []crmRow {
	rows := []crmRow{txt(c.Root.Name)}
	type node struct {
		row      crmRow
		children []crmRow
	}
	nodes := []node{}
	for _, e := range c.Outgoing {
		n := node{row: lnk(e.Person, "", e.Type+" → "+e.Person.Name, e.Notes)}
		for _, ch := range e.Children {
			n.children = append(n.children, lnk(ch.Person, "", ch.Type+" → "+ch.Person.Name, ""))
		}
		nodes = append(nodes, n)
	}
	for _, e := range c.Incoming {
		n := node{row: lnk(e.Person, "", e.Type+" ← "+e.Person.Name, e.Notes)}
		for _, ch := range e.Children {
			n.children = append(n.children, lnk(ch.Person, "", ch.Type+" → "+ch.Person.Name, ""))
		}
		nodes = append(nodes, n)
	}
	for _, o := range c.Organizations {
		label := "member of " + o.Organization.Name
		if o.Organization.ID == c.Root.ID {
			label = "members"
		}
		n := node{row: lnk(o.Organization, "", label, o.Title)}
		for _, p := range o.People {
			n.children = append(n.children, lnk(p, "", p.Name, p.Role))
		}
		nodes = append(nodes, n)
	}
	for _, p := range c.Projects {
		n := node{row: lnk(p.Project, "", "project "+p.Project.Name, p.Status)}
		for _, who := range p.People {
			n.children = append(n.children, lnk(who, "", who.Name, who.Role))
		}
		nodes = append(nodes, n)
	}
	for i, n := range nodes {
		branch, trunk := "├─ ", "│  "
		if i == len(nodes)-1 {
			branch, trunk = "└─ ", "   "
		}
		n.row.tag = branch
		rows = append(rows, n.row)
		for j, ch := range n.children {
			ch.tag = trunk + "├─ "
			if j == len(n.children)-1 {
				ch.tag = trunk + "└─ "
			}
			rows = append(rows, ch)
		}
	}
	if len(nodes) == 0 {
		rows = append(rows, dim("No connections recorded — press r to add a relationship."))
	}
	return rows
}

func connectionsDetail(c domain.Connections, sub int, today string) []crmRow {
	rows := []crmRow{}
	switch sub {
	case 0:
		rows = treeRows(c)
	case 1:
		type reason struct {
			ref  domain.Ref
			why  []string
			rank int
		}
		byID := map[string]*reason{}
		order := []string{}
		add := func(ref domain.Ref, why string, rank int) {
			if ref.Kind != "person" || ref.ID == c.Root.ID {
				return
			}
			if byID[ref.ID] == nil {
				byID[ref.ID] = &reason{ref: ref}
				order = append(order, ref.ID)
			}
			byID[ref.ID].why = append(byID[ref.ID].why, why)
			byID[ref.ID].rank += rank
		}
		for _, e := range c.Outgoing {
			add(e.Person, e.Type, 4)
		}
		for _, e := range c.Incoming {
			add(e.Person, e.Type+" (theirs)", 4)
		}
		for _, o := range c.Organizations {
			for _, p := range o.People {
				add(p, "at "+o.Organization.Name, 2)
			}
		}
		for _, p := range c.Projects {
			for _, who := range p.People {
				add(who, "on "+p.Project.Name, 2)
			}
		}
		for _, e := range c.Shared {
			add(e.Person, fmt.Sprintf("%d shared activities", e.Count), 1)
		}
		sort.SliceStable(order, func(i, j int) bool { return byID[order[i]].rank > byID[order[j]].rank })
		for _, id := range order {
			rows = append(rows, lnk(byID[id].ref, "who ", byID[id].ref.Name, strings.Join(byID[id].why, " · ")))
		}
		if len(rows) == 0 {
			rows = append(rows, dim("No connected people recorded."))
		}
	case 2:
		for _, o := range c.Organizations {
			status := "past"
			if o.Current {
				status = "current"
			}
			rows = append(rows, lnk(o.Organization, "org ", o.Organization.Name, strings.TrimSpace(o.Title+" · "+status)))
			for _, p := range o.People {
				rows = append(rows, lnk(p, "  └ ", p.Name, p.Role))
			}
		}
		if len(rows) == 0 {
			rows = append(rows, dim("No organization connections."))
		}
	case 3:
		for _, p := range c.Projects {
			rows = append(rows, lnk(p.Project, "proj", p.Project.Name, strings.TrimSpace(p.Project.Role+" · "+p.Status)))
			for _, who := range p.People {
				rows = append(rows, lnk(who, "  └ ", who.Name, who.Role))
			}
		}
		if len(rows) == 0 {
			rows = append(rows, dim("No shared projects."))
		}
	default:
		if c.Flagged {
			rows = append(rows, txt("⚑ Flagged for review "+c.FlagNote))
		}
		rows = append(rows, head("Relationships"))
		for _, e := range c.Outgoing {
			rows = append(rows, crmRow{kind: "edge", id: e.Person.ID + "|" + e.Type, tag: "→   ", text: e.Type + " · " + e.Person.Name, meta: e.Notes, ref: e.Person, data: e})
		}
		for _, e := range c.Incoming {
			rows = append(rows, lnk(e.Person, "←   ", e.Type+" · "+e.Person.Name, e.Notes))
		}
		if len(c.Outgoing)+len(c.Incoming) == 0 {
			rows = append(rows, dim("None — press r to add one."))
		}
		rows = body(rows, "Relationship notes", c.Notes)
		rows = append(rows, followLines(c.FollowUp, today)...)
		rows = append(rows, dateLines(c.Dates, today)...)
		rows = append(rows, head("Relationship history"))
		for _, a := range c.Recent {
			rows = append(rows, lnk(domain.Ref{Kind: "activity", ID: a.ID, Name: a.Title}, a.OccurredAt.Format("2006-01-02")+" "+activityTag(a.Type), a.Title, ""))
		}
		if len(c.Recent) == 0 {
			rows = append(rows, dim("No recorded activity."))
		}
	}
	return rows
}

// dossierRows renders the contact profile's CRM sections.
func dossierRows(d domain.Dossier, timeline bool, today string) []crmRow {
	rows := []crmRow{}
	if timeline {
		rows = append(rows, dim(fmt.Sprintf("%d recorded activities · showing the latest %d · A logs one", d.Activities, len(d.Recent))))
		day := ""
		for _, a := range d.Recent {
			if x := domain.Day(a.OccurredAt); x != day {
				day = x
				rows = append(rows, head(dayLabel(x, today)))
			}
			rows = append(rows, lnk(domain.Ref{Kind: "activity", ID: a.ID, Name: a.Title}, a.OccurredAt.Format("15:04")+" "+activityTag(a.Type), a.Title, provenanceMark(a)))
		}
		if len(d.Recent) == 0 {
			rows = append(rows, dim("Nothing recorded yet. Activities are only ever what you log or import."))
		} else if d.Activities > len(d.Recent) {
			rows = append(rows, txt(""), lnk(domain.Ref{Kind: "timeline", ID: d.Root.ID, Name: d.Root.Name}, "", fmt.Sprintf("Open the full timeline (%d activities)…", d.Activities), ""))
		}
		return rows
	}
	if d.Flagged {
		rows = append(rows, txt("⚑ Flagged for review "+d.FlagNote))
	}
	rows = append(rows, followLines(d.FollowUp, today)...)
	rows = append(rows, head(fmt.Sprintf("Open tasks (%d)", d.OpenTaskCount)))
	for _, t := range d.OpenTasks {
		row := taskRow(t, today)
		row.text = t.Title
		rows = append(rows, row)
	}
	if len(d.OpenTasks) == 0 {
		rows = append(rows, dim("None — press T to add one."))
	}
	if len(d.Commitments) > 0 {
		rows = append(rows, head("Recent commitments"))
		for _, t := range d.Commitments {
			row := taskRow(t, today)
			row.text = t.Title
			rows = append(rows, row)
		}
	}
	if len(d.Projects) > 0 {
		rows = append(rows, head("Projects"))
		for _, p := range d.Projects {
			rows = append(rows, lnk(p.Project, "proj", p.Project.Name, strings.TrimSpace(p.Project.Role+" "+p.Status)))
		}
	}
	rows = append(rows, head("Connections"))
	n := 0
	for _, e := range append(append([]domain.Edge{}, d.Outgoing...), d.Incoming...) {
		rows = append(rows, lnk(e.Person, refTag(e.Person.Kind), e.Person.Name, e.Type))
		n++
	}
	for _, o := range d.Organizations {
		if o.Organization.ID != d.Root.ID {
			rows = append(rows, lnk(o.Organization, "org ", o.Organization.Name, o.Title))
			n++
		}
	}
	if n == 0 {
		rows = append(rows, dim("None recorded."))
	}
	rows = append(rows, dateLines(d.Dates, today)...)
	return rows
}
