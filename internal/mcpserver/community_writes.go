package mcpserver

import (
	"context"
	"database/sql"
	"encoding/hex"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/allisonhere/tidecontacts/internal/domain"
)

type communityWrite struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
}

type communityRevision struct {
	ID       string `json:"id" jsonschema:"UUID of the live record"`
	Revision int    `json:"revision" jsonschema:"Positive revision returned when the record was last read"`
}

func (r communityRevision) validate() error {
	if err := communityValidID(r.ID); err != nil {
		return err
	}
	if r.Revision <= 0 {
		return problem("invalid_input", "a positive expected revision is required")
	}
	return nil
}

func communityMatchRevision(current, expected int) error {
	if current != expected {
		return problem("conflict", "record changed; reload before retrying")
	}
	return nil
}

func communityPatchText(dst *string, src *string, limit int, required bool) error {
	if src == nil {
		return nil
	}
	value, err := communityText(*src, limit, required)
	if err != nil {
		return err
	}
	*dst = value
	return nil
}

func communityPatchDay(dst *string, src *string) error {
	if src == nil {
		return nil
	}
	if *src != "" {
		if _, ok := domain.ParseDay(*src); !ok || len(*src) != 10 {
			return problem("invalid_input", "date must be YYYY-MM-DD or empty to clear it")
		}
	}
	*dst = *src
	return nil
}

func communityPatchTags(dst *[]string, src []string) error {
	if src == nil {
		return nil
	}
	if len(src) > communityTags {
		return problem("invalid_input", "at most 16 tags are allowed")
	}
	for i, tag := range src {
		tag, err := communityText(tag, communityLabel, true)
		if err != nil {
			return err
		}
		for _, earlier := range src[:i] {
			if strings.EqualFold(tag, earlier) {
				return problem("invalid_input", "tags must be distinct")
			}
		}
		src[i] = tag
	}
	*dst = src
	return nil
}

func communityPatchPriority(dst *int, src *int) error {
	if src == nil {
		return nil
	}
	if *src < 0 || *src > 3 {
		return problem("invalid_input", "priority must be between 0 and 3")
	}
	*dst = *src
	return nil
}

func communityAddLink(dst *[]domain.Ref, ref domain.Ref) error {
	if slices.ContainsFunc(*dst, func(r domain.Ref) bool { return r.ID == ref.ID }) {
		return problem("invalid_input", "record is already linked")
	}
	*dst = append(*dst, ref)
	return nil
}

func (s *Server) communityLinkEntity(ctx context.Context, dst *[]domain.Ref, entity *communityEntity) error {
	if entity == nil {
		return nil
	}
	kind, err := s.communityCheckEntity(ctx, *entity)
	if err != nil {
		return err
	}
	return communityAddLink(dst, domain.Ref{ID: entity.ID, Kind: kind})
}

func (s *Server) communityLinkProject(ctx context.Context, dst *[]domain.Ref, id *string) error {
	if id == nil {
		return nil
	}
	if err := s.require("projects.read"); err != nil {
		return err
	}
	if _, err := s.communityReadProject(ctx, *id); err != nil {
		return err
	}
	return communityAddLink(dst, domain.Ref{ID: *id, Kind: "project"})
}

func (s *Server) communityLinkTask(ctx context.Context, dst *[]domain.Ref, id *string) error {
	if id == nil {
		return nil
	}
	if err := s.require("tasks.read"); err != nil {
		return err
	}
	if _, err := s.communityReadTask(ctx, *id); err != nil {
		return err
	}
	return communityAddLink(dst, domain.Ref{ID: *id, Kind: "task"})
}

type communityContactFields struct {
	Given          *string  `json:"given_name,omitempty" jsonschema:"New given name; empty clears it"`
	Family         *string  `json:"family_name,omitempty" jsonschema:"New family name; empty clears it"`
	Tags           []string `json:"tags,omitempty" jsonschema:"Replace tags with up to 16 labels, or [] to clear them"`
	OrganizationID *string  `json:"organization_id,omitempty" jsonschema:"UUID of an existing organization to add as a current membership"`
}

type communityContactCreate struct {
	Name  string `json:"name" jsonschema:"Display name, at most 160 bytes"`
	Email string `json:"email,omitempty" jsonschema:"Optional plain email address, without a display name"`
	communityContactFields
}

type communityContactUpdate struct {
	communityRevision
	Name *string `json:"name,omitempty" jsonschema:"New nonempty display name, at most 160 bytes"`
	communityContactFields
}

