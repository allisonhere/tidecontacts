package webapp

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/allisonhere/tidecontacts/internal/demo"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

const (
	fixtureHost  = "127.0.0.1:43891"
	fixtureToken = "test-launch-token-not-an-asset"
)

type wireFields = map[string]any

type wirePage[T any] struct {
	Items []T `json:"items"`
}

type wireContact struct {
	Record  domain.Contact  `json:"record"`
	Dossier *domain.Dossier `json:"dossier"`
}

type wireBoard struct {
	Record domain.Pipeline       `json:"record"`
	Items  []domain.PipelineItem `json:"items"`
}

func webFixture(t *testing.T, assets fs.FS) (*sqlite.Store, http.Handler) {
	t.Helper()
	store, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "contacts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := demo.Seed(t.Context(), store); err != nil {
		t.Fatal(err)
	}
	return store, New(store, true, fixtureHost, fixtureToken, assets)
}

func webRequest(method, path, body string) *http.Request {
	return httptest.NewRequest(method, "http://"+fixtureHost+path, strings.NewReader(body))
}

func webSend(handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, req)
	return out
}

func webEnvelope(t *testing.T, out *httptest.ResponseRecorder, status int, code string) json.RawMessage {
	t.Helper()
	if out.Code != status || !strings.HasPrefix(out.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("response = %d %s, body %s; want JSON with HTTP %d", out.Code, out.Header().Get("Content-Type"), out.Body, status)
	}
	if strings.Contains(out.Body.String(), fixtureToken) {
		t.Fatal("response exposed the launch token")
	}
	var v struct {
		Version int             `json:"schema_version"`
		Data    json.RawMessage `json:"data"`
		Error   *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Version != 1 {
		t.Fatalf("schema_version = %d; want 1", v.Version)
	}
	if code != "" {
		if v.Error == nil || v.Error.Code != code || strings.TrimSpace(v.Error.Message) == "" || len(v.Data) != 0 {
			t.Fatalf("want error %q and no data, got %s", code, out.Body)
		}
	} else if v.Error != nil || len(v.Data) == 0 || bytes.Equal(v.Data, []byte("null")) {
		t.Fatalf("want data and no error, got %s", out.Body)
	}
	return v.Data
}

func webCall(t *testing.T, h http.Handler, method, path string, fields wireFields, status int, code string) json.RawMessage {
	t.Helper()
	var body []byte
	if fields != nil {
		var err error
		body, err = json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
	}
	r := webRequest(method, "/api/v1/"+path, string(body))
	r.Header.Set("Authorization", "Bearer "+fixtureToken)
	if fields != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	return webEnvelope(t, webSend(h, r), status, code)
}

func webDecode[T any](t *testing.T, data json.RawMessage) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func webCreate[T any](t *testing.T, h http.Handler, kind string, fields wireFields) T {
	t.Helper()
	return webDecode[T](t, webCall(t, h, http.MethodPost, kind, fields, http.StatusCreated, ""))
}

func webGet[T any](t *testing.T, h http.Handler, path string) T {
	t.Helper()
	return webDecode[T](t, webCall(t, h, http.MethodGet, path, nil, http.StatusOK, ""))
}

func TestWebSessionBoundary(t *testing.T) {
	const secret = "private file content"
	_, h := webFixture(t, fstest.MapFS{
		"index.html":        {Data: []byte("<!doctype html><title>TideContacts</title>")},
		"assets/client.js":  {Data: []byte("export {};")},
		"assets/key.json":   {Data: []byte(secret)},
		"session.key":       {Data: []byte(secret)},
		"tidecontacts.db":   {Data: []byte(secret)},
		"other/unlisted.js": {Data: []byte(secret)},
	})
	for _, tc := range []struct {
		name, suffix, host, auth, origin, site string
		status                                 int
		code                                   string
	}{
		{name: "no credential", status: 401, code: "session_required"},
		{name: "query is not authentication", suffix: "?token=" + fixtureToken, status: 401, code: "session_required"},
		{name: "wrong bearer", auth: "Bearer wrong", status: 401, code: "session_required"},
		{name: "host alias", host: "localhost:43891", auth: "Bearer " + fixtureToken, status: 403, code: "permission_denied"},
		{name: "wrong port", host: "127.0.0.1:43892", auth: "Bearer " + fixtureToken, status: 403, code: "permission_denied"},
		{name: "foreign origin", auth: "Bearer " + fixtureToken, origin: "http://127.0.0.1:43892", status: 403, code: "permission_denied"},
		{name: "opaque origin", auth: "Bearer " + fixtureToken, origin: "null", status: 403, code: "permission_denied"},
		{name: "cross site", auth: "Bearer " + fixtureToken, site: "cross-site", status: 403, code: "permission_denied"},
		{name: "same origin", auth: "Bearer " + fixtureToken, origin: "http://" + fixtureHost, site: "same-origin", status: 200},
		{name: "native client", auth: "Bearer " + fixtureToken, status: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := webRequest(http.MethodGet, "/api/v1/profile"+tc.suffix, "")
			if tc.host != "" {
				r.Host = tc.host
			}
			r.Header.Set("Authorization", tc.auth)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Sec-Fetch-Site", tc.site)
			out := webSend(h, r)
			data := webEnvelope(t, out, tc.status, tc.code)
			if out.Header().Get("Access-Control-Allow-Origin") != "" || out.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("unexpected CORS or caching headers: %v", out.Header())
			}
			if tc.status == 200 {
				profile := webDecode[struct {
					Demo         bool     `json:"demo"`
					Professional []string `json:"professional"`
				}](t, data)
				if !profile.Demo || profile.Professional == nil || len(profile.Professional) != 0 {
					t.Fatalf("expected a Demo profile with no Professional capabilities, got %+v", profile)
				}
			}
		})
	}
	r := webRequest(http.MethodOptions, "/api/v1/people", "")
	r.Header.Set("Origin", "https://another.example")
	r.Header.Set("Access-Control-Request-Method", "POST")
	r.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
	out := webSend(h, r)
	webEnvelope(t, out, http.StatusForbidden, "permission_denied")
	if out.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("preflight granted origin: %v", out.Header())
	}

	for _, path := range []string{"/", "/assets/client.js", "/people"} {
		out := webSend(h, webRequest(http.MethodGet, path, ""))
		if out.Code != http.StatusOK || strings.Contains(out.Body.String(), fixtureToken) || out.Header().Get("Content-Security-Policy") == "" || len(out.Result().Cookies()) != 0 {
			t.Fatalf("unexpected public shell/asset response for %s: %d %v", path, out.Code, out.Header())
		}
	}
	r = webRequest(http.MethodGet, "/", "")
	r.Host = "localhost:43891"
	webEnvelope(t, webSend(h, r), http.StatusForbidden, "permission_denied")
	for _, path := range []string{"/session.key", "/tidecontacts.db", "/assets/key.json", "/other/unlisted.js", "/assets/%2e%2e/session.key", "/people/session.key"} {
		out := webSend(h, webRequest(http.MethodGet, path, ""))
		if out.Code != http.StatusNotFound || strings.Contains(out.Body.String(), secret) || strings.Contains(out.Body.String(), fixtureToken) {
			t.Fatalf("private file route %s: HTTP %d", path, out.Code)
		}
	}
}

