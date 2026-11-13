package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Ref is a link to another record. Writers may supply an ID or an exact unique Name.
type Ref struct {
	Kind string `json:"kind,omitempty"` // person, organization, project, task, activity, item, pipeline, reminder, collection
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Role string `json:"role,omitempty"`
}

func RefNames(refs []Ref) string {
	names := make([]string, len(refs))
	for i, r := range refs {
		names[i] = r.Name
	}
	return strings.Join(names, ", ")
}

// Provenance distinguishes who owns a record.
const (
	Local    = "local"    // created in TideContacts; fully editable
	Imported = "imported" // copied in from a file or tool; retained with its source identifiers
	External = "external" // a reference to a record owned by another application
)

// ActivityTypes are built in; any other non-empty lower-case label is a custom type.
var ActivityTypes = []string{"email", "call", "meeting", "conversation", "message", "note", "event", "task-completion", "project-update"}

// InteractionTypes count as contact with a person when computing follow-ups.
var InteractionTypes = []string{"email", "call", "meeting", "conversation", "message", "event"}

func IsInteraction(typ string) bool {
	for _, t := range InteractionTypes {
		if t == typ {
			return true
		}
	}
	// Custom types are interactions; notes and system entries are not.
	return typ != "note" && typ != "task-completion" && typ != "project-update"
}

type Activity struct {
	ID, Type, Title, Body              string
	OccurredAt                         time.Time
	Timezone                           string // IANA zone the activity was recorded in
	Provenance, SourceApp, ExternalRef string
	ExternalURL, Visibility            string
	Entities, Projects, Tasks          []Ref
	Tags                               []string
	Deleted                            bool
	CreatedAt, UpdatedAt               string
	Revision                           int
}

func (a *Activity) Normalize(now time.Time) {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	a.Type = strings.ToLower(strings.TrimSpace(a.Type))
	if a.Type == "" {
		a.Type = "note"
	}
	a.Title = strings.TrimSpace(a.Title)
	if a.Title == "" {
		line, _, _ := strings.Cut(strings.TrimSpace(a.Body), "\n")
		a.Title = line
	}
	if a.Provenance == "" {
		a.Provenance = Local
	}
	if a.Visibility == "" {
		a.Visibility = "private"
	}
	if a.OccurredAt.IsZero() {
		a.OccurredAt = now
	}
	if a.Timezone == "" {
		a.Timezone = a.OccurredAt.Location().String()
	}
	a.Tags = cleanTags(a.Tags)
}

func (a Activity) Validate() error {
	if a.Title == "" {
		return fmt.Errorf("an activity needs a title or text")
	}
	if strings.ContainsAny(a.Type, " \t\n:\"") {
		return fmt.Errorf("activity type must be a single word")
	}
	if a.Provenance != Local && a.Provenance != Imported && a.Provenance != External {
		return fmt.Errorf("provenance must be local, imported or external")
	}
	if a.Provenance != Local && (a.SourceApp == "" || a.ExternalRef == "") {
		return fmt.Errorf("imported and external activities need a source application and reference")
	}
	if a.Visibility != "private" && a.Visibility != "shared" {
		return fmt.Errorf("visibility must be private or shared")
	}
	return nil
}

func cleanTags(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t != "" && !seen[strings.ToLower(t)] {
			seen[strings.ToLower(t)] = true
			out = append(out, t)
		}
	}
	return out
}

var TaskStatuses = []string{"inbox", "next", "scheduled", "waiting", "completed", "canceled"}
var Priorities = []string{"none", "low", "medium", "high"}

func PriorityName(p int) string { return Priorities[min(max(p, 0), 3)] }

func ParsePriority(s string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "none", "0":
		return 0, nil
	case "low", "l", "1":
		return 1, nil
	case "medium", "med", "m", "2":
		return 2, nil
	case "high", "h", "3", "!":
		return 3, nil
	}
	return 0, fmt.Errorf("priority must be none, low, medium or high")
}

