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
	ID          string
	Granted     PermissionSet
	Repo        app.CRM // nil: the host has no records to serve (denied as unavailable)
	Storage     *Storage
	Config      map[string]any
	Entitlement string // capability to re-check on every call, if the manifest declares one
	Entitled    EntitlementProvider
}

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
	if s.Entitlement != "" {
		d, err := s.Entitled.Check(ctx, s.Entitlement)
		if err != nil || !d.Allowed {
			return nil, perr(CodeEntitlement, "capability %s is not entitled", s.Entitlement)
		}
	}
	if s.Repo == nil && !strings.HasPrefix(method, "storage.") && method != "config.get" {
		return nil, perr(CodeUnknownMethod, "records are not available")
	}
	switch method {
	case "config.get":
		return s.Config, nil
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
		a.Provenance, a.SourceApp = "extension", s.ID
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