func TestWebRequestLimits(t *testing.T) {
	_, h := webFixture(t, nil)
	for _, tc := range []struct {
		name, mime, body string
		status           int
		code             string
	}{
		{"content type required", "", `{"name":"Limitfixture"}`, 415, "json_required"},
		{"not JSON", "text/plain", `{"name":"Limitfixture"}`, 415, "json_required"},
		{"unknown field", "application/json", `{"name":"Limitfixture","unrecognized":true}`, 400, "invalid_input"},
		{"two objects", "application/json", `{"name":"Limitfixture"} {}`, 400, "invalid_input"},
		{"oversize", "application/json", `{"name":"Limitfixture","notes":"` + strings.Repeat("x", 64<<10) + `"}`, 413, "body_too_large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := webRequest(http.MethodPost, "/api/v1/people", tc.body)
			r.Header.Set("Authorization", "Bearer "+fixtureToken)
			r.Header.Set("Content-Type", tc.mime)
			webEnvelope(t, webSend(h, r), tc.status, tc.code)
		})
	}
	webCall(t, h, http.MethodGet, "people?limit=101", nil, 400, "invalid_input")
	webCall(t, h, http.MethodGet, "not-a-workspace", nil, 404, "not_found")
	if rows := webGet[wirePage[domain.Summary]](t, h, "people?q=Limitfixture"); len(rows.Items) != 0 {
		t.Fatalf("rejected requests created contacts: %+v", rows.Items)
	}
}