type Task struct {
	ID, Title, Description, Status string
	Priority                       int
	Due, DueTime, Start            string // YYYY-MM-DD, HH:MM, YYYY-MM-DD
	Timezone                       string // IANA zone for DueTime; empty floats with the local zone
	SnoozedUntil, CompletedAt      string
	Recurrence                     Recurrence
	SeriesID                       string
	Occurrence                     int
	Entities, Projects, Activities []Ref
	Tags                           []string
	Archived, Deleted              bool
	CreatedAt, UpdatedAt           string
	Revision                       int
}

func (t Task) Open() bool { return t.Status != "completed" && t.Status != "canceled" }

func (t *Task) Normalize() {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	t.Title = strings.TrimSpace(t.Title)
	if t.Status == "" {
		t.Status = "inbox"
	}
	if !t.Recurrence.IsZero() {
		if t.SeriesID == "" {
			t.SeriesID = uuid.NewString()
		}
		if t.Recurrence.Anchor == "" {
			t.Recurrence.Anchor = t.Due
		}
	} else if t.Occurrence == 0 {
		t.SeriesID = ""
	}
	t.Tags = cleanTags(t.Tags)
}

func (t Task) Validate() error {
	if t.Title == "" {
		return fmt.Errorf("a task needs a title")
	}
	ok := false
	for _, s := range TaskStatuses {
		ok = ok || s == t.Status
	}
	if !ok {
		return fmt.Errorf("status must be one of %s", strings.Join(TaskStatuses, ", "))
	}
	if t.Priority < 0 || t.Priority > 3 {
		return fmt.Errorf("priority out of range")
	}
	for _, d := range []string{t.Due, t.Start, t.SnoozedUntil} {
		if _, ok := ParseDay(d); d != "" && !ok {
			return fmt.Errorf("dates must be YYYY-MM-DD")
		}
	}
	if !ClockValid(t.DueTime) {
		return fmt.Errorf("due time must be HH:MM")
	}
	if t.DueTime != "" && t.Due == "" {
		return fmt.Errorf("a due time needs a due date")
	}
	if !t.Recurrence.IsZero() && t.Due == "" && !t.Recurrence.AfterCompletion {
		return fmt.Errorf("a recurring task needs a due date, or use \"after completion\"")
	}
	if t.Timezone != "" {
		if _, e := time.LoadLocation(t.Timezone); e != nil {
			return fmt.Errorf("unknown time zone %q", t.Timezone)
		}
	}
	return nil
}

type Reminder struct {
	ID, Title, Note      string
	At                   time.Time
	Timezone             string
	Recurrence           Recurrence
	Task, Entity         Ref
	Status               string // pending, dismissed
	SnoozedUntil         time.Time
	CreatedAt, UpdatedAt string
}

// Due is when the reminder should next be shown.
func (r Reminder) Due() time.Time {
	if r.SnoozedUntil.After(r.At) {
		return r.SnoozedUntil
	}
	return r.At
}

type FollowUp struct {
	Entity          Ref
	Interval        Interval
	Never           bool
	NextDate        string // an explicit one-off follow-up date
	SnoozedUntil    string
	SetAt           string
	LastInteraction string // calendar date of the latest recorded interaction
	LastType        string
	Due             string // computed
	Reason          string // computed, always present when Due is set
}

