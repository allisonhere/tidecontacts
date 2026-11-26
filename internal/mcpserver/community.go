package mcpserver

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
	"github.com/google/uuid"
)

const (
	communityTitle = 160
	communityLabel = 64
	communityLimit = 100
	communityLinks = 16
	communityTags  = 16
)

func (s *Server) addCommunity() {
	addTool(s, "contact_search", "contacts.read", "Search live contacts by name or tag. Follow next_cursor even after an empty page; reuse the same query.", false, s.communityRecords("contact"))
	addTool(s, "contact_get", "contacts.read", "Read one live contact's identity, preferred email and tags by UUID.", false, s.communityContact("person"))
	addTool(s, "organization_search", "organizations.read", "Search live organizations by name or tag. Follow next_cursor even after an empty page; reuse the same query.", false, s.communityRecords("organization"))
	addTool(s, "organization_get", "organizations.read", "Read one live organization's name, website, industry and tags by UUID.", false, s.communityContact("organization"))
	addTool(s, "relationships_get", "relationships.read", "Read recorded relationship edges, memberships and projects for a contact or organization, limited to granted scopes.", false, s.communityConnections)
	addTool(s, "dossier_get", "relationships.read", "Read the note-free CRM dossier for a contact or organization. Linked sections require their own read scopes.", false, s.communityDossier)

	addTool(s, "activity_list", "activities.read", "Page through activity summaries by UUID; optionally search titles. Follow next_cursor even after an empty page.", false, s.communityRecords("activity"))
	addTool(s, "activity_get", "activities.read", "Read one recorded activity's title, date, type and permitted links, without its body.", false, s.communityActivity)
	addTool(s, "task_list", "tasks.read", "Page through task summaries by UUID; optionally search titles. Follow next_cursor even after an empty page.", false, s.communityRecords("task"))
	addTool(s, "task_get", "tasks.read", "Read one live task's title, status, dates, priority and permitted links.", false, s.communityTask)
	addTool(s, "project_list", "projects.read", "Page through live project summaries by UUID; optionally search names. Follow next_cursor even after an empty page.", false, s.communityRecords("project"))
	addTool(s, "project_get", "projects.read", "Read one live project's status, dates, milestones and permitted members.", false, s.communityProject)
	addTool(s, "pipeline_list", "pipelines.read", "List live pipelines in board order, with stage names and item counts.", false, s.communityPipelines)
	addTool(s, "pipeline_item_list", "pipelines.read", "Page through live item titles and current stages in a live pipeline by UUID.", false, s.communityPipelineItems)
	addTool(s, "pipeline_item_get", "pipelines.read", "Read a pipeline item's title and current stage without notes or next-action text.", false, s.communityPipelineItem)
	addTool(s, "followup_list", "followups.read", "Page through configured follow-ups for contacts or organizations; the corresponding entity read scope is also required.", false, s.communityFollowUps)
	addTool(s, "followup_get", "followups.read", "Read one follow-up schedule, computed due date and the settings revision needed for an update.", false, s.communityFollowUp)
	addTool(s, "radar_list", "radar.read", "Read recorded relationship signals for granted sections, by kind and date, without private explanatory text.", false, s.communityRadar)
	addTool(s, "inbox_list", "inbox.read", "Read a non-mutating People Inbox snapshot for granted sections, without notes or reasons.", false, s.communityInbox)

	addTool(s, "contact_create", "contacts.write", "Create a person with a name and optional plain email, tags and existing organization link.", true, s.communityCreateContact)
	addTool(s, "contact_update", "contacts.write", "At the expected revision, patch a person's names or tags or add an existing organization link. Other fields are preserved.", true, s.communityUpdateContact)
	addTool(s, "activity_create", "activities.write", "Log a locally owned activity with an explicit title and optional authorized record links, without a body or external source metadata.", true, s.communityCreateActivity)
	addTool(s, "activity_update", "activities.write", "At the expected revision, patch a locally owned activity's title, type, date or tags, or add authorized links. The body is preserved.", true, s.communityUpdateActivity)
	addTool(s, "task_create", "tasks.write", "Create an open task with its title, dates, priority, tags and optional authorized links.", true, s.communityCreateTask)
	addTool(s, "task_update", "tasks.write", "At the expected revision, patch an open task or add authorized links. Use task_complete to complete a task.", true, s.communityUpdateTask)
	addTool(s, "task_complete", "tasks.write", "Complete a task at its expected revision, with normal timeline and recurrence behavior.", true, s.communityCompleteTask)
	addTool(s, "project_create", "projects.write", "Create a project with its name, status, dates, priority, tags and optional authorized member link.", true, s.communityCreateProject)
	addTool(s, "project_update", "projects.write", "At the expected revision, patch a project or add an authorized member link without editing descriptions or milestones.", true, s.communityUpdateProject)
	addTool(s, "followup_set", "followups.write", "Patch a contact's or organization's follow-up settings at the revision returned by followup_get. Omitted fields stay unchanged.", true, s.communitySetFollowUp)
}