func TestWebContactDossierAndRevision(t *testing.T) {
	_, h := webFixture(t, nil)
	org := webCreate[domain.Contact](t, h, "organizations", wireFields{"name": "Fixture Guild"})
	person := webCreate[domain.Contact](t, h, "people", wireFields{"name": "Cora Fixture", "organization_id": org.ID, "notes": "Original notes", "tags": []string{"fixture"}})
	project := webCreate[domain.Project](t, h, "projects", wireFields{"title": "Fixture project", "entity_id": person.ID})
	task := webCreate[domain.Task](t, h, "tasks", wireFields{"title": "Review fixture", "entity_id": person.ID, "project_id": project.ID})
	activity := webCreate[domain.Activity](t, h, "activities", wireFields{"title": "Met at the guild", "type": "meeting", "entity_id": person.ID, "project_id": project.ID})
	path := "people/" + person.ID
	detail := webGet[wireContact](t, h, path)
	d := detail.Dossier
	if detail.Record.ID != person.ID || d == nil || d.Root.ID != person.ID || d.Root.Kind != "person" || d.OpenTaskCount != 1 || d.Activities != 1 {
		t.Fatalf("unexpected person dossier: %+v", d)
	}
	if len(d.Organizations) != 1 || !d.Organizations[0].Current || len(d.Projects) != 1 || len(d.OpenTasks) != 1 || len(d.Recent) != 1 {
		t.Fatalf("missing dossier links: %+v", d)
	}
	orgRef, projectRef := d.Organizations[0].Organization, d.Projects[0].Project
	if orgRef.Kind != "organization" || orgRef.ID != org.ID || projectRef.Kind != "project" || projectRef.ID != project.ID || d.OpenTasks[0].ID != task.ID || d.Recent[0].ID != activity.ID {
		t.Fatalf("dossier links point to the wrong records: %+v", d)
	}
	organization := webGet[wireContact](t, h, "organizations/"+orgRef.ID)
	if organization.Record.ID != org.ID || organization.Dossier == nil || len(organization.Dossier.Organizations) != 1 || !hasWebRef(organization.Dossier.Organizations[0].People, person.ID) {
		t.Fatalf("organization dossier has no navigable member: %+v", organization.Dossier)
	}
	if p := webGet[domain.Project](t, h, "projects/"+projectRef.ID); !hasWebRef(p.Members, person.ID) {
		t.Fatalf("project lost its linked person: %+v", p.Members)
	}
	if item := webGet[domain.Task](t, h, "tasks/"+d.OpenTasks[0].ID); !hasWebRef(item.Projects, project.ID) {
		t.Fatalf("task lost its project: %+v", item.Projects)
	}
	if item := webGet[domain.Activity](t, h, "activities/"+d.Recent[0].ID); !hasWebRef(item.Entities, person.ID) {
		t.Fatalf("activity lost its person: %+v", item.Entities)
	}
	webCall(t, h, http.MethodGet, "organizations/"+person.ID, nil, 404, "not_found")

	updated := webDecode[domain.Contact](t, webCall(t, h, http.MethodPatch, path, wireFields{"revision": detail.Record.Revision, "notes": "Saved first"}, 200, ""))
	webCall(t, h, http.MethodPatch, path, wireFields{"revision": detail.Record.Revision, "name": "Stale overwrite", "notes": "Stale notes", "organization_id": nil}, 409, "revision_conflict")
	current := webGet[wireContact](t, h, path).Record
	if current.Revision <= detail.Record.Revision || current.Revision != updated.Revision || current.DisplayName != person.DisplayName || current.Notes != "Saved first" || !slices.Equal(current.Tags, []string{"fixture"}) || len(current.Memberships) != 1 || current.Memberships[0].OrganizationID != org.ID || !current.Memberships[0].Current {
		t.Fatalf("stale or partial patch clobbered contact: %+v", current)
	}

	webCall(t, h, http.MethodPatch, path, wireFields{"revision": current.Revision, "archived": true}, 200, "")
	archived := webGet[wireContact](t, h, path)
	if !archived.Record.Archived || archived.Dossier != nil {
		t.Fatalf("archived record should remain restorable without an active dossier: %+v", archived)
	}
	listed := func(path string) bool {
		return slices.ContainsFunc(webGet[wirePage[domain.Summary]](t, h, path).Items, func(row domain.Summary) bool {
			return row.ID == person.ID
		})
	}
	if listed("people") || !listed("people?q=is%3Aarchived") {
		t.Fatal("archiving did not move the person out of the active collection")
	}
	webCall(t, h, http.MethodPatch, path, wireFields{"revision": archived.Record.Revision, "archived": false}, 200, "")
	restored := webGet[wireContact](t, h, path)
	if restored.Record.Archived || restored.Dossier == nil || restored.Dossier.OpenTaskCount != 1 || !listed("people") || listed("people?q=is%3Aarchived") {
		t.Fatalf("restoration did not restore the active collection and dossier: %+v", restored)
	}
}