// Evaluate fills Due and Reason from recorded facts only. With no interval and no
// explicit date nothing is due: missing history is never treated as neglect.
func (f *FollowUp) Evaluate(today string) {
	f.Due, f.Reason = "", ""
	if f.Never {
		return
	}
	if f.NextDate != "" {
		f.Due = f.NextDate
		f.Reason = "Follow-up date you set: " + f.NextDate
	}
	if !f.Interval.IsZero() {
		base, why := f.LastInteraction, ""
		if base != "" {
			why = fmt.Sprintf("%s cadence; last recorded %s on %s (%d days ago)", capitalize(f.Interval.String()), f.LastType, base, DaysBetween(base, today))
		} else {
			base = f.SetAt
			why = fmt.Sprintf("%s cadence set on %s; no interaction recorded since", capitalize(f.Interval.String()), base)
		}
		due := AddInterval(base, f.Interval.Every, f.Interval.Unit)
		if f.Due == "" || due < f.Due {
			f.Due, f.Reason = due, why
		}
	}
	if f.Due != "" && f.SnoozedUntil > f.Due {
		f.Reason += "; snoozed until " + f.SnoozedUntil
		f.Due = f.SnoozedUntil
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

type ImportantDate struct {
	ID, Label, Date, Notes string // Date is YYYY-MM-DD or yearless --MM-DD
	Entity                 Ref
	Annual                 bool
	Builtin                bool   // birthday or anniversary stored on the contact
	Next                   string // computed next occurrence
}

var ProjectStatuses = []string{"planned", "active", "on-hold", "completed", "archived"}

type Project struct {
	ID, Name, Description, Status, Notes string
	Priority                             int
	Start, Target                        string
	Members                              []Ref
	Tags                                 []string
	Milestones                           []Milestone
	OpenTasks, DoneTasks                 int
	Deleted                              bool
	CreatedAt, UpdatedAt                 string
	Revision                             int
}

type Milestone struct {
	ID, ProjectID, Title, Due, DoneAt string
	Position                          int
}

func (p *Project) Normalize() {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Status == "" {
		p.Status = "active"
	}
	p.Tags = cleanTags(p.Tags)
	for i := range p.Milestones {
		if p.Milestones[i].ID == "" {
			p.Milestones[i].ID = uuid.NewString()
		}
	}
}

func (p Project) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("a project needs a name")
	}
	ok := false
	for _, s := range ProjectStatuses {
		ok = ok || s == p.Status
	}
	if !ok {
		return fmt.Errorf("status must be one of %s", strings.Join(ProjectStatuses, ", "))
	}
	for _, d := range []string{p.Start, p.Target} {
		if _, ok := ParseDay(d); d != "" && !ok {
			return fmt.Errorf("project dates must be YYYY-MM-DD")
		}
	}
	if p.Start != "" && p.Target != "" && p.Target < p.Start {
		return fmt.Errorf("target date precedes start date")
	}
	for _, m := range p.Milestones {
		if strings.TrimSpace(m.Title) == "" {
			return fmt.Errorf("a milestone needs a title")
		}
		if _, ok := ParseDay(m.Due); m.Due != "" && !ok {
			return fmt.Errorf("milestone dates must be YYYY-MM-DD")
		}
	}
	return nil
}

type Pipeline struct {
	ID, Name, Description string
	Archived              bool
	Position              int
	Stages                []Stage
}

type Stage struct {
	ID, PipelineID, Name string
	Position, Count      int
}

type PipelineItem struct {
	ID, PipelineID, StageID, Stage, Pipeline string
	Title, Notes, NextAction                 string
	StageEnteredAt                           string
	Entities, Projects, Tasks, Activities    []Ref
	Tags                                     []string
	History                                  []StageChange
	Archived, Deleted                        bool
	CreatedAt, UpdatedAt                     string
	Revision                                 int
}

type StageChange struct{ From, To, At string }

// InboxItem is one actionable entry in the People Inbox.
type InboxItem struct {
	Key, Kind, Title, Reason string
	Source                   Ref
	Person                   Ref // the person to act on, when there is one
	Due                      string
	Urgency                  int // 3 overdue, 2 due now, 1 upcoming, 0 informational
	FirstSeen, Status        string
	Fingerprint              string
}