func (f communityContactFields) present() bool {
	return f.Given != nil || f.Family != nil || f.Tags != nil || f.OrganizationID != nil
}

func (s *Server) communityContactFields(ctx context.Context, c *domain.Contact, f communityContactFields) error {
	if err := communityPatchText(&c.Given, f.Given, communityLabel, false); err != nil {
		return err
	}
	if err := communityPatchText(&c.Family, f.Family, communityLabel, false); err != nil {
		return err
	}
	if err := communityPatchTags(&c.Tags, f.Tags); err != nil {
		return err
	}
	if f.OrganizationID != nil {
		org := communityEntity{ID: *f.OrganizationID, Kind: "organization"}
		if _, err := s.communityCheckEntity(ctx, org); err != nil {
			return err
		}
		if slices.ContainsFunc(c.Memberships, func(m domain.Membership) bool { return m.OrganizationID == org.ID }) {
			return problem("invalid_input", "contact already has this organization membership")
		}
		c.Memberships = append(c.Memberships, domain.Membership{OrganizationID: org.ID, Current: true})
	}
	return nil
}

func (s *Server) communitySaveContact(ctx context.Context, c domain.Contact) (communityWrite, error) {
	c.Normalize()
	if c.Validate() != nil {
		return communityWrite{}, problem("invalid_input", "contact fields are invalid")
	}
	batch, err := s.repo.SaveContacts(ctx, []domain.Contact{c}, "MCP contact save")
	if err != nil {
		return communityWrite{}, communityError(err)
	}
	if len(batch) != 1 {
		return communityWrite{}, problem("runtime", "contact save did not return its saved revision")
	}
	return communityWrite{ID: batch[0].ID, Revision: batch[0].Revision}, nil
}

func (s *Server) communityCreateContact(ctx context.Context, in communityContactCreate) (communityWrite, error) {
	name, err := communityText(in.Name, communityTitle, true)
	if err != nil {
		return communityWrite{}, err
	}
	c := domain.NewContact("person")
	c.DisplayName = name
	if in.Email != "" {
		email, err := communityText(in.Email, 254, true)
		if err != nil {
			return communityWrite{}, err
		}
		address, err := mail.ParseAddress(email)
		if err != nil || address.Address != email {
			return communityWrite{}, problem("invalid_input", "email must be a plain address")
		}
		c.Methods = []domain.Method{{Kind: "email", Value: email, Preferred: true}}
	}
	if err := s.communityContactFields(ctx, &c, in.communityContactFields); err != nil {
		return communityWrite{}, err
	}
	return s.communitySaveContact(ctx, c)
}

func (s *Server) communityUpdateContact(ctx context.Context, in communityContactUpdate) (communityWrite, error) {
	if err := in.validate(); err != nil {
		return communityWrite{}, err
	}
	if in.Name == nil && !in.communityContactFields.present() {
		return communityWrite{}, problem("invalid_input", "provide at least one contact field to update")
	}
	c, err := s.communityReadContact(ctx, in.ID, "person")
	if err != nil {
		return communityWrite{}, err
	}
	if c.DirectoryReadOnly() {
		return communityWrite{}, problem("read_only", "directory contacts are read-only")
	}
	if err := communityMatchRevision(c.Revision, in.Revision); err != nil {
		return communityWrite{}, err
	}
	if err := communityPatchText(&c.DisplayName, in.Name, communityTitle, true); err != nil {
		return communityWrite{}, err
	}
	if err := s.communityContactFields(ctx, &c, in.communityContactFields); err != nil {
		return communityWrite{}, err
	}
	return s.communitySaveContact(ctx, c)
}

type communityActivityFields struct {
	Type       *string          `json:"type,omitempty" jsonschema:"email, call, meeting, conversation, message, note or event; default note"`
	OccurredAt *string          `json:"occurred_at,omitempty" jsonschema:"RFC 3339 instant with an offset; default is now when creating"`
	Tags       []string         `json:"tags,omitempty" jsonschema:"Replace tags with up to 16 labels, or [] to clear them"`
	Entity     *communityEntity `json:"entity,omitempty" jsonschema:"Existing contact or organization to add as a link"`
	ProjectID  *string          `json:"project_id,omitempty" jsonschema:"UUID of an existing project to add as a link"`
}

type communityActivityCreate struct {
	Title  string  `json:"title" jsonschema:"Explicit nonempty title, at most 160 bytes"`
	TaskID *string `json:"task_id,omitempty" jsonschema:"UUID of an existing task to add as a link"`
	communityActivityFields
}