func hasWebRef(refs []domain.Ref, id string) bool {
	return slices.ContainsFunc(refs, func(ref domain.Ref) bool { return ref.ID == id })
}

func TestWebActivityProvenanceAndLinks(t *testing.T) {
	store, h := webFixture(t, nil)
	person := webCreate[domain.Contact](t, h, "people", wireFields{"name": "Casey Activity Fixture"})
	project := webCreate[domain.Project](t, h, "projects", wireFields{"title": "Activity fixture project"})
	task := webCreate[domain.Task](t, h, "tasks", wireFields{"title": "Activity fixture task"})
	org := webCreate[domain.Contact](t, h, "organizations", wireFields{"name": "Fixture correspondence org"})
	activity := domain.Activity{
		Title: "Original activity", Type: "email", OccurredAt: time.Date(2024, 1, 2, 15, 4, 5, 0, time.UTC),
		Entities: []domain.Ref{{ID: person.ID}}, Projects: []domain.Ref{{ID: project.ID}}, Tasks: []domain.Ref{{ID: task.ID}},
		Tags: []string{"fixture"},
	}
	local, err := store.SaveActivity(t.Context(), activity)
	if err != nil {
		t.Fatal(err)
	}
	activity.Provenance, activity.SourceApp = domain.External, "test-mail"
	activity.ExternalRef, activity.ExternalURL = "private-connector-ref", "https://example.test/private-routing"
	external, err := store.SaveActivity(t.Context(), activity)
	if err != nil {
		t.Fatal(err)
	}
	localPath, externalPath := "activities/"+local.ID, "activities/"+external.ID
	webCall(t, h, http.MethodPatch, localPath, wireFields{"revision": local.Revision, "title": "Revised activity", "entity_id": org.ID}, 200, "")
	a := webGet[domain.Activity](t, h, localPath)
	if a.Title != "Revised activity" || a.Provenance != domain.Local || !a.OccurredAt.Equal(activity.OccurredAt) || !slices.Equal(a.Tags, activity.Tags) || len(a.Entities) != 2 || !hasWebRef(a.Entities, person.ID) || !hasWebRef(a.Entities, org.ID) || !hasWebRef(a.Projects, project.ID) || !hasWebRef(a.Tasks, task.ID) {
		t.Fatalf("activity edit lost data or existing links: %+v", a)
	}
	webCall(t, h, http.MethodPatch, externalPath, wireFields{"revision": external.Revision, "title": "Attempted overwrite", "entity_id": org.ID}, 403, "permission_denied")
	checkExternal := func(a domain.Activity) {
		t.Helper()
		if a.Title != external.Title || a.Revision != external.Revision || a.Provenance != domain.External || a.SourceApp != activity.SourceApp || a.ExternalRef != "" || a.ExternalURL != "" {
			t.Fatalf("external activity changed or exposed source routing: %+v", a)
		}
	}
	checkExternal(webGet[domain.Activity](t, h, externalPath))
	for _, rows := range [][]domain.Activity{
		webGet[wirePage[domain.Activity]](t, h, "activities?entity="+person.ID).Items,
		webGet[wireContact](t, h, "people/"+person.ID).Dossier.Recent,
	} {
		i := slices.IndexFunc(rows, func(a domain.Activity) bool { return a.ID == external.ID })
		if i < 0 {
			t.Fatal("external activity is absent from a related activity view")
		}
		checkExternal(rows[i])
	}
	stored, err := store.Activity(t.Context(), external.ID)
	if err != nil || stored.ExternalRef != activity.ExternalRef || stored.ExternalURL != activity.ExternalURL || !hasWebRef(stored.Entities, person.ID) || hasWebRef(stored.Entities, org.ID) {
		t.Fatalf("external source metadata or relationships changed in SQLite: %+v, %v", stored, err)
	}
}