type communityKey struct {
	ID string `json:"id" jsonschema:"UUID of the live record"`
}

type communityEntity struct {
	ID   string `json:"id" jsonschema:"UUID of the live person or organization"`
	Kind string `json:"kind" jsonschema:"contact or organization"`
}

type communitySearch struct {
	Query  string `json:"query,omitempty" jsonschema:"Literal case-insensitive substring of a name or title (also tags for contacts and organizations)"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque next_cursor from the previous page, exclusive; keep the query and type unchanged"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Page size, 1 to 100; default 25"`
}

type communityCount struct {
	Limit int `json:"limit,omitempty" jsonschema:"Maximum number of results, 1 to 100; default 25"`
}

type communityPage[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type communityList[T any] struct {
	Items []T `json:"items"`
}

type communityRecord struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Name     string   `json:"name"`
	Status   string   `json:"status,omitempty"`
	Type     string   `json:"type,omitempty"`
	Due      string   `json:"due,omitempty"`
	Date     string   `json:"date,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Priority int      `json:"priority,omitempty"`
	Revision int      `json:"revision,omitempty"`
}

type communityRef struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name,omitempty"`
}

func communityError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return problem("not_found", "live record not found")
	case errors.Is(err, sqlite.ErrConflict):
		return problem("conflict", "record changed; reload before retrying")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return problem("canceled", "operation interrupted")
	default:
		return problem("runtime", "CRM operation failed")
	}
}

func communityControl(r rune) bool {
	return unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp)
}

func communityText(s string, maxBytes int, required bool) (string, error) {
	s = strings.TrimSpace(s)
	if (required && s == "") || len(s) > maxBytes || !utf8.ValidString(s) || strings.ContainsFunc(s, communityControl) {
		return "", problem("invalid_input", "text field is empty, too long or contains invalid characters")
	}
	return s, nil
}

func communityClip(s string, maxBytes int) string {
	if s == "" {
		return ""
	}
	s = strings.TrimSpace(s)
	if len(s) > maxBytes {
		for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
			maxBytes--
		}
		s = s[:maxBytes]
	}
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if communityControl(r) || r == utf8.RuneError {
			return ' '
		}
		return r
	}, s))
}

func communityUUID(id string) bool {
	if len(id) != 36 {
		return false
	}
	u, err := uuid.Parse(id)
	return err == nil && u != uuid.Nil
}

func communityValidID(id string) error {
	if !communityUUID(id) {
		return problem("invalid_input", "record ID must be a nonzero UUID")
	}
	return nil
}

func communityPageLimit(n int) (int, error) {
	if n < 0 || n > communityLimit {
		return 0, problem("invalid_input", "limit must be between 1 and 100")
	}
	if n == 0 {
		n = 25
	}
	return n, nil
}

func communityCursor(cursor string, key [8]byte) (string, error) {
	if cursor == "" {
		return "", nil
	}
	if len(cursor) > base64.RawURLEncoding.EncodedLen(128) {
		return "", problem("invalid_input", "invalid page cursor")
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil || len(data) <= len(key) || [8]byte(data[:8]) != key {
		return "", problem("invalid_input", "cursor does not belong to this page")
	}
	id := string(data[len(key):])
	if !utf8.ValidString(id) || strings.ContainsFunc(id, communityControl) {
		return "", problem("invalid_input", "invalid page cursor")
	}
	return id, nil
}

func communityPageKey(kind, filter string) [8]byte {
	sum := sha256.Sum256([]byte(kind + "\x00" + filter))
	return [8]byte(sum[:8])
}

func communityNext(id string, key [8]byte) string {
	return base64.RawURLEncoding.EncodeToString(append(key[:], id...))
}

func communityDisplayTags(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, min(len(in), communityTags))
	for _, tag := range in[:min(len(in), communityTags)] {
		if tag := communityClip(tag, communityLabel); tag != "" {
			out = append(out, tag)
		}
	}
	return out
}