type communityActivityUpdate struct {
	communityRevision
	Title *string `json:"title,omitempty" jsonschema:"New nonempty title, at most 160 bytes"`
	communityActivityFields
}

func (f communityActivityFields) present() bool {
	return f.Type != nil || f.OccurredAt != nil || f.Tags != nil || f.Entity != nil || f.ProjectID != nil
}

func (s *Server) communityActivityFields(ctx context.Context, a *domain.Activity, f communityActivityFields) error {
	if f.Type != nil {
		if *f.Type != "note" && !slices.Contains(domain.InteractionTypes, *f.Type) {
			return problem("invalid_input", "unsupported activity type")
		}
		a.Type = *f.Type
	}
	if f.OccurredAt != nil {
		if len(*f.OccurredAt) > 40 {
			return problem("invalid_input", "activity time must be an RFC 3339 instant")
		}
		when, err := time.Parse(time.RFC3339, *f.OccurredAt)
		if err != nil || when.IsZero() {
			return problem("invalid_input", "activity time must be an RFC 3339 instant")
		}
		a.OccurredAt, a.Timezone = when.UTC(), "UTC"
	}
	if err := communityPatchTags(&a.Tags, f.Tags); err != nil {
		return err
	}
	if err := s.communityLinkEntity(ctx, &a.Entities, f.Entity); err != nil {
		return err
	}
	return s.communityLinkProject(ctx, &a.Projects, f.ProjectID)
}

func (s *Server) communitySaveActivity(ctx context.Context, a domain.Activity) (communityWrite, error) {
	if a.Validate() != nil {
		return communityWrite{}, problem("invalid_input", "activity fields are invalid")
	}
	saved, err := s.repo.SaveActivity(ctx, a)
	if err != nil {
		return communityWrite{}, communityError(err)
	}
	return communityWrite{ID: saved.ID, Revision: saved.Revision}, nil
}

func (s *Server) communityCreateActivity(ctx context.Context, in communityActivityCreate) (communityWrite, error) {
	title, err := communityText(in.Title, communityTitle, true)
	if err != nil {
		return communityWrite{}, err
	}
	a := domain.Activity{Title: title, Type: "note", Provenance: domain.Local, Visibility: "private"}
	if err := s.communityActivityFields(ctx, &a, in.communityActivityFields); err != nil {
		return communityWrite{}, err
	}
	if err := s.communityLinkTask(ctx, &a.Tasks, in.TaskID); err != nil {
		return communityWrite{}, err
	}
	return s.communitySaveActivity(ctx, a)
}

func (s *Server) communityUpdateActivity(ctx context.Context, in communityActivityUpdate) (communityWrite, error) {
	if err := in.validate(); err != nil {
		return communityWrite{}, err
	}
	if in.Title == nil && !in.communityActivityFields.present() {
		return communityWrite{}, problem("invalid_input", "provide at least one activity field to update")
	}
	a, err := s.repo.Activity(ctx, in.ID)
	if err == nil && a.Deleted {
		err = sql.ErrNoRows
	}
	if err != nil {
		return communityWrite{}, communityError(err)
	}
	if a.Provenance != domain.Local || a.Type == "task-completion" || a.Type == "project-update" {
		return communityWrite{}, problem("read_only", "source-owned and generated activities are read-only")
	}
	if err := communityMatchRevision(a.Revision, in.Revision); err != nil {
		return communityWrite{}, err
	}
	if err := communityPatchText(&a.Title, in.Title, communityTitle, true); err != nil {
		return communityWrite{}, err
	}
	if err := s.communityActivityFields(ctx, &a, in.communityActivityFields); err != nil {
		return communityWrite{}, err
	}
	if a.Validate() != nil {
		return communityWrite{}, problem("invalid_input", "activity fields are invalid")
	}
	store, ok := s.repo.(interface {
		SaveActivityPreservingTaskLinks(context.Context, domain.Activity) (int, error)
	})
	if !ok {
		return communityWrite{}, problem("unavailable", "link-preserving activity updates are unavailable")
	}
	revision, err := store.SaveActivityPreservingTaskLinks(ctx, a)
	if err != nil {
		return communityWrite{}, communityError(err)
	}
	return communityWrite{ID: a.ID, Revision: revision}, nil
}