func TestWebTaskCompletionEffects(t *testing.T) {
	_, h := webFixture(t, nil)
	person := webCreate[domain.Contact](t, h, "people", wireFields{"name": "Task Fixture Person"})
	project := webCreate[domain.Project](t, h, "projects", wireFields{"title": "Task fixture project"})
	task := webCreate[domain.Task](t, h, "tasks", wireFields{"title": "Make the call", "entity_id": person.ID, "project_id": project.ID})
	path := "tasks/" + task.ID
	activityPath := "activities?entity=" + person.ID + "&project=" + project.ID
	latest := webDecode[domain.Task](t, webCall(t, h, http.MethodPatch, path, wireFields{"revision": task.Revision, "description": "Use the work number"}, 200, ""))
	webCall(t, h, http.MethodPatch, path, wireFields{"revision": latest.Revision, "status": "completed"}, 400, "invalid_input")
	webCall(t, h, http.MethodPost, path+"/complete", wireFields{"revision": task.Revision}, 409, "revision_conflict")
	if open := webGet[domain.Task](t, h, path); !open.Open() || open.CompletedAt != "" || open.Revision != latest.Revision {
		t.Fatalf("rejected completion modified the task: %+v", open)
	}
	if rows := webGet[wirePage[domain.Activity]](t, h, activityPath).Items; len(rows) != 0 {
		t.Fatalf("rejected completion produced activity: %+v", rows)
	}
	completed := webDecode[domain.Task](t, webCall(t, h, http.MethodPost, path+"/complete", wireFields{"revision": latest.Revision}, 200, ""))
	if done := webGet[domain.Task](t, h, path); done.Status != "completed" || done.CompletedAt == "" || done.Revision != completed.Revision || done.Revision <= latest.Revision || done.Description != latest.Description || !hasWebRef(done.Entities, person.ID) || !hasWebRef(done.Projects, project.ID) {
		t.Fatalf("completion was not persisted with the prior content and links: %+v", done)
	}
	webCall(t, h, http.MethodPost, path+"/complete", wireFields{"revision": latest.Revision}, 409, "revision_conflict")
	entries := webGet[wirePage[domain.Activity]](t, h, activityPath).Items
	if len(entries) != 1 || entries[0].Type != "task-completion" {
		t.Fatalf("expected one real completion entry, got %+v", entries)
	}
	entryPath := "activities/" + entries[0].ID
	entry := webGet[domain.Activity](t, h, entryPath)
	if !hasWebRef(entry.Tasks, task.ID) || !hasWebRef(entry.Entities, person.ID) || !hasWebRef(entry.Projects, project.ID) {
		t.Fatalf("completion entry lost workflow links: %+v", entry)
	}
	webCall(t, h, http.MethodPatch, entryPath, wireFields{"revision": entry.Revision, "title": "Not a completion"}, 403, "permission_denied")
	d := webGet[wireContact](t, h, "people/"+person.ID).Dossier
	if d == nil || d.OpenTaskCount != 0 || len(d.Commitments) != 1 || d.Commitments[0].ID != task.ID || len(d.Recent) != 1 || d.Recent[0].ID != entry.ID {
		t.Fatalf("person dossier did not reflect completion: %+v", d)
	}
}

