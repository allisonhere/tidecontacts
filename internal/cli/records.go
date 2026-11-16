package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
	"github.com/allisonhere/tidecontacts/internal/transfer"
)

var actions = map[string]string{
	"contacts":      "list get create update delete restore history dossier connections query",
	"organizations": "list get create update delete restore history dossier connections",
	"tasks":         "list get create update delete restore complete reopen duplicate",
	"activities":    "list get create update delete restore",
	"projects":      "list get create update delete restore",
	"pipelines":     "list get create update archive",
	"items":         "list get create update delete restore move",
	"crm":           "export import",
}

func validateCommand(resource, action, id string, o options) error {
	allowed := map[string]bool{"demo": true, "database": true, "json": true, "help": true}
	if action == "create" || action == "update" || action == "import" {
		allowed["input"] = true
	}
	if action == "delete" || action == "archive" || action == "restore" {
		allowed["yes"] = true
	}
	if resource == "pipelines" && (action == "update" || action == "archive") {
		allowed["if-match"] = true
	}
	if resource == "contacts" && (action == "list" || action == "create") {
		allowed["kind"] = true
	}
	if action == "list" {
		if resource != "pipelines" {
			allowed["query"] = true
		}
		switch resource {
		case "contacts", "organizations":
			allowed["full"] = true
		case "tasks":
			allowed["view"] = true
			allowed["entity"] = true
			allowed["limit"] = true
		case "activities":
			allowed["entity"] = true
			allowed["limit"] = true
		case "projects":
			allowed["status"] = true
		case "items":
			allowed["pipeline"] = true
		}
	}
	if resource == "items" && action == "move" {
		allowed["stage"] = true
	}
	if resource == "contacts" && action == "query" {
		allowed["query"] = true
		allowed["format"] = true
		allowed["limit"] = true
		allowed["kind"] = true
	}
	for key := range o.present {
		if !allowed[key] {
			return usage("--" + key + " is not supported for " + resource + " " + action)
		}
	}
	if o.view != "" && !strings.Contains(" inbox today upcoming waiting open all ", " "+o.view+" ") {
		return usage("unknown task view " + o.view)
	}
	if o.status != "" && !strings.Contains(" all planned active on-hold completed archived ", " "+o.status+" ") {
		return usage("unknown project status " + o.status)
	}
	supported := false
	for _, a := range splitActions(resource) {
		supported = supported || action == a
	}
	if !supported {
		return usage("unsupported action " + resource + " " + action)
	}
	needsID := action != "query" && action != "list" && action != "create" && action != "export" && action != "import"
	if needsID && id == "" {
		return usage("this action requires an ID")
	}
	if !needsID && id != "" {
		return usage("this action does not accept an ID")
	}
	needsInput := action == "create" || action == "update" || action == "import"
	if needsInput && o.input == "" {
		return usage("this action requires --input FILE or --input -")
	}
	if !needsInput && o.input != "" {
		return usage("--input is not accepted for this action")
	}
	if (action == "delete" || action == "archive" || action == "restore") && !o.yes {
		return usage("this action requires --yes")
	}
	if resource == "contacts" && action == "query" && o.format == "aerc" && o.limit > 99 {
		return usage("aerc --limit must be at most 99 to fit its completion reader")
	}
	if o.limit < 1 || o.limit > 5000 {
		return usage("--limit must be between 1 and 5000")
	}
	if resource == "activities" && o.limit > 1000 {
		return usage("activity --limit must be at most 1000")
	}
	if o.kind != "person" && o.kind != "organization" {
		return usage("--kind must be person or organization")
	}
	if action == "move" && o.stage == "" {
		return usage("items move requires --stage ID")
	}
	if resource == "pipelines" && (action == "update" || action == "archive") && o.match == "" {
		return usage("pipeline edits require --if-match from pipelines get's etag")
	}
	if o.format != "json" && o.format != "aerc" && o.format != "neomutt" {
		return usage("--format must be json, aerc or neomutt")
	}
	return nil
}
func splitActions(resource string) []string { return strings.Fields(actions[resource]) }

