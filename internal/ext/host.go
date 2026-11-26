package ext

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

// Session is the host API as seen by one extension for one invocation. Every
// method checks the extension's granted permissions first (default deny) and
// then goes through the same application repository, validation and revision
// checks the UI and CLI use. Extensions never receive a database handle.
type Session struct {
	ID           string
	Granted      PermissionSet
	Repo         app.CRM // nil: the host has no records to serve (denied as unavailable)
	Storage      *Storage
	Config       map[string]any
	Entitlement  string // capability to re-check on every call, if the manifest declares one
	Entitled     EntitlementProvider
	Bus          *Bus              // where events.publish goes; nil drops them
	Published    map[string]string // declared event type -> entity type
	durableTypes []string
	published    int
	mcpOnly      bool
}

// maxPublishPerCall bounds events one request may publish (an import, say).
const maxPublishPerCall = 1000

func perr(code, format string, a ...any) *ProtocolError {
	return &ProtocolError{Code: code, Message: fmt.Sprintf(format, a...)}
}

func (s *Session) need(perm string) *ProtocolError {
	if !s.Granted.Has(perm) {
		return perr(CodePermissionDenied, "permission %s is not granted", perm)
	}
	return nil
}

func decode(data json.RawMessage, v any) *ProtocolError {
	if len(bytes.TrimSpace(data)) == 0 {
		data = json.RawMessage("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return perr(CodeInvalid, "invalid arguments: %v", err)
	}
	return nil
}

func mapErr(err error) *ProtocolError {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sqlite.ErrConflict):
		return perr(CodeConflict, "the record changed; reload and retry")
	case errors.Is(err, ErrQuota):
		return perr(CodeQuota, "%v", err)
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return perr(CodeInternal, "operation was cancelled")
	}
	msg := err.Error()
	if strings.Contains(strings.ToLower(msg), "not found") || strings.Contains(msg, "no rows") {
		return perr(CodeNotFound, "not found")
	}
	return perr(CodeInvalid, "%s", Clean(msg, 200, false))
}

func limitOf(n, def, max int) int {
	if n <= 0 {
		n = def
	}
	return min(n, max)
}

