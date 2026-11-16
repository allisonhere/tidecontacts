// Package cli exposes the offline CRM to shell scripts without a terminal or daemon.
package cli

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/config"
	"github.com/allisonhere/tidecontacts/internal/demo"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

// Error carries a documented process exit code. Run already writes its JSON
// diagnostic to stderr; the executable must not print it a second time.
type Error struct {
	ExitCode      int
	Kind, Message string
}

func (e *Error) Error() string                  { return e.Message }
func fail(code int, kind, message string) error { return &Error{code, kind, message} }
func usage(message string) error                { return fail(2, "usage", message) }
func invalid(message string) error              { return fail(3, "invalid_input", message) }

var resources = map[string]bool{"contacts": true, "organizations": true, "tasks": true, "activities": true, "projects": true, "pipelines": true, "items": true, "crm": true, "schema": true}

func Handles(args []string) bool {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return true
	}
	for _, a := range args {
		if resources[a] {
			return true
		}
	}
	return false
}

const Help = `TideContacts scripting CLI (JSON API version 1)

  contacts|organizations list|get|create|update|delete|restore|history|dossier|connections
  tasks                  list|get|create|update|delete|restore|complete|reopen|duplicate
  activities             list|get|create|update|delete|restore
  projects               list|get|create|update|delete|restore
  pipelines              list|get|create|update|archive
  items                  list|get|create|update|delete|restore|move
  crm                    export|import
  schema                 [RESOURCE]

Options (before or after subcommands):
  --demo                 use the separate demo database, seeding it once
  --database PATH        explicit absolute database path; incompatible with --demo
  --input PATH           JSON file, or - for stdin; required for create/update/import
  --query TEXT           same search grammar as the UI
  --kind person|organization  contact list/create kind (default person)
  --full                 return full records from contact lists
  --view VIEW            task list view; --status STATUS for projects
  --limit N              task/activity page size (default 200)
  --pipeline ID          pipeline whose items to list
  --entity ID            person/organization whose tasks/activities to list
  --stage ID             destination stage for items move
  --if-match ETAG        required for pipeline update/archive; from get's etag
  --yes                  required for delete/archive/restore
  --json                 explicit JSON output (already the default)
  --help                 show this help without opening a database

Record JSON uses existing domain names, such as ID, Revision, DisplayName and Title.
Update input is a partial object and must include the current Revision, except
pipelines, which require --if-match. Omitted fields are preserved; arrays replace.
Read data at .data; pipeline concurrency token is .etag. See schema and docs/cli.md.
Exit codes: 0 success; 1 runtime; 2 usage; 3 invalid input; 4 not found; 5 conflict.
`

type options struct {
	present                                                                    map[string]bool
	demo, json, yes, help, full                                                bool
	database, input, query, kind, view, status, pipeline, entity, stage, match string
	limit                                                                      int
}
type envelope struct {
	Version int       `json:"schema_version"`
	Data    any       `json:"data"`
	ETag    string    `json:"etag,omitempty"`
	Meta    *metadata `json:"meta,omitempty"`
}
type metadata struct {
	Resource          string `json:"resource"`
	Action            string `json:"action"`
	Count             int    `json:"count"`
	Limit             int    `json:"limit,omitempty"`
	PossiblyTruncated bool   `json:"possibly_truncated"`
}