func TestWebPipelineMoveIntegrity(t *testing.T) {
	store, h := webFixture(t, nil)
	person := webCreate[domain.Contact](t, h, "people", wireFields{"name": "Pipeline Fixture Person"})
	ctx := t.Context()
	board, err := store.SavePipeline(ctx, domain.Pipeline{Name: "Source fixture board", Stages: []domain.Stage{{Name: "Received"}, {Name: "Reviewed"}}})
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.SavePipeline(ctx, domain.Pipeline{Name: "Other fixture board", Stages: []domain.Stage{{Name: "Elsewhere"}}})
	if err != nil {
		t.Fatal(err)
	}
	item, err := store.SaveItem(ctx, domain.PipelineItem{Title: "Move fixture item", PipelineID: board.ID, Notes: "Retain this note", Entities: []domain.Ref{{ID: person.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	boardPath, otherPath := "pipelines/"+board.ID, "pipelines/"+other.ID
	before := webGet[wireBoard](t, h, boardPath)
	if before.Record.ID != board.ID || len(before.Record.Stages) != 2 || len(before.Items) != 1 || before.Items[0].ID != item.ID || before.Items[0].StageID != before.Record.Stages[0].ID || len(item.History) != 1 || item.History[0].To != before.Record.Stages[0].Name {
		t.Fatalf("unexpected starting board or stage history: %+v / %+v", before, item.History)
	}
	foreign := webGet[wireBoard](t, h, otherPath)
	if len(foreign.Record.Stages) != 1 {
		t.Fatalf("expected one foreign stage: %+v", foreign)
	}
	from, to := before.Record.Stages[0], before.Record.Stages[1]
	path := "items/" + item.ID + "/move"
	observedRevision := before.Items[0].Revision
	webCall(t, h, http.MethodPost, path, wireFields{"revision": observedRevision, "stage_id": foreign.Record.Stages[0].ID}, 400, "invalid_input")
	moved := webDecode[domain.PipelineItem](t, webCall(t, h, http.MethodPost, path, wireFields{"revision": observedRevision, "stage_id": to.ID}, 200, ""))
	if moved.ID != item.ID || moved.PipelineID != board.ID || moved.StageID != to.ID || moved.Revision <= observedRevision || moved.Notes != item.Notes || !hasWebRef(moved.Entities, person.ID) || len(moved.History) != 2 || moved.History[0] != item.History[0] || moved.History[1].From != from.Name || moved.History[1].To != to.Name || moved.History[1].At == "" || moved.History[1].At != moved.StageEnteredAt {
		t.Fatalf("move did not preserve item data and append the source-to-destination history: %+v", moved)
	}
	webCall(t, h, http.MethodPost, path, wireFields{"revision": observedRevision, "stage_id": from.ID}, 409, "revision_conflict")
	webCall(t, h, http.MethodPost, path, wireFields{"revision": moved.Revision, "stage_id": foreign.Record.Stages[0].ID}, 400, "invalid_input")
	webCall(t, h, http.MethodPost, path, wireFields{"revision": moved.Revision, "stage_id": to.ID}, 200, "")
	after := webGet[wireBoard](t, h, boardPath)
	if len(after.Items) != 1 || after.Items[0].ID != item.ID || after.Items[0].StageID != to.ID || after.Items[0].Revision != moved.Revision || len(webGet[wireBoard](t, h, otherPath).Items) != 0 {
		t.Fatalf("move, stale request, or repeat left unexpected board state: %+v", after)
	}
	persisted, err := store.Item(ctx, item.ID)
	if err != nil || !slices.Equal(persisted.History, moved.History) {
		t.Fatalf("SQLite stage history diverged from move result: %+v, %v", persisted.History, err)
	}
}
