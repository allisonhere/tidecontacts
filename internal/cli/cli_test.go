package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

type reply struct {
	Version int             `json:"schema_version"`
	Data    json.RawMessage `json:"data"`
	ETag    string          `json:"etag"`
	Meta    metadata        `json:"meta"`
}
type harness struct {
	t  *testing.T
	db string
}

func setup(t *testing.T) harness {
	t.Helper()
	root := t.TempDir()
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
		t.Setenv(key, root)
	}
	return harness{t, filepath.Join(root, "script.db")}
}
func (h harness) run(code int, input string, args ...string) reply {
	h.t.Helper()
	args = append(args, "--database", h.db)
	var out, stderr bytes.Buffer
	err := Run(context.Background(), args, strings.NewReader(input), &out, &stderr)
	if code != 0 {
		var e *Error
		if !errors.As(err, &e) || e.ExitCode != code {
			h.t.Fatalf("%v: wanted exit %d, got %v stderr=%s", args, code, err, stderr.String())
		}
		if out.Len() != 0 {
			h.t.Fatal("failure wrote stdout", out.String())
		}
		var diag map[string]any
		if json.Unmarshal(stderr.Bytes(), &diag) != nil {
			h.t.Fatal("non-JSON diagnostic", stderr.String())
		}
		return reply{}
	}
	if err != nil || stderr.Len() != 0 {
		h.t.Fatalf("%v: %v %s", args, err, stderr.String())
	}
	var got reply
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		h.t.Fatal(err, out.String())
	}
	if got.Version != 1 {
		h.t.Fatal(got)
	}
	return got
}
func fields(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func payload(value any) string { raw, _ := json.Marshal(value); return string(raw) }
func TestCoreCRUDAndLinks(t *testing.T) {
	h := setup(t)
	contact := h.run(0, `{"DisplayName":"Alex CLI","Tags":["friend"],"Notes":"Keep this","Methods":[{"Kind":"email","Value":"alex@example.com"}]}`, "contacts", "create", "--input", "-")
	c := fields(t, contact.Data)
	cid := c["ID"].(string)
	org := h.run(0, `{"DisplayName":"Harbor"}`, "organizations", "create", "--input", "-")
	if fields(t, org.Data)["Kind"] != "organization" {
		t.Fatal("wrong kind")
	}
	h.run(4, "", "organizations", "get", cid)
	list := h.run(0, "", "contacts", "list", "--query", "tag:friend", "--full")
	if list.Meta.Count != 1 || !bytes.Contains(list.Data, []byte("alex@example.com")) {
		t.Fatal(string(list.Data))
	}
	list = h.run(0, "", "contacts", "list")
	if bytes.Contains(list.Data, []byte("SearchText")) {
		t.Fatal("search index leaked")
	}
	p := h.run(0, payload(map[string]any{"Name": "CLI Project", "Members": []map[string]string{{"ID": cid}}, "Milestones": []map[string]string{{"Title": "Ship", "Due": "2026-12-01"}}}), "projects", "create", "--input", "-")
	pid := fields(t, p.Data)["ID"].(string)
	task := h.run(0, payload(map[string]any{"Title": "CLI task", "Entities": []map[string]string{{"ID": cid}}, "Projects": []map[string]string{{"ID": pid}}}), "tasks", "create", "--input", "-")
	taskFields := fields(t, task.Data)
	tid := taskFields["ID"].(string)
	updated := h.run(0, payload(map[string]any{"Revision": taskFields["Revision"], "Priority": 3}), "tasks", "update", tid, "--input", "-")
	if fields(t, updated.Data)["Title"] != "CLI task" {
		t.Fatal("partial update lost fields")
	}
	h.run(5, payload(map[string]any{"Revision": taskFields["Revision"], "Title": "stale"}), "tasks", "update", tid, "--input", "-")
	h.run(0, "", "tasks", "complete", tid)
	h.run(0, "", "tasks", "reopen", tid)
	h.run(0, "", "tasks", "duplicate", tid)
	h.run(0, payload(map[string]any{"Title": "CLI meeting", "Type": "meeting", "OccurredAt": "2026-11-15T12:00:00Z", "Entities": []map[string]string{{"ID": cid}}}), "activities", "create", "--input", "-")
	h.run(0, "", "contacts", "dossier", cid)
	h.run(0, "", "contacts", "connections", cid)
	h.run(0, payload(map[string]any{"Revision": c["Revision"], "DisplayName": "Alex Updated"}), "contacts", "update", cid, "--input", "-")
	h.run(0, "", "contacts", "history", cid)
	h.run(2, "", "contacts", "delete", cid)
	deleted := h.run(0, "", "contacts", "delete", cid, "--yes")
	if fields(t, deleted.Data)["Deleted"] != true {
		t.Fatal("not deleted")
	}
	h.run(0, "", "contacts", "restore", cid, "--yes")
	for _, resource := range []string{"tasks", "projects"} {
		id := tid
		if resource == "projects" {
			id = pid
		}
		h.run(0, "", resource, "delete", id, "--yes")
		h.run(0, "", resource, "restore", id, "--yes")
	}
}
func TestPipelinesConcurrencyAndStageOwnership(t *testing.T) {
	h := setup(t)
	p := h.run(0, `{"Name":"Hiring","Stages":[{"Name":"Applied"},{"Name":"Interview"}]}`, "pipelines", "create", "--input", "-")
	f := fields(t, p.Data)
	id := f["ID"].(string)
	stages := f["Stages"].([]any)
	first := stages[0].(map[string]any)["ID"].(string)
	second := stages[1].(map[string]any)["ID"].(string)
	item := h.run(0, payload(map[string]any{"Title": "Opportunity", "PipelineID": id, "StageID": first}), "items", "create", "--input", "-")
	iid := fields(t, item.Data)["ID"].(string)
	// Derived counts changing must not invalidate configuration edits.
	updated := h.run(0, `{"Name":"Hiring updated"}`, "pipelines", "update", id, "--if-match", p.ETag, "--input", "-")
	h.run(5, `{"Name":"Stale"}`, "pipelines", "update", id, "--if-match", p.ETag, "--input", "-")
	h.run(2, `{"Name":"No token"}`, "pipelines", "update", id, "--input", "-")
	h.run(3, `{"Stages":[{"ID":"other-pipeline-stage","Name":"Bad"}]}`, "pipelines", "update", id, "--if-match", updated.ETag, "--input", "-")
	h.run(0, "", "items", "move", iid, "--stage", second)
	h.run(0, "", "items", "list", "--pipeline", id)
	h.run(0, "", "items", "delete", iid, "--yes")
	h.run(0, "", "items", "restore", iid, "--yes")
	h.run(0, "", "pipelines", "archive", id, "--if-match", updated.ETag, "--yes")
	h.run(0, "", "pipelines", "get", id)
}
func TestValidationAndExitCodes(t *testing.T) {
	h := setup(t)
	for _, tc := range []struct {
		input string
		args  []string
		code  int
	}{
		{``, []string{"tasks", "get", "missing"}, 4},
		{`{}`, []string{"tasks", "create", "--input", "-"}, 3},
		{`{"Title":"x","Typo":1}`, []string{"tasks", "create", "--input", "-"}, 3},
		{`{"DisplayName":"x","Methods":[{"Kind":"email","Vlaue":"x"}]}`, []string{"contacts", "create", "--input", "-"}, 3},
		{`null`, []string{"tasks", "create", "--input", "-"}, 3},
		{`{"Title":null}`, []string{"tasks", "create", "--input", "-"}, 3},
		{`{"Title":"x"} {}`, []string{"tasks", "create", "--input", "-"}, 3},
		{`{"ID":"forged","Title":"x"}`, []string{"tasks", "create", "--input", "-"}, 3},
		{``, []string{"tasks", "list", "--view", "typo"}, 2},
		{``, []string{"contacts", "list", "--view", "today"}, 2},
		{``, []string{"activities", "list", "--limit", "5001"}, 2},
		{``, []string{"pipelines", "delete", "x", "--yes"}, 2},
		{``, []string{"unknown", "get", "x"}, 2},
	} {
		t.Run(strings.Join(tc.args, " ")+tc.input, func(t *testing.T) { h.run(tc.code, tc.input, tc.args...) })
	}
	task := h.run(0, `{"Title":"test"}`, "tasks", "create", "--input", "-")
	id := fields(t, task.Data)["ID"].(string)
	h.run(3, `{"Title":"no revision"}`, "tasks", "update", id, "--input", "-")
	h.run(3, `{"Revision":1,"ID":"another"}`, "tasks", "update", id, "--input", "-")
}
func TestDemoIsolationSchemaAndArchive(t *testing.T) {
	h := setup(t)
	sentinel := []byte("keep live file")
	root := filepath.Dir(h.db)
	live := filepath.Join(root, "tidecontacts", "tidecontacts.db")
	if err := os.MkdirAll(filepath.Dir(live), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(live, sentinel, 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--demo", "contacts", "list"}, strings.NewReader(""), &out, &stderr); err != nil {
		t.Fatal(err, stderr.String())
	}
	data, _ := os.ReadFile(live)
	if !bytes.Equal(data, sentinel) {
		t.Fatal("demo touched live database")
	}
	h.run(2, "", "--demo", "contacts", "list") // explicit --database and demo cannot combine
	out.Reset()
	stderr.Reset()
	if err := Run(context.Background(), []string{"schema", "tasks"}, strings.NewReader(""), &out, &stderr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Revision")) {
		t.Fatal("schema missing fields")
	}
	if _, err := os.Stat(h.db); !os.IsNotExist(err) {
		t.Fatal("schema opened database")
	}
	h.run(0, `{"Title":"Archive task"}`, "tasks", "create", "--input", "-")
	exported := h.run(0, "", "crm", "export")
	h.run(0, string(exported.Data), "crm", "import", "--input", "-")
}
func TestExternalActivitiesRemainReadOnly(t *testing.T) {
	h := setup(t)
	s, e := sqlite.Open(context.Background(), h.db)
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.UpsertExternalActivity(context.Background(), domain.Activity{Type: "email", Title: "Source owned", SourceApp: "tidemail", ExternalRef: "test"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	h.run(3, payload(map[string]any{"Revision": a.Revision, "Title": "overwrite"}), "activities", "update", a.ID, "--input", "-")
}

func TestSchemaUsesNestedJSONTags(t *testing.T) {
	value, err := schema("tasks")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(value)
	for _, name := range []string{`"by_day"`, `"after_completion"`, `"id"`} {
		if !bytes.Contains(raw, []byte(name)) {
			t.Fatal("schema ignored JSON tag", name, string(raw))
		}
	}
}