func Run(ctx context.Context, args []string, in io.Reader, out, stderr io.Writer) (result error) {
	defer func() {
		if result != nil {
			var e *Error
			if !errors.As(result, &e) {
				e = &Error{1, "runtime", result.Error()}
			}
			_ = json.NewEncoder(stderr).Encode(map[string]any{"schema_version": 1, "error": map[string]string{"code": e.Kind, "message": e.Message}})
			result = e
		}
	}()
	o, pos, err := parse(args)
	if err != nil {
		return err
	}
	if o.help {
		_, err = io.WriteString(out, Help)
		return err
	}
	if len(pos) == 1 && pos[0] == "help" {
		_, err = io.WriteString(out, Help)
		return err
	}
	if len(pos) == 0 {
		return usage("specify a resource and action; use --help")
	}
	if pos[0] == "schema" {
		if len(pos) > 2 {
			return usage("schema accepts at most one resource")
		}
		name := ""
		if len(pos) == 2 {
			name = pos[1]
		}
		data, err := schema(name)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(envelope{Version: 1, Data: data})
	}
	if len(pos) < 2 || len(pos) > 3 {
		return usage("expected RESOURCE ACTION [ID]; use --help")
	}
	resource, action := pos[0], pos[1]
	id := ""
	if len(pos) == 3 {
		id = pos[2]
	}
	if !resources[resource] {
		return usage("unknown resource " + resource)
	}
	if err := validateCommand(resource, action, id, o); err != nil {
		return err
	}
	var input []byte
	if o.input != "" {
		input, err = readInput(o.input, in)
		if err != nil {
			return err
		}
	}
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	if o.database != "" {
		if o.demo {
			return usage("--database and --demo cannot be combined")
		}
		if !filepath.IsAbs(o.database) {
			return usage("--database must be absolute")
		}
		paths.Database = o.database
	}
	if o.demo {
		paths = demo.Paths(paths)
	}
	s, err := sqlite.Open(ctx, paths.Database)
	if err != nil {
		return err
	}
	defer func() {
		if err := s.Close(); result == nil && err != nil {
			result = err
		}
	}()
	if o.demo {
		if err := demo.Seed(ctx, s); err != nil {
			return err
		}
	}
	data, limit, err := execute(ctx, s, resource, action, id, o, input)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fail(4, "not_found", "record not found")
		}
		if errors.Is(err, sqlite.ErrConflict) {
			return fail(5, "conflict", err.Error())
		}
		return err
	}
	response := envelope{Version: 1, Data: data, Meta: &metadata{Resource: resource, Action: action, Limit: limit}}
	value := reflect.ValueOf(data)
	if value.IsValid() && value.Kind() == reflect.Slice {
		response.Meta.Count = value.Len()
		response.Meta.PossiblyTruncated = limit > 0 && value.Len() >= limit
	}
	if value.IsValid() && value.Kind() != reflect.Slice {
		response.Meta.Count = 1
	}
	if p, ok := data.(domain.Pipeline); ok {
		response.ETag = pipelineETag(p)
	}
	return json.NewEncoder(out).Encode(response)
}
func parse(args []string) (options, []string, error) {
	var o options
	o.present = map[string]bool{}
	fs := flag.NewFlagSet("tidecontacts", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&o.full, "full", false, "")
	fs.BoolVar(&o.demo, "demo", false, "")
	fs.BoolVar(&o.json, "json", true, "")
	fs.BoolVar(&o.yes, "yes", false, "")
	fs.BoolVar(&o.help, "help", false, "")
	fs.StringVar(&o.database, "database", "", "")
	fs.StringVar(&o.input, "input", "", "")
	fs.StringVar(&o.query, "query", "", "")
	fs.StringVar(&o.kind, "kind", "person", "")
	fs.StringVar(&o.view, "view", "", "")
	fs.StringVar(&o.status, "status", "", "")
	fs.StringVar(&o.pipeline, "pipeline", "", "")
	fs.StringVar(&o.entity, "entity", "", "")
	fs.StringVar(&o.stage, "stage", "", "")
	fs.StringVar(&o.match, "if-match", "", "")
	fs.IntVar(&o.limit, "limit", 200, "")
	flags, pos := []string{}, []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			pos = append(pos, a)
			continue
		}
		key := strings.TrimLeft(strings.SplitN(a, "=", 2)[0], "-")
		f := fs.Lookup(key)
		if f == nil {
			return o, nil, usage("unknown option " + a)
		}
		o.present[key] = true
		flags = append(flags, a)
		if !strings.Contains(a, "=") && key != "demo" && key != "json" && key != "yes" && key != "help" && key != "full" {
			if i+1 >= len(args) {
				return o, nil, usage("missing value for " + a)
			}
			i++
			flags = append(flags, args[i])
		}
	}
	if err := fs.Parse(flags); err != nil {
		return o, nil, usage(err.Error())
	}
	if !o.json {
		return o, nil, usage("this CLI supports JSON output only")
	}
	return o, pos, nil
}
func readInput(path string, in io.Reader) ([]byte, error) {
	source := in
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		source = f
	}
	data, err := io.ReadAll(io.LimitReader(source, 50*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 50*1024*1024 {
		return nil, invalid("input exceeds 50 MiB")
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, invalid("input is empty")
	}
	return data, nil
}
func pipelineETag(p domain.Pipeline) string {
	p.Stages = append([]domain.Stage{}, p.Stages...)
	for i := range p.Stages {
		p.Stages[i].Count = 0
	}
	raw, _ := json.Marshal(p)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