func (s *Server) communityRecords(kind string) func(context.Context, communitySearch) (communityPage[communityRecord], error) {
	return func(ctx context.Context, in communitySearch) (communityPage[communityRecord], error) {
		return s.communityRecordPage(ctx, kind, in)
	}
}

func (s *Server) communityRecordPage(ctx context.Context, kind string, in communitySearch) (communityPage[communityRecord], error) {
	var out communityPage[communityRecord]
	query, err := communityText(in.Query, 128, false)
	if err != nil {
		return out, err
	}
	query = strings.ToLower(query)
	key := communityPageKey(kind, query)
	after, err := communityCursor(in.Cursor, key)
	if err != nil {
		return out, err
	}
	limit, err := communityPageLimit(in.Limit)
	if err != nil {
		return out, err
	}
	source, ok := s.repo.(app.RuleRecordSource)
	if !ok {
		return out, problem("unavailable", "bounded CRM record paging is unavailable")
	}
	out.Items = make([]communityRecord, 0, limit)
	// Bound both SQL rows read and matching work. A sparse search can return an
	// empty page with a cursor; only a missing next_cursor means end of results.
	batchSize := 100
	if query == "" {
		batchSize = limit + 1
	}
	for range 4 {
		batch, err := source.RuleRecords(ctx, kind, after, batchSize)
		if err != nil {
			return communityPage[communityRecord]{}, communityError(err)
		}
		for _, r := range batch {
			name := communityClip(r.Name, communityTitle)
			tags := communityDisplayTags(r.Tags)
			if communityUUID(r.ID) && (strings.Contains(strings.ToLower(name), query) || slices.ContainsFunc(tags, func(t string) bool {
				return strings.Contains(strings.ToLower(t), query)
			})) {
				if len(out.Items) == limit {
					out.NextCursor = communityNext(after, key)
					return out, nil
				}
				revision := r.Revision
				if r.Kind == "followup" {
					revision = 0 // Follow-up settings have their own opaque revision.
				}
				out.Items = append(out.Items, communityRecord{ID: r.ID, Kind: r.Kind, Name: name,
					Status: communityClip(r.Status, communityLabel), Type: communityClip(r.Detail, communityLabel),
					Date: communityClip(r.When, 10), Due: communityClip(r.Due, 10), Tags: tags, Priority: r.Priority, Revision: revision})
			}
			after = r.ID
		}
		if len(batch) < batchSize {
			return out, nil
		}
	}
	out.NextCursor = communityNext(after, key)
	return out, nil
}

func communityScope(kind string) string {
	switch kind {
	case "person", "contact":
		return "contacts.read"
	case "organization":
		return "organizations.read"
	case "activity":
		return "activities.read"
	case "task":
		return "tasks.read"
	case "project":
		return "projects.read"
	case "pipeline", "item":
		return "pipelines.read"
	case "reminder":
		return "inbox.read"
	default:
		return ""
	}
}

func communityEntityKind(kind string) string {
	switch kind {
	case "contact":
		return "person"
	case "organization":
		return "organization"
	default:
		return ""
	}
}

func (s *Server) communityGrantEntity(in communityEntity) (string, error) {
	kind := communityEntityKind(in.Kind)
	if kind == "" {
		return "", problem("invalid_input", "entity kind must be contact or organization")
	}
	if err := s.require(communityScope(kind)); err != nil {
		return "", err
	}
	return kind, communityValidID(in.ID)
}

func (s *Server) communityReadContact(ctx context.Context, id, kind string) (domain.Contact, error) {
	if err := communityValidID(id); err != nil {
		return domain.Contact{}, err
	}
	c, err := s.repo.Contact(ctx, id)
	if err == nil && (c.Deleted || c.Archived || c.Kind != kind) {
		err = sql.ErrNoRows
	}
	return c, communityError(err)
}

func (s *Server) communityReference(r domain.Ref) *communityRef {
	scope := communityScope(r.Kind)
	if scope == "" || !communityUUID(r.ID) || s.require(scope) != nil {
		return nil
	}
	if r.Kind == "person" {
		r.Kind = "contact"
	}
	if r.Kind == "activity" {
		// Link aggregates lack the body needed to identify a fallback title.
		r.Name = ""
	}
	return &communityRef{ID: r.ID, Kind: r.Kind, Name: communityClip(r.Name, communityTitle)}
}

func (s *Server) communityReferences(groups ...[]domain.Ref) []communityRef {
	var out []communityRef
	for _, group := range groups {
		for _, r := range group[:min(len(group), communityLinks)] {
			if ref := s.communityReference(r); ref != nil {
				out = append(out, *ref)
			}
		}
	}
	return out
}

