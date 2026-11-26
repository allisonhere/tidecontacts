package webapp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/allisonhere/tidecontacts/internal/domain"
)

type contactEdit struct {
	Revision     int             `json:"revision"`
	Name         *string         `json:"name"`
	Notes        *string         `json:"notes"`
	Website      *string         `json:"website"`
	Industry     *string         `json:"industry"`
	Tags         *[]string       `json:"tags"`
	Email        *string         `json:"email"`
	Organization json.RawMessage `json:"organization_id"` // null clears current affiliation; omitted keeps it
	Archived     *bool           `json:"archived"`
}

type operationEdit struct {
	Revision    int     `json:"revision"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
	Date        *string `json:"date"`
	Type        *string `json:"type"`
	Priority    *int    `json:"priority"`
	Entity      *string `json:"entity_id"`
	Project     *string `json:"project_id"`
}

func (s Server) save(w http.ResponseWriter, r *http.Request, kind, id string) (any, error) {
	ctx := r.Context()
	if kind == "people" || kind == "organizations" {
		v, err := input[contactEdit](w, r)
		if err != nil {
			return nil, err
		}
		return s.saveContact(ctx, kind, id, v)
	}
	v, err := input[operationEdit](w, r)
	if err != nil {
		return nil, err
	}
	if id == "" && v.Revision != 0 {
		return nil, invalid("New records do not have a revision.")
	}
	if v.Title != nil && (strings.TrimSpace(*v.Title) == "" || len(*v.Title) > 512) {
		return nil, textError("Title", 512)
	}
	if v.Description != nil && len(*v.Description) > 16000 {
		return nil, invalid("Description is limited to 16000 bytes.")
	}
	if v.Date != nil && *v.Date != "" {
		if _, ok := domain.ParseDay(*v.Date); !ok {
			return nil, invalid("Use a date in YYYY-MM-DD format.")
		}
	}
	entity, err := s.activeEntity(ctx, v.Entity)
	if err != nil {
		return nil, err
	}
	project, err := s.activeProject(ctx, v.Project)
	if err != nil {
		return nil, err
	}
	switch kind {
	case "tasks":
		if v.Type != nil {
			return nil, invalid("Tasks do not have an activity type.")
		}
		var t domain.Task
		if id != "" {
			t, err = s.store.Task(ctx, id)
			if err != nil {
				return nil, err
			}
			if t.Deleted || t.Archived {
				return nil, notFound()
			}
			if err = checkRevision(v.Revision, t.Revision); err != nil {
				return nil, err
			}
		}
		patch(&t.Title, v.Title)
		patch(&t.Description, v.Description)
		if v.Status != nil && t.Status != *v.Status && (!t.Open() || *v.Status == "completed" || *v.Status == "canceled") {
			return nil, invalid("Use a task lifecycle action to complete, reopen or cancel a task.")
		}
		patch(&t.Status, v.Status)
		patch(&t.Priority, v.Priority)
		patch(&t.Due, v.Date)
		t.Entities, t.Projects = include(t.Entities, entity), include(t.Projects, project)
		t.Normalize()
		if err = t.Validate(); err != nil {
			return nil, invalid(err.Error())
		}
		return s.store.SaveTask(ctx, t)
	case "activities":
		if v.Priority != nil || v.Status != nil {
			return nil, invalid("Activities do not have priority or state.")
		}
		var a domain.Activity
		if id != "" {
			a, err = s.store.Activity(ctx, id)
			if err != nil {
				return nil, err
			}
			if a.Deleted {
				return nil, notFound()
			}
			if a.Provenance != domain.Local || a.Type == "task-completion" {
				return nil, denied("Edit this activity in its original source.")
			}
			if err = checkRevision(v.Revision, a.Revision); err != nil {
				return nil, err
			}
		}
		patch(&a.Title, v.Title)
		patch(&a.Body, v.Description)
		patch(&a.Type, v.Type)
		if v.Date != nil && *v.Date != "" {
			loc := time.Local
			if a.Timezone != "" {
				if recordZone, e := time.LoadLocation(a.Timezone); e == nil {
					loc = recordZone
				}
			}
			d, _ := time.ParseInLocation(time.DateOnly, *v.Date, loc)
			if a.OccurredAt.IsZero() {
				a.OccurredAt = d
			} else {
				h, m, sec := a.OccurredAt.In(loc).Clock()
				a.OccurredAt = time.Date(d.Year(), d.Month(), d.Day(), h, m, sec, a.OccurredAt.Nanosecond(), loc)
			}
		}
		a.Entities, a.Projects = include(a.Entities, entity), include(a.Projects, project)
		a.Normalize(time.Now())
		if a.Type == "task-completion" {
			return nil, invalid("Task completion entries are created by completing a task.")
		}
		if err = a.Validate(); err != nil {
			return nil, invalid(err.Error())
		}
		a, err = s.store.SaveActivity(ctx, a)
		redactActivity(&a)
		return a, err
	case "projects":
		if v.Project != nil || v.Type != nil {
			return nil, invalid("A project cannot be nested or have an activity type.")
		}
		var p domain.Project
		if id != "" {
			p, err = s.store.Project(ctx, id)
			if err != nil {
				return nil, err
			}
			if p.Deleted {
				return nil, notFound()
			}
			if err = checkRevision(v.Revision, p.Revision); err != nil {
				return nil, err
			}
		}
		patch(&p.Name, v.Title)
		patch(&p.Description, v.Description)
		patch(&p.Status, v.Status)
		patch(&p.Target, v.Date)
		patch(&p.Priority, v.Priority)
		if p.Priority < 0 || p.Priority > 3 {
			return nil, invalid("Priority must be between 0 and 3.")
		}
		p.Members = include(p.Members, entity)
		p.Normalize()
		if err = p.Validate(); err != nil {
			return nil, invalid(err.Error())
		}
		return s.store.SaveProject(ctx, p)
	}
	return nil, notFound()
}

func patch[T any](dst *T, v *T) {
	if v != nil {
		*dst = *v
	}
}

func (s Server) saveContact(ctx context.Context, kind, id string, v contactEdit) (any, error) {
	c := domain.NewContact(entityKind(kind))
	if id != "" {
		var err error
		c, err = s.contact(ctx, kind, id)
		if err != nil {
			return nil, err
		}
		if c.DirectoryReadOnly() {
			return nil, denied("This contact belongs to a directory. Edit the original and refresh the directory.")
		}
		if err = checkRevision(v.Revision, c.Revision); err != nil {
			return nil, err
		}
	} else if v.Revision != 0 {
		return nil, invalid("New records do not have a revision.")
	}
	if (v.Name == nil && id == "") || v.Name != nil && (strings.TrimSpace(*v.Name) == "" || len(*v.Name) > 512) {
		return nil, textError("Name", 512)
	}
	if v.Notes != nil && len(*v.Notes) > 16000 {
		return nil, invalid("Notes are limited to 16000 bytes.")
	}
	if v.Tags != nil {
		if len(*v.Tags) > 50 {
			return nil, invalid("Use at most 50 tags.")
		}
		for _, tag := range *v.Tags {
			if len(tag) > 128 {
				return nil, invalid("Each tag is limited to 128 bytes.")
			}
		}
	}
	patch(&c.DisplayName, v.Name)
	patch(&c.Notes, v.Notes)
	patch(&c.Website, v.Website)
	patch(&c.Industry, v.Industry)
	patch(&c.Tags, v.Tags)
	patch(&c.Archived, v.Archived)
	if v.Organization != nil {
		if c.Kind != "person" {
			return nil, invalid("Only people have organization affiliations through this editor.")
		}
		var orgID string
		if !bytes.Equal(v.Organization, []byte("null")) {
			if json.Unmarshal(v.Organization, &orgID) != nil || !validID(orgID) {
				return nil, invalid("Choose an existing organization.")
			}
			org, err := s.contact(ctx, "organizations", orgID)
			if err != nil || org.Archived {
				return nil, invalid("Choose an active organization.")
			}
		}
		found := false
		for i := range c.Memberships {
			m := &c.Memberships[i]
			if m.OrganizationID == orgID && orgID != "" && !found {
				m.Current, found = true, true
			} else {
				m.Current = false
			}
		}
		if !found && orgID != "" {
			c.Memberships = append(c.Memberships, domain.Membership{OrganizationID: orgID, Current: true})
		}
	}
	if v.Email != nil {
		if err := editEmail(&c, strings.TrimSpace(*v.Email)); err != nil {
			return nil, err
		}
	}
	c.Normalize()
	if err := c.Validate(); err != nil {
		return nil, invalid(err.Error())
	}
	out, err := s.store.SaveContacts(ctx, []domain.Contact{c}, "graphical edit")
	if err != nil {
		return nil, err
	}
	out[0].Extensions = nil
	return out[0], nil
}

func editEmail(c *domain.Contact, address string) error {
	if address != "" {
		a, err := mail.ParseAddress(address)
		if err != nil || a.Address != address || len(address) > 320 {
			return invalid("Enter an email address without an accompanying name.")
		}
	}
	index := -1
	for i, m := range c.Methods {
		if m.Kind == "email" && (index < 0 || m.Preferred) {
			index = i
			if m.Preferred {
				break
			}
		}
	}
	if index < 0 {
		if address != "" {
			c.Methods = append(c.Methods, domain.Method{Kind: "email", Value: address, Preferred: true})
		}
		return nil
	}
	m := &c.Methods[index]
	if m.Value == address {
		return nil
	}
	if m.Source != "" && m.Source != "manual" {
		return denied("This email address is managed by its source.")
	}
	if address == "" {
		c.Methods = slices.Delete(c.Methods, index, index+1)
	} else {
		m.Value, m.Verification = address, "unknown"
	}
	return nil
}

func (s Server) activeEntity(ctx context.Context, id *string) (domain.Ref, error) {
	if id == nil || *id == "" {
		return domain.Ref{}, nil
	}
	if !validID(*id) {
		return domain.Ref{}, invalid("Choose an existing person or organization.")
	}
	c, err := s.store.Contact(ctx, *id)
	if err != nil || c.Deleted || c.Archived {
		return domain.Ref{}, invalid("Choose an active person or organization.")
	}
	return domain.Ref{ID: c.ID, Kind: c.Kind}, nil
}

func (s Server) activeProject(ctx context.Context, id *string) (domain.Ref, error) {
	if id == nil || *id == "" {
		return domain.Ref{}, nil
	}
	if !validID(*id) {
		return domain.Ref{}, invalid("Choose an existing project.")
	}
	p, err := s.store.Project(ctx, *id)
	if err != nil || p.Deleted || p.Status == "archived" {
		return domain.Ref{}, invalid("Choose an active project.")
	}
	return domain.Ref{ID: p.ID, Kind: "project"}, nil
}

func include(records []domain.Ref, r domain.Ref) []domain.Ref {
	if r.ID == "" || slices.ContainsFunc(records, func(item domain.Ref) bool { return item.ID == r.ID }) {
		return records
	}
	return append(records, r)
}

func (s Server) action(w http.ResponseWriter, r *http.Request, kind, id string) (any, error) {
	ctx := r.Context()
	if kind == "tasks" {
		v, err := input[struct {
			Revision int `json:"revision"`
		}](w, r)
		if err != nil {
			return nil, err
		}
		if v.Revision < 1 {
			return nil, invalid("The current task revision is required.")
		}
		return s.store.CompleteTaskRevision(ctx, id, v.Revision)
	}
	v, err := input[struct {
		Revision int    `json:"revision"`
		Stage    string `json:"stage_id"`
	}](w, r)
	if err != nil {
		return nil, err
	}
	if v.Revision < 1 || !validID(v.Stage) {
		return nil, invalid("The item's current revision and a stage ID are required.")
	}
	item, err := s.store.Item(ctx, id)
	if err != nil {
		return nil, err
	}
	if item.Archived || item.Deleted {
		return nil, notFound()
	}
	if err = checkRevision(v.Revision, item.Revision); err != nil {
		return nil, err
	}
	p, err := s.pipeline(ctx, item.PipelineID)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(p.Stages, func(stage domain.Stage) bool { return stage.ID == v.Stage }) {
		return nil, invalid("Choose a stage on the item's current board.")
	}
	if item.StageID == v.Stage {
		return item, nil
	}
	if err = s.store.MoveItemRevision(ctx, id, v.Stage, v.Revision); err != nil {
		return nil, err
	}
	return s.store.Item(ctx, id)
}