// Call dispatches one host_call. It is the HostFunc given to Invoke.
func (s *Session) Call(ctx context.Context, method string, data json.RawMessage) (any, *ProtocolError) {
	if s.mcpOnly {
		switch method {
		case "entitlement.check", "permissions.check", "records.rule-page", "records.resolve",
			"attention.list", "contacts.engagement", "contacts.list", "contacts.get",
			"contacts.connections", "activities.list", "tasks.list", "projects.list", "pipelines.list":
		default:
			return nil, perr(CodePermissionDenied, "callback not available to MCP")
		}
	}
	if s.Entitlement != "" {
		d, err := s.Entitled.Check(ctx, s.Entitlement)
		if err != nil || !d.Allowed {
			return nil, perr(CodeEntitlement, "capability %s is not entitled", s.Entitlement)
		}
	}
	if s.Repo == nil && !strings.HasPrefix(method, "storage.") && method != "config.get" && method != "entitlement.check" && method != "events.publish" && method != "permissions.check" {
		return nil, perr(CodeUnknownMethod, "records are not available")
	}
	ctx = app.WithEventOrigin(ctx, app.EventOrigin{Source: ExtensionSource + s.ID})
	switch method {
	case "config.get":
		return s.Config, nil
	case "entitlement.check":
		var a struct {
			Capability string `json:"capability"`
		}
		if e := decode(data, &a); e != nil {
			return nil, e
		}
		if !capPattern.MatchString(a.Capability) || s.Entitled == nil {
			return nil, perr(CodeInvalid, "capability is malformed")
		}
		d, err := s.Entitled.Check(ctx, a.Capability)
		if err != nil {
			d = EntitlementDecision{Capability: a.Capability, Reason: "decision unavailable", Source: "error"}
		}
		return d, nil
	case "permissions.check":
		return s.permissions(data)
	case "records.rule-page":
		return s.ruleRecords(ctx, data)
	case "records.rule-write":
		return s.ruleWrite(ctx, data)
	case "attention.list", "attention.act", "tasks.complete", "contacts.engagement":
		return s.attention(ctx, method, data)
	case "events.next", "events.settle", "events.dead", "events.replay":
		return s.eventJournal(ctx, method, data)
	case "events.publish":
		return s.publish(data)
	case "records.resolve":
		return s.resolve(ctx, data)
	case "storage.get", "storage.put", "storage.delete", "storage.keys":
		return s.storage(method, data)
	case "contacts.list":
		var a struct {
			Query string `json:"query"`
			Kind  string `json:"kind"`
			Limit int    `json:"limit"`
		}
		if e := decode(data, &a); e != nil {
			return nil, e
		}
		kind := a.Kind
		if kind == "" {
			kind = "person"
		}
		perm := map[string]string{"person": "contacts.read", "organization": "organizations.read"}[kind]
		if perm == "" {
			return nil, perr(CodeInvalid, "kind must be person or organization")
		}
		if e := s.need(perm); e != nil {
			return nil, e
		}
		q, err := search.Parse(a.Query)
		if err != nil {
			return nil, perr(CodeInvalid, "%s", Clean(err.Error(), 200, false))
		}
		list, err := s.Repo.List(ctx, kind, q, "name")
		if err != nil {
			return nil, mapErr(err)
		}
		return list[:min(len(list), limitOf(a.Limit, 100, 500))], nil
	case "contacts.get", "contacts.connections":
		var a struct {
			ID string `json:"id"`
		}
		if e := decode(data, &a); e != nil {
			return nil, e
		}
		if method == "contacts.connections" {
			if e := s.need("relationships.read"); e != nil {
				return nil, e
			}
		}
		// Deny before touching storage so "not found" cannot be used to probe ids.
		if !s.Granted.Has("contacts.read") && !s.Granted.Has("organizations.read") {
			return nil, perr(CodePermissionDenied, "permission contacts.read is not granted")
		}
		c, err := s.Repo.Contact(ctx, a.ID)
		if err != nil {
			return nil, mapErr(err)
		}
		perm := "contacts.read"
		if c.Kind == "organization" {
			perm = "organizations.read"
		}
		if e := s.need(perm); e != nil { // checked after the read; nothing is returned on denial
			return nil, e
		}
		if method == "contacts.connections" {
			out, err := s.Repo.Connections(ctx, a.ID)
			if err != nil {
				return nil, mapErr(err)
			}
			return out, nil
		}
		return c, nil
	case "contacts.save":
		var c domain.Contact
		if e := decode(data, &c); e != nil {
			return nil, e
		}
		perm := "contacts.write"
		if c.Kind == "organization" {
			perm = "organizations.write"
		}
		if e := s.need(perm); e != nil {
			return nil, e
		}
		saved, err := s.Repo.SaveContacts(ctx, []domain.Contact{c}, ExtensionSource+s.ID)
		if err != nil {
			return nil, mapErr(err)
		}
		return saved[0], nil
	case "activities.list":
		if e := s.need("activities.read"); e != nil {
			return nil, e
		}
		var a struct {
			Entity string `json:"entity_id"`
			Query  string `json:"query"`
			Before string `json:"before"`
			Limit  int    `json:"limit"`
		}
		if e := decode(data, &a); e != nil {
			return nil, e
		}
		q, err := search.ParseScope("activity", a.Query)
		if err != nil {
			return nil, perr(CodeInvalid, "%s", Clean(err.Error(), 200, false))
		}
		f := app.ActivityFilter{Query: q, EntityID: a.Entity, Limit: limitOf(a.Limit, 100, 500)}
		if a.Before != "" {
			t, err := time.Parse(time.RFC3339, a.Before)
			if err != nil {
				return nil, perr(CodeInvalid, "before must be RFC 3339")
			}
			f.Before = t
		}
		out, err := s.Repo.Activities(ctx, f)
		if err != nil {
			return nil, mapErr(err)
		}
		return out, nil
	case "activities.save":
		if e := s.need("activities.write"); e != nil {
			return nil, e
		}
		var a domain.Activity
		if e := decode(data, &a); e != nil {
			return nil, e
		}
		a.Provenance, a.SourceApp = domain.Local, ExtensionSource+s.ID // editable in Community; the source app records who wrote it
		out, err := s.Repo.SaveActivity(ctx, a)
		if err != nil {
			return nil, mapErr(err)
		}
		return out, nil
	case "tasks.list":
		if e := s.need("tasks.read"); e != nil {
			return nil, e
		}
		var a struct {
			View   string `json:"view"`
			Entity string `json:"entity_id"`
			Limit  int    `json:"limit"`
		}
		if e := decode(data, &a); e != nil {
			return nil, e
		}
		out, err := s.Repo.Tasks(ctx, app.TaskFilter{View: a.View, EntityID: a.Entity, Limit: limitOf(a.Limit, 100, 500)})
		if err != nil {
			return nil, mapErr(err)
		}
		return out, nil
	case "tasks.save":
		if e := s.need("tasks.write"); e != nil {
			return nil, e
		}
		var t domain.Task
		if e := decode(data, &t); e != nil {
			return nil, e
		}
		out, err := s.Repo.SaveTask(ctx, t)
		if err != nil {
			return nil, mapErr(err)
		}
		return out, nil
	case "projects.save":
		if e := s.need("projects.write"); e != nil {
			return nil, e
		}
		var p domain.Project
		if e := decode(data, &p); e != nil {
			return nil, e
		}
		out, err := s.Repo.SaveProject(ctx, p)
		if err != nil {
			return nil, mapErr(err)
		}
		return out, nil
	case "projects.list":
		if e := s.need("projects.read"); e != nil {
			return nil, e
		}
		var a struct {
			Status string `json:"status"`
		}
		if e := decode(data, &a); e != nil {
			return nil, e
		}
		out, err := s.Repo.Projects(ctx, a.Status, search.Query{})
		if err != nil {
			return nil, mapErr(err)
		}
		return out, nil
	case "pipelines.list":
		if e := s.need("pipelines.read"); e != nil {
			return nil, e
		}
		out, err := s.Repo.Pipelines(ctx, false)
		if err != nil {
			return nil, mapErr(err)
		}
		return out, nil
	}
	return nil, perr(CodeUnknownMethod, "unknown host method %q", method)
}