type communityContactView struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Name     string   `json:"name"`
	Given    string   `json:"given_name,omitempty"`
	Family   string   `json:"family_name,omitempty"`
	Email    string   `json:"email,omitempty"`
	Website  string   `json:"website,omitempty"`
	Industry string   `json:"industry,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Revision int      `json:"revision"`
}

func (s *Server) communityContact(kind string) func(context.Context, communityKey) (communityContactView, error) {
	return func(ctx context.Context, in communityKey) (communityContactView, error) {
		c, err := s.communityReadContact(ctx, in.ID, kind)
		if err != nil {
			return communityContactView{}, err
		}
		out := communityContactView{ID: c.ID, Kind: kind, Name: communityClip(c.DisplayName, communityTitle),
			Tags: communityDisplayTags(c.Tags), Revision: c.Revision}
		if c.Kind == "organization" {
			out.Website, out.Industry = communityClip(c.Website, 256), communityClip(c.Industry, communityLabel)
		} else {
			out.Kind = "contact"
			out.Given, out.Family = communityClip(c.Given, communityLabel), communityClip(c.Family, communityLabel)
			for _, m := range c.Methods {
				if m.Kind == "email" && (out.Email == "" || m.Preferred) {
					out.Email = communityClip(m.Value, 254)
					if m.Preferred {
						break
					}
				}
			}
		}
		return out, nil
	}
}

type communityActivityView struct {
	ID         string         `json:"id"`
	Title      string         `json:"title"`
	Type       string         `json:"type"`
	OccurredAt string         `json:"occurred_at"`
	Provenance string         `json:"provenance"`
	Tags       []string       `json:"tags,omitempty"`
	Links      []communityRef `json:"links,omitempty"`
	Revision   int            `json:"revision"`
}

func (s *Server) communityActivityView(a domain.Activity) communityActivityView {
	return communityActivityView{ID: a.ID, Title: communityClip(a.SummaryTitle(), communityTitle), Type: communityClip(a.Type, communityLabel),
		OccurredAt: a.OccurredAt.UTC().Format(time.RFC3339), Provenance: communityClip(a.Provenance, communityLabel),
		Tags: communityDisplayTags(a.Tags), Links: s.communityReferences(a.Entities, a.Projects, a.Tasks), Revision: a.Revision}
}

func (s *Server) communityActivity(ctx context.Context, in communityKey) (communityActivityView, error) {
	if err := communityValidID(in.ID); err != nil {
		return communityActivityView{}, err
	}
	a, err := s.repo.Activity(ctx, in.ID)
	if err == nil && a.Deleted {
		err = sql.ErrNoRows
	}
	if err != nil {
		return communityActivityView{}, communityError(err)
	}
	return s.communityActivityView(a), nil
}

type communityTaskView struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Status      string         `json:"status"`
	Priority    int            `json:"priority"`
	Due         string         `json:"due,omitempty"`
	DueTime     string         `json:"due_time,omitempty"`
	Start       string         `json:"start,omitempty"`
	CompletedAt string         `json:"completed_at,omitempty"`
	Recurring   bool           `json:"recurring,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Links       []communityRef `json:"links,omitempty"`
	Revision    int            `json:"revision"`
}

func (s *Server) communityTaskView(t domain.Task) communityTaskView {
	return communityTaskView{ID: t.ID, Title: communityClip(t.Title, communityTitle), Status: communityClip(t.Status, communityLabel),
		Priority: t.Priority, Due: communityClip(t.Due, 10), DueTime: communityClip(t.DueTime, 5), Start: communityClip(t.Start, 10),
		CompletedAt: communityClip(t.CompletedAt, 32), Recurring: !t.Recurrence.IsZero(), Tags: communityDisplayTags(t.Tags),
		Links: s.communityReferences(t.Entities, t.Projects, t.Activities), Revision: t.Revision}
}

func (s *Server) communityReadTask(ctx context.Context, id string) (domain.Task, error) {
	if err := communityValidID(id); err != nil {
		return domain.Task{}, err
	}
	t, err := s.repo.Task(ctx, id)
	if err == nil && (t.Archived || t.Deleted) {
		err = sql.ErrNoRows
	}
	return t, communityError(err)
}

func (s *Server) communityTask(ctx context.Context, in communityKey) (communityTaskView, error) {
	t, err := s.communityReadTask(ctx, in.ID)
	if err != nil {
		return communityTaskView{}, err
	}
	return s.communityTaskView(t), nil
}