type communityTaskFields struct {
	Status    *string          `json:"status,omitempty" jsonschema:"inbox, next, scheduled or waiting"`
	Priority  *int             `json:"priority,omitempty" jsonschema:"0 none, 1 low, 2 medium, 3 high"`
	Due       *string          `json:"due,omitempty" jsonschema:"Due date YYYY-MM-DD; empty clears it"`
	DueTime   *string          `json:"due_time,omitempty" jsonschema:"Local due time HH:MM; empty clears it"`
	Start     *string          `json:"start,omitempty" jsonschema:"Start date YYYY-MM-DD; empty clears it"`
	Tags      []string         `json:"tags,omitempty" jsonschema:"Replace tags with up to 16 labels, or [] to clear them"`
	Entity    *communityEntity `json:"entity,omitempty" jsonschema:"Existing contact or organization to add as a link"`
	ProjectID *string          `json:"project_id,omitempty" jsonschema:"UUID of an existing project to add as a link"`
}

type communityTaskCreate struct {
	Title string `json:"title" jsonschema:"Nonempty title, at most 160 bytes"`
	communityTaskFields
}

type communityTaskUpdate struct {
	communityRevision
	Title *string `json:"title,omitempty" jsonschema:"New nonempty title, at most 160 bytes"`
	communityTaskFields
}

func (f communityTaskFields) present() bool {
	return f.Status != nil || f.Priority != nil || f.Due != nil || f.DueTime != nil || f.Start != nil || f.Tags != nil || f.Entity != nil || f.ProjectID != nil
}

func (s *Server) communityTaskFields(ctx context.Context, t *domain.Task, f communityTaskFields) error {
	if f.Status != nil {
		switch *f.Status {
		case "inbox", "next", "scheduled", "waiting":
			t.Status = *f.Status
		default:
			return problem("invalid_input", "task status must be inbox, next, scheduled or waiting")
		}
	}
	if err := communityPatchPriority(&t.Priority, f.Priority); err != nil {
		return err
	}
	if err := communityPatchDay(&t.Due, f.Due); err != nil {
		return err
	}
	if err := communityPatchDay(&t.Start, f.Start); err != nil {
		return err
	}
	if t.Due != "" && t.Start != "" && t.Start > t.Due {
		return problem("invalid_input", "task start date must not follow its due date")
	}
	if f.DueTime != nil {
		if !domain.ClockValid(*f.DueTime) {
			return problem("invalid_input", "due time must be HH:MM or empty")
		}
		t.DueTime = *f.DueTime
	}
	if err := communityPatchTags(&t.Tags, f.Tags); err != nil {
		return err
	}
	if err := s.communityLinkEntity(ctx, &t.Entities, f.Entity); err != nil {
		return err
	}
	return s.communityLinkProject(ctx, &t.Projects, f.ProjectID)
}

func (s *Server) communitySaveTask(ctx context.Context, t domain.Task) (communityWrite, error) {
	if t.Validate() != nil {
		return communityWrite{}, problem("invalid_input", "task fields are invalid")
	}
	saved, err := s.repo.SaveTask(ctx, t)
	if err != nil {
		return communityWrite{}, communityError(err)
	}
	return communityWrite{ID: saved.ID, Revision: saved.Revision}, nil
}

func (s *Server) communityCreateTask(ctx context.Context, in communityTaskCreate) (communityWrite, error) {
	title, err := communityText(in.Title, communityTitle, true)
	if err != nil {
		return communityWrite{}, err
	}
	t := domain.Task{Title: title, Status: "inbox"}
	if err := s.communityTaskFields(ctx, &t, in.communityTaskFields); err != nil {
		return communityWrite{}, err
	}
	return s.communitySaveTask(ctx, t)
}

func (s *Server) communityUpdateTask(ctx context.Context, in communityTaskUpdate) (communityWrite, error) {
	if err := in.validate(); err != nil {
		return communityWrite{}, err
	}
	if in.Title == nil && !in.communityTaskFields.present() {
		return communityWrite{}, problem("invalid_input", "provide at least one task field to update")
	}
	t, err := s.communityReadTask(ctx, in.ID)
	if err != nil {
		return communityWrite{}, err
	}
	if !t.Open() {
		return communityWrite{}, problem("read_only", "the task is closed")
	}
	if err := communityMatchRevision(t.Revision, in.Revision); err != nil {
		return communityWrite{}, err
	}
	if err := communityPatchText(&t.Title, in.Title, communityTitle, true); err != nil {
		return communityWrite{}, err
	}
	if err := s.communityTaskFields(ctx, &t, in.communityTaskFields); err != nil {
		return communityWrite{}, err
	}
	if t.Validate() != nil {
		return communityWrite{}, problem("invalid_input", "task fields are invalid")
	}
	store, ok := s.repo.(interface {
		SaveTaskPreservingActivityLinks(context.Context, domain.Task) (int, error)
	})
	if !ok {
		return communityWrite{}, problem("unavailable", "link-preserving task updates are unavailable")
	}
	revision, err := store.SaveTaskPreservingActivityLinks(ctx, t)
	if err != nil {
		return communityWrite{}, communityError(err)
	}
	return communityWrite{ID: t.ID, Revision: revision}, nil
}