func execute(ctx context.Context, s *sqlite.Store, r, a, id string, o options, input []byte) (any, int, error) {
	if r == "crm" {
		if a == "export" {
			v, e := s.ExportCRM(ctx)
			return v, 0, e
		}
		archive, e := transfer.ReadArchive(input)
		if e != nil {
			return nil, 0, invalid(e.Error())
		}
		v, e := s.ImportCRM(ctx, archive)
		return v, 0, e
	}
	if a == "query" {
		v, e := recipientQuery(ctx, s, o)
		return v, o.limit, e
	}
	if a == "list" {
		return listRecords(ctx, s, r, o)
	}
	if a == "create" {
		v := newRecord(r, o.kind)
		if r == "organizations" {
			v = newRecord(r, "organization")
		}
		if e := decodeRecord(input, v, nil, true); e != nil {
			return nil, 0, e
		}
		value, e := saveRecord(ctx, s, r, v, nil)
		return value, 0, writeError(e)
	}
	existing, e := getRecord(ctx, s, r, id)
	if e != nil {
		return nil, 0, e
	}
	switch a {
	case "get":
		return existing, 0, nil
	case "history":
		v, e := s.History(ctx, id)
		return v, 0, e
	case "dossier":
		v, e := s.Dossier(ctx, id)
		return v, 0, e
	case "connections":
		v, e := s.Connections(ctx, id)
		return v, 0, e
	case "update":
		v := reflect.New(reflect.TypeOf(existing)).Interface()
		if e := decodeRecord(input, v, existing, false); e != nil {
			return nil, 0, e
		}
		if r == "pipelines" && o.match != pipelineETag(existing.(domain.Pipeline)) {
			return nil, 0, fail(5, "conflict", "pipeline changed; reload before editing")
		}
		value, e := saveRecord(ctx, s, r, v, existing)
		return value, 0, writeError(e)
	case "archive":
		p := existing.(domain.Pipeline)
		if o.match != pipelineETag(p) {
			return nil, 0, fail(5, "conflict", "pipeline changed; reload before archiving")
		}
		updated := p
		updated.Archived = true
		v, e := s.SavePipelineIfUnchanged(ctx, updated, p)
		return v, 0, writeError(e)
	case "complete":
		v, e := s.CompleteTask(ctx, id)
		return v, 0, writeError(e)
	case "reopen":
		v, e := s.ReopenTask(ctx, id)
		return v, 0, writeError(e)
	case "duplicate":
		v, e := s.DuplicateTask(ctx, id)
		return v, 0, writeError(e)
	case "move":
		e := s.MoveItem(ctx, id, o.stage)
		if e != nil {
			return nil, 0, writeError(e)
		}
		v, e := s.Item(ctx, id)
		return v, 0, e
	case "delete", "restore":
		if r == "contacts" || r == "organizations" {
			c := existing.(domain.Contact)
			c.Deleted = a == "delete"
			if a == "restore" {
				c.Archived = false
			}
			saved, e := s.SaveContacts(ctx, []domain.Contact{c}, "CLI "+a)
			if e != nil {
				return nil, 0, writeError(e)
			}
			return saved[0], 0, nil
		}
		if a == "restore" {
			kind := map[string]string{"tasks": "task", "activities": "activity", "projects": "project", "items": "item"}[r]
			e = s.Restore(ctx, kind, id)
		} else {
			switch r {
			case "tasks":
				e = s.DeleteTask(ctx, id)
			case "activities":
				e = s.DeleteActivity(ctx, id)
			case "projects":
				e = s.DeleteProject(ctx, id)
			case "items":
				e = s.DeleteItem(ctx, id)
			}
		}
		if e != nil {
			return nil, 0, writeError(e)
		}
		v, e := getRecord(ctx, s, r, id)
		return v, 0, e
	}
	return nil, 0, usage("unsupported action")
}
func listRecords(ctx context.Context, s *sqlite.Store, r string, o options) (any, int, error) {
	scope := map[string]string{"contacts": "contact", "organizations": "contact", "tasks": "task", "activities": "activity", "projects": "project", "items": "item"}[r]
	var q search.Query
	var e error
	if scope != "" {
		q, e = search.ParseScope(scope, o.query)
		if e != nil {
			return nil, 0, invalid(e.Error())
		}
	}
	switch r {
	case "contacts", "organizations":
		kind := o.kind
		if r == "organizations" {
			kind = "organization"
		}
		v, e := s.List(ctx, kind, q, "name")
		if e != nil {
			return nil, 0, e
		}
		if o.full {
			full := []domain.Contact{}
			for _, row := range v {
				c, e := s.Contact(ctx, row.ID)
				if e != nil {
					return nil, 0, e
				}
				full = append(full, c)
			}
			return full, 0, nil
		}
		summaries := []contactSummary{}
		for _, row := range v {
			summaries = append(summaries, contactSummary{ID: row.ID, Kind: row.Kind, Name: row.Name, Organization: row.Organization, Tags: row.Tags, UpdatedAt: row.UpdatedAt, CreatedAt: row.CreatedAt, Favorite: row.Favorite, Archived: row.Archived, Revision: row.Revision})
		}
		return summaries, 0, nil
	case "tasks":
		v, e := s.Tasks(ctx, app.TaskFilter{Query: q, View: o.view, EntityID: o.entity, Limit: o.limit})
		return v, o.limit, e
	case "activities":
		v, e := s.Activities(ctx, app.ActivityFilter{Query: q, EntityID: o.entity, Limit: o.limit})
		return v, o.limit, e
	case "projects":
		v, e := s.Projects(ctx, o.status, q)
		return v, 2000, e
	case "pipelines":
		v, e := s.Pipelines(ctx, true)
		return v, 0, e
	case "items":
		v, e := s.Items(ctx, o.pipeline, q)
		return v, 5000, e
	}
	return nil, 0, usage("resource has no list action")
}
func getRecord(ctx context.Context, s *sqlite.Store, r, id string) (any, error) {
	switch r {
	case "contacts", "organizations":
		c, e := s.Contact(ctx, id)
		if e == nil && r == "organizations" && c.Kind != "organization" {
			return nil, sql.ErrNoRows
		}
		return c, e
	case "tasks":
		return s.Task(ctx, id)
	case "activities":
		return s.Activity(ctx, id)
	case "projects":
		return s.Project(ctx, id)
	case "items":
		return s.Item(ctx, id)
	case "pipelines":
		list, e := s.Pipelines(ctx, true)
		if e != nil {
			return nil, e
		}
		for _, p := range list {
			if p.ID == id {
				return p, nil
			}
		}
		return nil, sql.ErrNoRows
	}
	return nil, usage("unsupported resource")
}
func newRecord(r, kind string) any {
	switch r {
	case "contacts", "organizations":
		c := domain.NewContact(kind)
		return &c
	case "tasks":
		return &domain.Task{}
	case "activities":
		return &domain.Activity{}
	case "projects":
		return &domain.Project{}
	case "pipelines":
		return &domain.Pipeline{}
	case "items":
		return &domain.PipelineItem{}
	}
	return nil
}
func saveRecord(ctx context.Context, s *sqlite.Store, r string, v, previous any) (any, error) {
	switch value := v.(type) {
	case *domain.Contact:
		if r == "organizations" && value.Kind != "organization" {
			return nil, invalid("organizations must have Kind organization")
		}
		value.Normalize()
		if e := value.Validate(); e != nil {
			return nil, invalid(e.Error())
		}
		list, e := s.SaveContacts(ctx, []domain.Contact{*value}, "CLI save")
		if e != nil {
			return nil, e
		}
		return list[0], nil
	case *domain.Task:
		value.Normalize()
		if e := value.Validate(); e != nil {
			return nil, invalid(e.Error())
		}
		return s.SaveTask(ctx, *value)
	case *domain.Activity:
		if previous == nil && value.Provenance != "" && value.Provenance != domain.Local {
			return nil, invalid("create local activities here; external activities belong to connectors")
		}
		value.Normalize(time.Now())
		if e := value.Validate(); e != nil {
			return nil, invalid(e.Error())
		}
		return s.SaveActivity(ctx, *value)
	case *domain.Project:
		allowed := map[string]bool{}
		if old, ok := previous.(domain.Project); ok {
			for _, m := range old.Milestones {
				allowed[m.ID] = true
			}
		}
		for _, m := range value.Milestones {
			if m.ID != "" && !allowed[m.ID] {
				return nil, invalid("new milestones must omit ID")
			}
		}
		value.Normalize()
		if e := value.Validate(); e != nil {
			return nil, invalid(e.Error())
		}
		return s.SaveProject(ctx, *value)
	case *domain.Pipeline:
		allowed := map[string]bool{}
		if old, ok := previous.(domain.Pipeline); ok {
			for _, stage := range old.Stages {
				allowed[stage.ID] = true
			}
		}
		for _, stage := range value.Stages {
			if stage.ID != "" && !allowed[stage.ID] {
				return nil, invalid("new stages must omit ID")
			}
		}
		var p domain.Pipeline
		var e error
		if old, ok := previous.(domain.Pipeline); ok {
			p, e = s.SavePipelineIfUnchanged(ctx, *value, old)
		} else {
			p, e = s.SavePipeline(ctx, *value)
		}
		if e != nil {
			return nil, e
		}
		return getRecord(ctx, s, "pipelines", p.ID)
	case *domain.PipelineItem:
		return s.SaveItem(ctx, *value)
	}
	return nil, usage("unsupported record type")
}
func writeError(err error) error {
	if err == nil {
		return nil
	}
	var structured *Error
	if errors.As(err, &structured) || errors.Is(err, sqlite.ErrConflict) || errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var driver interface{ Code() int }
	if errors.As(err, &driver) {
		return err
	}
	return invalid(err.Error())
}