type communityMilestone struct {
	Title  string `json:"title"`
	Due    string `json:"due,omitempty"`
	DoneAt string `json:"done_at,omitempty"`
}

type communityProjectView struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	Status     string               `json:"status"`
	Priority   int                  `json:"priority"`
	Start      string               `json:"start,omitempty"`
	Target     string               `json:"target,omitempty"`
	OpenTasks  *int                 `json:"open_tasks,omitempty"`
	DoneTasks  *int                 `json:"done_tasks,omitempty"`
	Tags       []string             `json:"tags,omitempty"`
	Members    []communityRef       `json:"members,omitempty"`
	Milestones []communityMilestone `json:"milestones,omitempty"`
	Revision   int                  `json:"revision"`
}

func (s *Server) communityReadProject(ctx context.Context, id string) (domain.Project, error) {
	if err := communityValidID(id); err != nil {
		return domain.Project{}, err
	}
	p, err := s.repo.Project(ctx, id)
	if err == nil && (p.Deleted || p.Status == "archived") {
		err = sql.ErrNoRows
	}
	return p, communityError(err)
}

func (s *Server) communityProject(ctx context.Context, in communityKey) (communityProjectView, error) {
	p, err := s.communityReadProject(ctx, in.ID)
	if err != nil {
		return communityProjectView{}, err
	}
	out := communityProjectView{ID: p.ID, Name: communityClip(p.Name, communityTitle), Status: communityClip(p.Status, communityLabel),
		Priority: p.Priority, Start: communityClip(p.Start, 10), Target: communityClip(p.Target, 10), Tags: communityDisplayTags(p.Tags),
		Members: s.communityReferences(p.Members), Revision: p.Revision}
	if s.require("tasks.read") == nil {
		out.OpenTasks, out.DoneTasks = &p.OpenTasks, &p.DoneTasks
	}
	for _, m := range p.Milestones[:min(len(p.Milestones), communityLinks)] {
		out.Milestones = append(out.Milestones, communityMilestone{Title: communityClip(m.Title, communityTitle),
			Due: communityClip(m.Due, 10), DoneAt: communityClip(m.DoneAt, 32)})
	}
	return out, nil
}

type communityEdge struct {
	Target communityRef `json:"target"`
	Type   string       `json:"type"`
	Count  int          `json:"count,omitempty"`
}

type communityMembership struct {
	Organization communityRef   `json:"organization"`
	Title        string         `json:"title,omitempty"`
	Current      bool           `json:"current"`
	People       []communityRef `json:"people,omitempty"`
}

type communityConnectionsView struct {
	Root          *communityRef         `json:"root"`
	Flagged       bool                  `json:"flagged"`
	Outgoing      []communityEdge       `json:"outgoing,omitempty"`
	Incoming      []communityEdge       `json:"incoming,omitempty"`
	Shared        []communityEdge       `json:"shared_activities,omitempty"`
	Organizations []communityMembership `json:"organizations,omitempty"`
	Projects      []communityRef        `json:"projects,omitempty"`
}

func (s *Server) communityEdges(input []domain.Edge) []communityEdge {
	var out []communityEdge
	for _, e := range input[:min(len(input), communityLinks)] {
		if r := s.communityReference(e.Person); r != nil {
			out = append(out, communityEdge{Target: *r, Type: communityClip(e.Type, communityLabel), Count: e.Count})
		}
	}
	return out
}

func (s *Server) communityConnectionsView(c domain.Connections) communityConnectionsView {
	out := communityConnectionsView{Root: s.communityReference(c.Root), Flagged: c.Flagged,
		Outgoing: s.communityEdges(c.Outgoing), Incoming: s.communityEdges(c.Incoming)}
	if s.require("activities.read") == nil {
		out.Shared = s.communityEdges(c.Shared)
	}
	for _, o := range c.Organizations[:min(len(c.Organizations), communityLinks)] {
		if org := s.communityReference(o.Organization); org != nil {
			out.Organizations = append(out.Organizations, communityMembership{Organization: *org,
				Title: communityClip(o.Title, communityLabel), Current: o.Current, People: s.communityReferences(o.People)})
		}
	}
	for _, p := range c.Projects[:min(len(c.Projects), communityLinks)] {
		if project := s.communityReference(p.Project); project != nil {
			out.Projects = append(out.Projects, *project)
		}
	}
	return out
}