func (s *Server) communityCompleteTask(ctx context.Context, in communityRevision) (communityWrite, error) {
	if err := in.validate(); err != nil {
		return communityWrite{}, err
	}
	store, ok := s.repo.(interface {
		CompleteTaskRevision(context.Context, string, int) (domain.Task, error)
	})
	if !ok {
		return communityWrite{}, problem("unavailable", "revision-aware task completion is unavailable")
	}
	t, err := store.CompleteTaskRevision(ctx, in.ID, in.Revision)
	if err != nil {
		return communityWrite{}, communityError(err)
	}
	return communityWrite{ID: t.ID, Revision: t.Revision}, nil
}

type communityProjectFields struct {
	Status   *string          `json:"status,omitempty" jsonschema:"planned, active, on-hold or completed; default active"`
	Priority *int             `json:"priority,omitempty" jsonschema:"0 none, 1 low, 2 medium, 3 high"`
	Start    *string          `json:"start,omitempty" jsonschema:"Start date YYYY-MM-DD; empty clears it"`
	Target   *string          `json:"target,omitempty" jsonschema:"Target date YYYY-MM-DD; empty clears it"`
	Tags     []string         `json:"tags,omitempty" jsonschema:"Replace tags with up to 16 labels, or [] to clear them"`
	Member   *communityEntity `json:"member,omitempty" jsonschema:"Existing contact or organization to add as a member"`
}

type communityProjectCreate struct {
	Name string `json:"name" jsonschema:"Nonempty project name, at most 160 bytes"`
	communityProjectFields
}

type communityProjectUpdate struct {
	communityRevision
	Name *string `json:"name,omitempty" jsonschema:"New nonempty project name, at most 160 bytes"`
	communityProjectFields
}

func (f communityProjectFields) present() bool {
	return f.Status != nil || f.Priority != nil || f.Start != nil || f.Target != nil || f.Tags != nil || f.Member != nil
}

func (s *Server) communityProjectFields(ctx context.Context, p *domain.Project, f communityProjectFields) error {
	if f.Status != nil {
		if *f.Status == "archived" || !slices.Contains(domain.ProjectStatuses, *f.Status) {
			return problem("invalid_input", "project status must be planned, active, on-hold or completed")
		}
		p.Status = *f.Status
	}
	if err := communityPatchPriority(&p.Priority, f.Priority); err != nil {
		return err
	}
	if err := communityPatchDay(&p.Start, f.Start); err != nil {
		return err
	}
	if err := communityPatchDay(&p.Target, f.Target); err != nil {
		return err
	}
	if err := communityPatchTags(&p.Tags, f.Tags); err != nil {
		return err
	}
	return s.communityLinkEntity(ctx, &p.Members, f.Member)
}

func (s *Server) communitySaveProject(ctx context.Context, p domain.Project) (communityWrite, error) {
	if p.Validate() != nil {
		return communityWrite{}, problem("invalid_input", "project fields are invalid")
	}
	saved, err := s.repo.SaveProject(ctx, p)
	if err != nil {
		return communityWrite{}, communityError(err)
	}
	return communityWrite{ID: saved.ID, Revision: saved.Revision}, nil
}

func (s *Server) communityCreateProject(ctx context.Context, in communityProjectCreate) (communityWrite, error) {
	name, err := communityText(in.Name, communityTitle, true)
	if err != nil {
		return communityWrite{}, err
	}
	p := domain.Project{Name: name, Status: "active"}
	if err := s.communityProjectFields(ctx, &p, in.communityProjectFields); err != nil {
		return communityWrite{}, err
	}
	return s.communitySaveProject(ctx, p)
}