var InboxKinds = map[string]string{
	"reminder": "Reminder due", "task-overdue": "Overdue commitment", "followup-due": "Follow-up due",
	"followup-suggested": "Suggested follow-up", "task-waiting": "Waiting on someone", "task-inbox": "Unprocessed task",
	"date": "Important date", "flagged": "Flagged for review",
}

// RadarConfig holds every threshold and switch of Relationship Radar.
type RadarConfig struct {
	Disabled      map[string]bool `json:"disabled,omitempty"`
	UpcomingDays  int             `json:"upcoming_days"`  // important-date window
	ActiveDays    int             `json:"active_days"`    // "recently active" window
	ActiveMinimum int             `json:"active_minimum"` // interactions needed inside the window
	LeadDays      int             `json:"lead_days"`      // show follow-ups this many days early
	WaitingDays   int             `json:"waiting_days"`   // waiting tasks surface after this many days
	Limit         int             `json:"limit"`          // rows per signal
}

func DefaultRadar() RadarConfig {
	return RadarConfig{Disabled: map[string]bool{}, UpcomingDays: 14, ActiveDays: 30, ActiveMinimum: 2, LeadDays: 0, WaitingDays: 7, Limit: 25}
}

func (c *RadarConfig) Normalize() {
	d := DefaultRadar()
	if c.Disabled == nil {
		c.Disabled = map[string]bool{}
	}
	clamp := func(v *int, def, lo, hi int) {
		if *v < lo || *v > hi {
			*v = def
		}
	}
	clamp(&c.UpcomingDays, d.UpcomingDays, 1, 366)
	clamp(&c.ActiveDays, d.ActiveDays, 1, 3650)
	clamp(&c.ActiveMinimum, d.ActiveMinimum, 1, 1000)
	clamp(&c.LeadDays, d.LeadDays, 0, 365)
	clamp(&c.WaitingDays, d.WaitingDays, 0, 3650)
	clamp(&c.Limit, d.Limit, 1, 500)
}

// RadarSignals lists signal IDs and what each one states. None of them is a score.
var RadarSignals = [][2]string{
	{"followup", "Due for follow-up"}, {"commitments", "Open commitments"}, {"dates", "Upcoming important dates"},
	{"cadence", "Configured contact intervals"}, {"active", "Recently active collaborators"}, {"overdue", "Linked to overdue work"},
}

type Signal struct {
	Kind, Reason, Date string
	Person             Ref
}

var CollectionEntities = []string{"person", "organization", "task", "activity", "project", "item"}

type Collection struct {
	ID, Name, Description, Entity, Query, Sort, Group string
	Pinned                                            bool
	CreatedAt, UpdatedAt                              string
}

func (c *Collection) Normalize() {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	c.Name = strings.TrimSpace(c.Name)
}

func (c Collection) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("a collection needs a name")
	}
	for _, e := range CollectionEntities {
		if e == c.Entity {
			return nil
		}
	}
	return fmt.Errorf("collection type must be one of %s", strings.Join(CollectionEntities, ", "))
}

// Connections is the relationship-explorer projection for one contact.
type Connections struct {
	Root          Ref
	Notes         string
	Flagged       bool
	FlagNote      string
	Outgoing      []Edge
	Incoming      []Edge
	Organizations []OrgLink
	Projects      []ProjectLink
	Shared        []Edge // people who share recorded activities
	Dates         []ImportantDate
	FollowUp      FollowUp
	Recent        []Activity
}

type Edge struct {
	Person      Ref
	Type, Notes string
	Count       int
	Children    []Edge // second-level outgoing relationships
}

type OrgLink struct {
	Organization Ref
	Title        string
	Current      bool
	People       []Ref
}

type ProjectLink struct {
	Project Ref
	Status  string
	People  []Ref
}

// Dossier is the CRM context shown on a contact profile. Every list is bounded.
type Dossier struct {
	Connections
	OpenTasks     []Task
	Commitments   []Task // waiting on this person, or completed recently
	Activities    int
	OpenTaskCount int
}