func (s *Session) storage(method string, data json.RawMessage) (any, *ProtocolError) {
	if e := s.need("storage.extension"); e != nil {
		return nil, e
	}
	if s.Storage == nil {
		return nil, perr(CodeUnknownMethod, "storage is not available")
	}
	var a struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if e := decode(data, &a); e != nil {
		return nil, e
	}
	switch method {
	case "storage.get":
		v, ok, err := s.Storage.Get(a.Key)
		if err != nil {
			return nil, mapErr(err)
		}
		return map[string]any{"found": ok, "value": v}, nil
	case "storage.put":
		if err := s.Storage.Put(a.Key, a.Value); err != nil {
			return nil, mapErr(err)
		}
	case "storage.delete":
		if err := s.Storage.Delete(a.Key); err != nil {
			return nil, mapErr(err)
		}
	case "storage.keys":
		k, err := s.Storage.Keys()
		if err != nil {
			return nil, mapErr(err)
		}
		return k, nil
	}
	return map[string]any{}, nil
}

// publish emits a declared, extension-namespaced event. The payload is
// identifiers only; the host stamps the source so it cannot be forged, and
// extension-sourced events are never delivered back to extensions.
func (s *Session) publish(data json.RawMessage) (any, *ProtocolError) {
	if e := s.need("events.publish"); e != nil {
		return nil, e
	}
	var a struct {
		Type          string `json:"type"`
		EntityID      string `json:"entity_id"`
		Revision      int    `json:"revision"`
		CorrelationID string `json:"correlation_id"`
	}
	if e := decode(data, &a); e != nil {
		return nil, e
	}
	entity, ok := s.Published[a.Type]
	if !ok {
		return nil, perr(CodePermissionDenied, "event %s is not declared in the manifest", a.Type)
	}
	if !idToken.MatchString(a.EntityID) || len(a.CorrelationID) > 64 {
		return nil, perr(CodeInvalid, "entity_id or correlation_id is malformed")
	}
	if s.published++; s.published > maxPublishPerCall {
		return nil, perr(CodeQuota, "too many events in one request")
	}
	if s.Bus != nil {
		e := NewEvent(a.Type, a.EntityID, a.Revision, ExtensionSource+s.ID, Clean(a.CorrelationID, 64, false))
		e.EntityType = entity
		s.Bus.Publish(e)
	}
	return map[string]any{}, nil
}

// Record is a minimal projection of a core record for display and integrity
// checks. It is deliberately smaller than the record itself.
type Record struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Name       string   `json:"name,omitempty"`
	Detail     string   `json:"detail,omitempty"` // primary email, organization, status…
	Status     string   `json:"status,omitempty"`
	Due        string   `json:"due,omitempty"`
	When       string   `json:"when,omitempty"`
	Updated    string   `json:"updated,omitempty"`
	Created    string   `json:"created,omitempty"`
	URL        string   `json:"url,omitempty"`
	Revision   int      `json:"revision,omitempty"`
	Priority   int      `json:"priority,omitempty"`
	Tags       []string `json:"tags,omitzero"`
	Archived   bool     `json:"archived,omitempty"`
	Deleted    bool     `json:"deleted,omitempty"`
	Missing    bool     `json:"missing,omitempty"`
	MergedInto string   `json:"merged_into,omitempty"`
}