func (s *Server) communityCheckEntity(ctx context.Context, in communityEntity) (string, error) {
	kind, err := s.communityGrantEntity(in)
	if err != nil {
		return "", err
	}
	_, err = s.communityReadContact(ctx, in.ID, kind)
	return kind, err
}

func (s *Server) communityConnections(ctx context.Context, in communityEntity) (communityConnectionsView, error) {
	if _, err := s.communityCheckEntity(ctx, in); err != nil {
		return communityConnectionsView{}, err
	}
	c, err := s.repo.Connections(ctx, in.ID)
	if err != nil {
		return communityConnectionsView{}, communityError(err)
	}
	return s.communityConnectionsView(c), nil
}

type communityDossierView struct {
	Relationships communityConnectionsView `json:"relationships"`
	Recent        []communityActivityView  `json:"recent_activities,omitempty"`
	ActivityCount *int                     `json:"activity_count,omitempty"`
	OpenTasks     []communityTaskView      `json:"open_tasks,omitempty"`
	Commitments   []communityTaskView      `json:"commitments,omitempty"`
	OpenTaskCount *int                     `json:"open_task_count,omitempty"`
	FollowUp      *communityFollowUpView   `json:"followup,omitempty"`
}

func (s *Server) communityDossier(ctx context.Context, in communityEntity) (communityDossierView, error) {
	if _, err := s.communityCheckEntity(ctx, in); err != nil {
		return communityDossierView{}, err
	}
	d, err := s.repo.Dossier(ctx, in.ID)
	if err != nil {
		return communityDossierView{}, communityError(err)
	}
	out := communityDossierView{Relationships: s.communityConnectionsView(d.Connections)}
	if s.require("activities.read") == nil {
		out.ActivityCount = &d.Activities
		for _, a := range d.Recent[:min(len(d.Recent), communityLinks)] {
			out.Recent = append(out.Recent, s.communityActivityView(a))
		}
	}
	if s.require("tasks.read") == nil {
		out.OpenTaskCount = &d.OpenTaskCount
		for _, t := range d.OpenTasks[:min(len(d.OpenTasks), communityLinks)] {
			out.OpenTasks = append(out.OpenTasks, s.communityTaskView(t))
		}
		for _, t := range d.Commitments[:min(len(d.Commitments), communityLinks)] {
			out.Commitments = append(out.Commitments, s.communityTaskView(t))
		}
	}
	if s.require("followups.read") == nil {
		follow := s.communityFollowUpView(d.FollowUp, "")
		out.FollowUp = &follow
	}
	return out, nil
}

type communityStage struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ItemCount int    `json:"item_count"`
}

type communityPipeline struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Stages []communityStage `json:"stages"`
}

func (s *Server) communityPipelines(ctx context.Context, in communityCount) (communityList[communityPipeline], error) {
	var out communityList[communityPipeline]
	limit, err := communityPageLimit(in.Limit)
	if err != nil {
		return out, err
	}
	ps, err := s.repo.Pipelines(ctx, false)
	if err != nil {
		return out, communityError(err)
	}
	out.Items = make([]communityPipeline, 0, min(len(ps), limit))
	for _, p := range ps {
		if p.Archived || !communityUUID(p.ID) {
			continue
		}
		result := communityPipeline{ID: p.ID, Name: communityClip(p.Name, communityTitle), Stages: make([]communityStage, 0, min(len(p.Stages), communityLimit))}
		for _, st := range p.Stages[:min(len(p.Stages), communityLimit)] {
			if communityUUID(st.ID) {
				result.Stages = append(result.Stages, communityStage{ID: st.ID, Name: communityClip(st.Name, communityLabel), ItemCount: st.Count})
			}
		}
		out.Items = append(out.Items, result)
		if len(out.Items) == limit {
			break
		}
	}
	return out, nil
}

type communityItemView struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	PipelineID string `json:"pipeline_id"`
	StageID    string `json:"stage_id"`
	Stage      string `json:"stage"`
	EnteredAt  string `json:"stage_entered_at,omitempty"`
	Revision   int    `json:"revision"`
}

func communityPipelineItemView(i domain.PipelineItem) communityItemView {
	return communityItemView{ID: i.ID, Title: communityClip(i.Title, communityTitle), PipelineID: i.PipelineID, StageID: i.StageID,
		Stage: communityClip(i.Stage, communityLabel), EnteredAt: communityClip(i.StageEnteredAt, 32), Revision: i.Revision}
}