// decodeRecord applies a top-level partial update. Arrays and nested objects are
// explicit replacements, preserving every omitted aggregate field.
func decodeRecord(raw []byte, destination, previous any, create bool) error {
	var fields map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(raw))
	if e := dec.Decode(&fields); e != nil {
		return invalid("expected a JSON object: " + e.Error())
	}
	if fields == nil {
		return invalid("expected a JSON object")
	}
	if e := dec.Decode(new(any)); e != io.EOF {
		return invalid("input must contain exactly one JSON object")
	}
	t := reflect.TypeOf(destination).Elem()
	known := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Tag.Get("json") == "-" {
			continue
		}
		known[f.Name] = true
	}
	for name, value := range fields {
		if !known[name] {
			return invalid("unknown field " + name + "; see schema")
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return invalid("null is not supported for " + name + "; use a typed zero value")
		}
	}
	if create {
		for _, key := range []string{"ID", "Revision", "CreatedAt", "UpdatedAt"} {
			if value, ok := fields[key]; ok && string(value) != "\"\"" && string(value) != "0" {
				return invalid("create must omit " + key)
			}
		}
	} else {
		encoded, _ := json.Marshal(previous)
		var before map[string]json.RawMessage
		_ = json.Unmarshal(encoded, &before)
		if _, hasRevision := t.FieldByName("Revision"); hasRevision {
			value, ok := fields["Revision"]
			if !ok {
				return invalid("update requires the current Revision")
			}
			var revision int
			if json.Unmarshal(value, &revision) != nil {
				return invalid("Revision must be an integer")
			}
			var current int
			_ = json.Unmarshal(before["Revision"], &current)
			if revision != current {
				return fail(5, "conflict", "revision changed; get the record again")
			}
		}
		for _, key := range []string{"ID", "Kind", "CreatedAt", "UpdatedAt"} {
			if supplied, ok := fields[key]; ok {
				var a, b any
				_ = json.Unmarshal(supplied, &a)
				_ = json.Unmarshal(before[key], &b)
				if !reflect.DeepEqual(a, b) {
					return invalid(key + " cannot be changed")
				}
			}
		}
		for key, value := range fields {
			before[key] = value
		}
		fields = before
	}
	merged, e := json.Marshal(fields)
	if e != nil {
		return invalid(e.Error())
	}
	strict := json.NewDecoder(bytes.NewReader(merged))
	strict.DisallowUnknownFields()
	if e = strict.Decode(destination); e != nil {
		return invalid(e.Error())
	}
	return nil
}
func schema(resource string) (any, error) {
	descriptions := map[string]any{}
	for _, r := range []string{"contacts", "organizations", "tasks", "activities", "projects", "pipelines", "items"} {
		if resource != "" && resource != r {
			continue
		}
		v := newRecord(r, "person")
		descriptions[r] = map[string]any{"actions": splitActions(r), "fields": describe(reflect.TypeOf(v).Elem()), "update": "partial top-level object; arrays/nested objects replace; Revision required except pipelines (--if-match)"}
	}
	if details, ok := descriptions["contacts"].(map[string]any); ok {
		details["query_fields"] = describe(reflect.TypeOf(domain.Recipient{}))
	}
	if resource != "" && len(descriptions) == 0 {
		return nil, usage("unknown schema resource " + resource)
	}
	return map[string]any{"schema_version": 1, "record_format": "domain JSON; exact field names are required for input", "resources": descriptions}, nil
}
func describe(t reflect.Type) any {
	if t == reflect.TypeOf(time.Time{}) {
		return "RFC3339 timestamp"
	}
	switch t.Kind() {
	case reflect.Struct:
		fields := map[string]any{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" || f.Tag.Get("json") == "-" {
				continue
			}
			name := f.Name
			if tag := strings.Split(f.Tag.Get("json"), ",")[0]; tag != "" {
				name = tag
			}
			fields[name] = describe(f.Type)
		}
		return fields
	case reflect.Slice:
		return []any{describe(t.Elem())}
	case reflect.Map:
		return "object"
	default:
		return t.String()
	}
}

type contactSummary struct {
	ID, Kind, Name, Organization, Tags, UpdatedAt, CreatedAt string
	Favorite, Archived                                       bool
	Revision                                                 int
}