const maxResolve = 500

// resolve looks up many core records in one host call (no per-record process
// round trips). It needs the read permission for the kind.
func (s *Session) resolve(ctx context.Context, data json.RawMessage) (any, *ProtocolError) {
	var a struct {
		Kind string   `json:"kind"`
		IDs  []string `json:"ids"`
	}
	if e := decode(data, &a); e != nil {
		return nil, e
	}
	perm := map[string]string{"contact": "contacts.read", "organization": "organizations.read", "task": "tasks.read", "activity": "activities.read", "project": "projects.read", "followup": "contacts.read"}[a.Kind]
	if perm == "" {
		return nil, perr(CodeInvalid, "unsupported record kind")
	}
	if e := s.need(perm); e != nil {
		return nil, e
	}
	if a.Kind == "followup" {
		if e := s.need("organizations.read"); e != nil {
			return nil, e
		}
	}
	if len(a.IDs) > maxResolve {
		return nil, perr(CodeInvalid, "at most %d ids", maxResolve)
	}
	out := make([]Record, 0, len(a.IDs))
	for _, id := range a.IDs {
		r := Record{ID: id, Kind: a.Kind}
		var err error
		switch a.Kind {
		case "contact", "organization":
			var c domain.Contact
			if c, err = s.Repo.Contact(ctx, id); err == nil {
				if (c.Kind == "organization") != (a.Kind == "organization") {
					r = Record{ID: id, Kind: a.Kind, Missing: true} // wrong kind: do not disclose
					break
				}
				r.Name, r.Archived, r.Deleted, r.Revision = c.DisplayName, c.Archived, c.Deleted, c.Revision
				r.Status, r.Tags = "active", c.Tags
				if r.Tags == nil {
					r.Tags = []string{}
				}
				if c.Archived {
					r.Status = "archived"
				}
				for _, m := range c.Methods {
					if m.Kind == "email" && (r.Detail == "" || m.Preferred) {
						r.Detail = m.Value
					}
				}
				if c.Deleted {
					var into string
					if raw, ok := c.Extensions["merged_into"]; ok && json.Unmarshal(raw, &into) == nil {
						r.MergedInto = into
					}
				}
				if r.Detail == "" && len(c.Memberships) > 0 {
					r.Detail = c.Memberships[0].Organization
				}
			}
		case "task":
			var t domain.Task
			if t, err = s.Repo.Task(ctx, id); err == nil {
				r.Name, r.Status, r.Due, r.Archived, r.Deleted, r.Revision = t.Title, t.Status, t.Due, t.Archived, t.Deleted, t.Revision
				r.Updated, r.Created = t.UpdatedAt, t.CreatedAt
				r.Priority = t.Priority
			}
		case "activity":
			var ac domain.Activity
			if ac, err = s.Repo.Activity(ctx, id); err == nil {
				r.Name, r.Detail, r.Deleted, r.Revision, r.URL = ac.Title, ac.Type, ac.Deleted, ac.Revision, ac.ExternalURL
				r.When = ac.OccurredAt.Format("2006-01-02")
				r.Status = "recorded"
			}
		case "project":
			var p domain.Project
			if p, err = s.Repo.Project(ctx, id); err == nil {
				r.Name, r.Status, r.Deleted, r.Revision = p.Name, p.Status, p.Deleted, p.Revision
			}
		case "followup":
			var c domain.Contact
			c, err = s.Repo.Contact(ctx, id)
			if err != nil || c.Archived || c.Deleted {
				r.Missing = true
				break
			}
			var f domain.FollowUp
			f, err = s.Repo.FollowUp(ctx, id)
			r.Name, r.Due, r.Revision = c.DisplayName, f.Due, c.Revision
			r.Detail = "contact"
			if c.Kind == "organization" {
				r.Detail = c.Kind
			}
			r.Status = app.FollowUpState(f.Never, f.Due, time.Now().Format(time.DateOnly))
		}
		if err != nil {
			r = Record{ID: id, Kind: a.Kind, Missing: true}
		}
		out = append(out, r)
	}
	return out, nil
}