type communityPipelinePage struct {
	PipelineID string `json:"pipeline_id" jsonschema:"UUID of the live pipeline"`
	Cursor     string `json:"cursor,omitempty" jsonschema:"Opaque next_cursor from the previous page of this pipeline"`
	Limit      int    `json:"limit,omitempty" jsonschema:"Page size, 1 to 100; default 25"`
}

func (s *Server) communityPipelineItems(ctx context.Context, in communityPipelinePage) (communityPage[communityItemView], error) {
	var out communityPage[communityItemView]
	if err := communityValidID(in.PipelineID); err != nil {
		return out, err
	}
	key := communityPageKey("pipeline-items", in.PipelineID)
	after, err := communityCursor(in.Cursor, key)
	if err != nil {
		return out, err
	}
	limit, err := communityPageLimit(in.Limit)
	if err != nil {
		return out, err
	}
	source, ok := s.repo.(interface {
		PipelineItemsPage(context.Context, string, string, int) ([]domain.PipelineItem, error)
	})
	if !ok {
		return out, problem("unavailable", "bounded pipeline paging is unavailable")
	}
	items, err := source.PipelineItemsPage(ctx, in.PipelineID, after, limit+1)
	if err != nil {
		return out, communityError(err)
	}
	count := min(len(items), limit)
	out.Items = make([]communityItemView, 0, count)
	for _, item := range items[:count] {
		if communityUUID(item.ID) && communityUUID(item.StageID) {
			out.Items = append(out.Items, communityPipelineItemView(item))
		}
	}
	if len(items) > limit {
		out.NextCursor = communityNext(items[count-1].ID, key)
	}
	return out, nil
}

func (s *Server) communityPipelineItem(ctx context.Context, in communityKey) (communityItemView, error) {
	if err := communityValidID(in.ID); err != nil {
		return communityItemView{}, err
	}
	source, ok := s.repo.(interface {
		PipelineItemSummary(context.Context, string) (domain.PipelineItem, error)
	})
	if !ok {
		return communityItemView{}, problem("unavailable", "pipeline item summaries are unavailable")
	}
	item, err := source.PipelineItemSummary(ctx, in.ID)
	if err != nil {
		return communityItemView{}, communityError(err)
	}
	return communityPipelineItemView(item), nil
}

type communityFollowUpSource interface {
	FollowUpRevision(context.Context, string) (domain.FollowUp, string, error)
	SetFollowUpRevision(context.Context, domain.FollowUp, string) (string, error)
}

type communityFollowUpPage struct {
	Kind   string `json:"kind" jsonschema:"contact or organization"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque next_cursor from the previous page of this kind"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Page size, 1 to 100; default 25"`
}

func (s *Server) communityFollowUps(ctx context.Context, in communityFollowUpPage) (communityPage[communityRecord], error) {
	kind := communityEntityKind(in.Kind)
	if kind == "" {
		return communityPage[communityRecord]{}, problem("invalid_input", "entity kind must be contact or organization")
	}
	if err := s.require(communityScope(kind)); err != nil {
		return communityPage[communityRecord]{}, err
	}
	return s.communityRecordPage(ctx, "followup-"+in.Kind, communitySearch{Cursor: in.Cursor, Limit: in.Limit})
}

type communityFollowUpView struct {
	Entity          *communityRef `json:"entity"`
	Every           int           `json:"every"`
	Unit            string        `json:"unit,omitempty"`
	Never           bool          `json:"never"`
	NextDate        string        `json:"next_date,omitempty"`
	SnoozedUntil    string        `json:"snoozed_until,omitempty"`
	Due             string        `json:"due,omitempty"`
	LastInteraction string        `json:"last_interaction,omitempty"`
	Revision        string        `json:"revision,omitempty"`
}

func (s *Server) communityFollowUpView(f domain.FollowUp, revision string) communityFollowUpView {
	out := communityFollowUpView{Entity: s.communityReference(f.Entity), Every: f.Interval.Every, Never: f.Never,
		NextDate: communityClip(f.NextDate, 10), SnoozedUntil: communityClip(f.SnoozedUntil, 10), Due: communityClip(f.Due, 10), Revision: revision}
	if f.Interval.Every > 0 {
		out.Unit = communityClip(f.Interval.Unit, communityLabel)
	}
	if s.require("activities.read") == nil {
		out.LastInteraction = communityClip(f.LastInteraction, 10)
	}
	return out
}

