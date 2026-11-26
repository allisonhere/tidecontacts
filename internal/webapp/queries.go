package webapp

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
)

// Each collection has a bounded wire page. Some legacy service methods still
// materialize the matching collection; keep its exact sorting and search rules.
const collectionWindow = 1000

type page[T any] struct {
	Items []T  `json:"items"`
	Next  *int `json:"next_offset"`
}

func window[T any](rows []T, from, size int, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	start, end := min(len(rows), from), min(len(rows), from+size)
	result := page[T]{Items: rows[start:end]}
	if result.Items == nil {
		result.Items = []T{}
	}
	if end < min(len(rows), collectionWindow) {
		result.Next = &end
	}
	return result, nil
}

func entityKind(kind string) string {
	if kind == "organizations" {
		return "organization"
	}
	return "person"
}

func (s Server) list(r *http.Request, kind string) (any, error) {
	params := r.URL.Query()
	getInt := func(name string, fallback, low, high int) (int, error) {
		if !params.Has(name) {
			return fallback, nil
		}
		n, err := strconv.Atoi(params.Get(name))
		if err != nil || n < low || n > high {
			return 0, invalid("Invalid " + name + " for this collection.")
		}
		return n, nil
	}
	offset, err := getInt("offset", 0, 0, collectionWindow-1)
	if err != nil {
		return nil, err
	}
	limit, err := getInt("limit", 50, 1, 100)
	if err != nil {
		return nil, err
	}
	limit = min(limit, collectionWindow-offset)
	fetch := min(offset+limit+1, collectionWindow)
	term := params.Get("q")
	if len(term) > 512 {
		return nil, invalid("Search is limited to 512 bytes.")
	}
	scope := map[string]string{"people": "contact", "organizations": "contact", "tasks": "task", "activities": "activity", "projects": "project"}[kind]
	var q search.Query
	if scope != "" {
		q, err = search.ParseScope(scope, term)
		if err != nil {
			return nil, invalid(err.Error())
		}
		for _, f := range q.Filters {
			if f.Key == "is" && f.Value == "deleted" {
				return nil, invalid("Deleted records are not available in graphical collections.")
			}
		}
	}
	ctx := r.Context()
	entity, project := params.Get("entity"), params.Get("project")
	for _, id := range []string{entity, project} {
		if id != "" && !validID(id) {
			return nil, invalid("Related filters require a record ID.")
		}
	}
	sortBy := params.Get("sort")
	switch kind {
	case "people", "organizations":
		if sortBy != "" && !slices.Contains([]string{"name", "updated", "added", "organization"}, sortBy) {
			return nil, invalid("Choose a supported contact sort order.")
		}
		rows, err := s.store.List(ctx, entityKind(kind), q, sortBy)
		return window(rows, offset, limit, err)
	case "tasks":
		view := params.Get("view")
		switch view {
		case "", "all", "inbox", "open", "today", "upcoming", "waiting":
		case "overdue", "completed", "unscheduled", "scheduled":
			key, value := "is", "overdue"
			switch view {
			case "completed":
				key, value = "status", "completed"
			case "scheduled":
				key, value = "status", "scheduled"
			case "unscheduled":
				key, value = "due", "none"
				view = "open"
			}
			q.Filters = append(q.Filters, search.Filter{Key: key, Value: value})
		default:
			return nil, invalid("Choose a supported commitments view.")
		}
		if sortBy != "" && !slices.Contains([]string{"due", "priority", "title", "updated", "created"}, sortBy) {
			return nil, invalid("Choose a supported commitments sort order.")
		}
		rows, err := s.store.Tasks(ctx, app.TaskFilter{Query: q, View: view, Sort: sortBy, EntityID: entity, ProjectID: project, Limit: fetch})
		return window(rows, offset, limit, err)
	case "activities":
		rows, err := s.store.Activities(ctx, app.ActivityFilter{Query: q, EntityID: entity, ProjectID: project, Limit: fetch})
		for i := range rows {
			redactActivity(&rows[i])
		}
		return window(rows, offset, limit, err)
	case "projects":
		rows, err := s.store.Projects(ctx, "", q)
		return window(rows, offset, limit, err)
	case "pipelines":
		rows, err := s.store.Pipelines(ctx, false)
		if term != "" {
			match := strings.ToLower(strings.TrimSpace(term))
			rows = slices.DeleteFunc(rows, func(v domain.Pipeline) bool { return !strings.Contains(strings.ToLower(v.Name), match) })
		}
		return window(rows, offset, limit, err)
	}
	return nil, notFound()
}

type contactDetail struct {
	Record  domain.Contact  `json:"record"`
	Dossier *domain.Dossier `json:"dossier"`
}

type pipelineDetail struct {
	Record domain.Pipeline       `json:"record"`
	Items  []domain.PipelineItem `json:"items"`
}

func redactActivity(a *domain.Activity) {
	// A connector reference can contain routing information; this UI does not use it.
	a.ExternalURL, a.ExternalRef = "", ""
}

func (s Server) contact(ctx context.Context, kind, id string) (domain.Contact, error) {
	c, err := s.store.Contact(ctx, id)
	if err == nil && (c.Deleted || c.Kind != entityKind(kind)) {
		return c, notFound()
	}
	return c, err
}

func (s Server) record(r *http.Request, kind, id string) (any, error) {
	ctx := r.Context()
	switch kind {
	case "people", "organizations":
		c, err := s.contact(ctx, kind, id)
		if err != nil {
			return nil, err
		}
		c.Extensions = nil // opaque extension data belongs to its own access boundary
		out := contactDetail{Record: c}
		if !c.Archived {
			d, err := s.store.Dossier(ctx, id)
			if err != nil {
				return nil, err
			}
			for i := range d.Recent {
				redactActivity(&d.Recent[i])
			}
			out.Dossier = &d
		}
		return out, nil
	case "tasks":
		t, err := s.store.Task(ctx, id)
		if err == nil && t.Deleted {
			return nil, notFound()
		}
		return t, err
	case "projects":
		p, err := s.store.Project(ctx, id)
		if err == nil && p.Deleted {
			return nil, notFound()
		}
		return p, err
	case "activities":
		a, err := s.store.Activity(ctx, id)
		if err == nil && a.Deleted {
			return nil, notFound()
		}
		redactActivity(&a)
		return a, err
	case "pipelines":
		p, err := s.pipeline(ctx, id)
		if err != nil {
			return nil, err
		}
		q, err := search.ParseScope("item", r.URL.Query().Get("q"))
		if err != nil {
			return nil, invalid(err.Error())
		}
		if q.Filters != nil {
			for _, f := range q.Filters {
				if f.Key == "is" && f.Value == "deleted" {
					return nil, invalid("Deleted records are not available on this board.")
				}
			}
		}
		items, err := s.store.Items(ctx, id, q)
		return pipelineDetail{p, items[:min(len(items), collectionWindow)]}, err
	}
	return nil, notFound()
}

func (s Server) pipeline(ctx context.Context, id string) (domain.Pipeline, error) {
	list, err := s.store.Pipelines(ctx, false)
	if err != nil {
		return domain.Pipeline{}, err
	}
	for _, p := range list {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.Pipeline{}, notFound()
}