func (s *Server) communityUpdateProject(ctx context.Context, in communityProjectUpdate) (communityWrite, error) {
	if err := in.validate(); err != nil {
		return communityWrite{}, err
	}
	if in.Name == nil && !in.communityProjectFields.present() {
		return communityWrite{}, problem("invalid_input", "provide at least one project field to update")
	}
	p, err := s.communityReadProject(ctx, in.ID)
	if err != nil {
		return communityWrite{}, err
	}
	if err := communityMatchRevision(p.Revision, in.Revision); err != nil {
		return communityWrite{}, err
	}
	if err := communityPatchText(&p.Name, in.Name, communityTitle, true); err != nil {
		return communityWrite{}, err
	}
	if err := s.communityProjectFields(ctx, &p, in.communityProjectFields); err != nil {
		return communityWrite{}, err
	}
	return s.communitySaveProject(ctx, p)
}

type communityFollowUpSet struct {
	communityEntity
	Revision     string  `json:"revision" jsonschema:"Opaque expected revision returned by followup_get"`
	Every        *int    `json:"every,omitempty" jsonschema:"Cadence interval 1 to 366; zero clears the cadence"`
	Unit         *string `json:"unit,omitempty" jsonschema:"Cadence unit: day, week or month"`
	Never        *bool   `json:"never,omitempty" jsonschema:"Disable follow-ups; clear any dates or cadence in the same request"`
	NextDate     *string `json:"next_date,omitempty" jsonschema:"Explicit date YYYY-MM-DD; empty clears it"`
	SnoozedUntil *string `json:"snoozed_until,omitempty" jsonschema:"Date YYYY-MM-DD to defer an existing schedule until; empty clears it"`
}

type communityFollowUpWrite struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
}

func (s *Server) communitySetFollowUp(ctx context.Context, in communityFollowUpSet) (communityFollowUpWrite, error) {
	kind, err := s.communityGrantEntity(in.communityEntity)
	if err != nil {
		return communityFollowUpWrite{}, err
	}
	var token [32]byte
	if len(in.Revision) != 64 {
		return communityFollowUpWrite{}, problem("invalid_input", "use the expected revision from followup_get")
	}
	if _, err := hex.Decode(token[:], []byte(in.Revision)); err != nil {
		return communityFollowUpWrite{}, problem("invalid_input", "use the expected revision from followup_get")
	}
	if in.Every == nil && in.Unit == nil && in.Never == nil && in.NextDate == nil && in.SnoozedUntil == nil {
		return communityFollowUpWrite{}, problem("invalid_input", "provide at least one follow-up setting to update")
	}
	source, ok := s.repo.(communityFollowUpSource)
	if !ok {
		return communityFollowUpWrite{}, problem("unavailable", "revision-aware follow-up storage is unavailable")
	}
	f, rev, err := source.FollowUpRevision(ctx, in.ID)
	if err == nil && f.Entity.Kind != kind {
		err = sql.ErrNoRows
	}
	if err != nil {
		return communityFollowUpWrite{}, communityError(err)
	}
	if rev != in.Revision {
		return communityFollowUpWrite{}, problem("conflict", "record changed; reload before retrying")
	}
	if in.Every != nil {
		if *in.Every < 0 || *in.Every > 366 {
			return communityFollowUpWrite{}, problem("invalid_input", "cadence interval must be between 0 and 366")
		}
		f.Interval.Every = *in.Every
	}
	if in.Unit != nil {
		if f.Interval.Every == 0 || (*in.Unit != "day" && *in.Unit != "week" && *in.Unit != "month") {
			return communityFollowUpWrite{}, problem("invalid_input", "a cadence unit of day, week or month requires a positive interval")
		}
		f.Interval.Unit = *in.Unit
	}
	if in.Never != nil {
		f.Never = *in.Never
	}
	if err := communityPatchDay(&f.NextDate, in.NextDate); err != nil {
		return communityFollowUpWrite{}, err
	}
	if err := communityPatchDay(&f.SnoozedUntil, in.SnoozedUntil); err != nil {
		return communityFollowUpWrite{}, err
	}
	hasSchedule := f.Interval.Every > 0 || f.NextDate != ""
	if (f.Never && hasSchedule) || (f.SnoozedUntil != "" && !hasSchedule) {
		return communityFollowUpWrite{}, problem("invalid_input", "never cannot be combined with a schedule, and snoozing requires a schedule")
	}
	revision, err := source.SetFollowUpRevision(ctx, f, in.Revision)
	if err != nil {
		return communityFollowUpWrite{}, communityError(err)
	}
	return communityFollowUpWrite{ID: in.ID, Revision: revision}, nil
}