func (s *Server) communityFollowUp(ctx context.Context, in communityEntity) (communityFollowUpView, error) {
	kind, err := s.communityGrantEntity(in)
	if err != nil {
		return communityFollowUpView{}, err
	}
	source, ok := s.repo.(communityFollowUpSource)
	if !ok {
		return communityFollowUpView{}, problem("unavailable", "revision-aware follow-up storage is unavailable")
	}
	f, rev, err := source.FollowUpRevision(ctx, in.ID)
	if err == nil && f.Entity.Kind != kind {
		err = sql.ErrNoRows
	}
	if err != nil {
		return communityFollowUpView{}, communityError(err)
	}
	return s.communityFollowUpView(f, rev), nil
}

type communitySignal struct {
	Kind   string       `json:"kind"`
	Date   string       `json:"date,omitempty"`
	Entity communityRef `json:"entity"`
}

func (s *Server) communityRadar(ctx context.Context, in communityCount) (communityList[communitySignal], error) {
	var out communityList[communitySignal]
	limit, err := communityPageLimit(in.Limit)
	if err != nil {
		return out, err
	}
	if s.require("contacts.read") != nil && s.require("organizations.read") != nil {
		return out, problem("permission_denied", "contact or organization read scope is required")
	}
	cfg, err := s.repo.RadarConfig(ctx)
	if err != nil {
		return out, communityError(err)
	}
	cfg.Normalize()
	followups := s.require("followups.read") == nil
	tasks := s.require("tasks.read") == nil
	cfg.Disabled["followup"] = cfg.Disabled["followup"] || !followups
	cfg.Disabled["cadence"] = cfg.Disabled["cadence"] || !followups
	cfg.Disabled["active"] = cfg.Disabled["active"] || s.require("activities.read") != nil
	cfg.Disabled["commitments"] = cfg.Disabled["commitments"] || !tasks
	cfg.Disabled["overdue"] = cfg.Disabled["overdue"] || !tasks || s.require("projects.read") != nil
	cfg.Limit = min(cfg.Limit, limit)
	ss, err := s.repo.Radar(ctx, cfg)
	if err != nil {
		return out, communityError(err)
	}
	out.Items = make([]communitySignal, 0, min(len(ss), limit))
	for _, sig := range ss {
		if cfg.Disabled[sig.Kind] || !slices.ContainsFunc(domain.RadarSignals, func(k [2]string) bool { return k[0] == sig.Kind }) {
			continue
		}
		if ref := s.communityReference(sig.Person); ref != nil {
			out.Items = append(out.Items, communitySignal{Kind: sig.Kind, Date: communityClip(sig.Date, 10), Entity: *ref})
			if len(out.Items) == limit {
				break
			}
		}
	}
	return out, nil
}

type communityInboxItem struct {
	Kind    string        `json:"kind"`
	Title   string        `json:"title"`
	Source  *communityRef `json:"source"`
	Person  *communityRef `json:"person,omitempty"`
	Due     string        `json:"due,omitempty"`
	Urgency int           `json:"urgency"`
	Status  string        `json:"status"`
}

func (s *Server) communityInbox(ctx context.Context, in communityCount) (communityList[communityInboxItem], error) {
	var out communityList[communityInboxItem]
	limit, err := communityPageLimit(in.Limit)
	if err != nil {
		return out, err
	}
	snapshot, ok := s.repo.(interface {
		InboxSnapshot(context.Context) ([]domain.InboxItem, error)
	})
	if !ok {
		return out, problem("unavailable", "read-only inbox snapshot is unavailable")
	}
	rows, err := snapshot.InboxSnapshot(ctx)
	if err != nil {
		return out, communityError(err)
	}
	out.Items = make([]communityInboxItem, 0, min(len(rows), limit))
	followups, relationships := s.require("followups.read") == nil, s.require("relationships.read") == nil
	for _, item := range rows {
		switch item.Kind {
		case "followup-due", "followup-suggested":
			if !followups {
				continue
			}
		case "flagged":
			if !relationships {
				continue
			}
		case "reminder", "task-overdue", "task-waiting", "task-inbox", "date":
		default:
			continue
		}
		source := s.communityReference(item.Source)
		if source == nil {
			continue
		}
		out.Items = append(out.Items, communityInboxItem{Kind: item.Kind, Title: communityClip(item.Title, communityTitle),
			Source: source, Person: s.communityReference(item.Person), Due: communityClip(item.Due, 10), Urgency: item.Urgency, Status: communityClip(item.Status, communityLabel)})
		if len(out.Items) == limit {
			break
		}
	}
	return out, nil
}
