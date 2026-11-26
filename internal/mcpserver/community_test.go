package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

func communityFixture(t *testing.T, scopes ...string) (context.Context, *sqlite.Store, *Server) {
	t.Helper()
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "community.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return ctx, store, &Server{repo: store, profile: app.MCPProfile{Scopes: scopes}}
}

func communityWantProblem(t *testing.T, err error, code string) {
	t.Helper()
	var p *Problem
	if !errors.As(err, &p) || p.Code != code {
		t.Fatalf("got %v; want %s", err, code)
	}
}

func communityAssertPrivate(t *testing.T, v any) {
	t.Helper()
	encoded, err := json.Marshal(v)
	if err != nil || strings.Contains(string(encoded), "PRIVATE") {
		t.Fatalf("unexpected projection: %s", encoded)
	}
}

func TestCommunitySearchAndScopedDossier(t *testing.T) {
	ctx, store, server := communityFixture(t, "contacts.read", "relationships.read", "activities.read")
	a := domain.NewContact("person")
	a.ID, a.DisplayName, a.Notes = "00000000-0000-4000-8000-000000000001", "Alice", "PRIVATE contact notes"
	b := domain.NewContact("person")
	b.ID, b.DisplayName, b.Tags = "00000000-0000-4000-8000-000000000002", "Bob", []string{"Alice cohort"}
	org := domain.NewContact("organization")
	org.DisplayName = "PRIVATE organization"
	a.Memberships = []domain.Membership{{OrganizationID: org.ID, Current: true}}
	a.Relationships = []domain.Relationship{{TargetID: b.ID, Type: "colleague", Notes: "PRIVATE edge notes"}}
	if _, err := store.SaveContacts(ctx, []domain.Contact{a, b, org}, "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRelationshipNotes(ctx, a.ID, "PRIVATE relationship notes"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetFlag(ctx, a.ID, true, "PRIVATE flag text"); err != nil {
		t.Fatal(err)
	}
	activity, err := store.SaveActivity(ctx, domain.Activity{Body: "\u00a0\nPRIVATE body fallback\nPRIVATE body", Entities: []domain.Ref{{ID: a.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveTask(ctx, domain.Task{Title: "PRIVATE task", Entities: []domain.Ref{{ID: a.ID}}}); err != nil {
		t.Fatal(err)
	}

	first, err := server.communityRecordPage(ctx, "contact", communitySearch{Query: " ALICE ", Limit: 1})
	if err != nil || len(first.Items) != 1 || first.Items[0].ID != a.ID || first.NextCursor == "" {
		t.Fatalf("unexpected first page: %+v (%v)", first, err)
	}
	last, err := server.communityRecordPage(ctx, "contact", communitySearch{Query: "alice", Cursor: first.NextCursor, Limit: 1})
	if err != nil || len(last.Items) != 1 || last.Items[0].ID != b.ID || last.NextCursor != "" {
		t.Fatalf("unexpected last page: %+v (%v)", last, err)
	}
	_, err = server.communityRecordPage(ctx, "contact", communitySearch{Query: "bob", Cursor: first.NextCursor})
	communityWantProblem(t, err, "invalid_input")
	_, err = server.communityContact("person")(ctx, communityKey{ID: org.ID})
	communityWantProblem(t, err, "not_found")

	summaries, err := server.communityRecordPage(ctx, "activity", communitySearch{})
	if err != nil || len(summaries.Items) != 1 || summaries.Items[0].Name != "" {
		t.Fatalf("body-derived title in activity page: %+v (%v)", summaries, err)
	}
	view, err := server.communityActivity(ctx, communityKey{ID: activity.ID})
	if err != nil || view.Title != "" {
		t.Fatalf("body-derived title in activity view: %+v (%v)", view, err)
	}
	dossier, err := server.communityDossier(ctx, communityEntity{ID: a.ID, Kind: "contact"})
	if err != nil || dossier.Relationships.Root == nil || !dossier.Relationships.Flagged || len(dossier.Relationships.Outgoing) != 1 || len(dossier.Recent) != 1 {
		t.Fatalf("unexpected dossier: %+v (%v)", dossier, err)
	}
	if dossier.FollowUp != nil || dossier.OpenTaskCount != nil || len(dossier.Relationships.Organizations) != 0 {
		t.Fatalf("dossier crossed scopes: %+v", dossier)
	}
	communityAssertPrivate(t, dossier)
}

func TestCommunityWritesRespectLinksAndRevisions(t *testing.T) {
	ctx, store, server := communityFixture(t, "contacts.read", "tasks.read", "activities.read")
	contact, err := server.communityCreateContact(ctx, communityContactCreate{Name: " Alice ", Email: "alice@example.test"})
	if err != nil || contact.Revision != 1 {
		t.Fatalf("create contact: %+v (%v)", contact, err)
	}
	_, err = server.communityCreateContact(ctx, communityContactCreate{Name: "Name", Email: "Name <alice@example.test>"})
	communityWantProblem(t, err, "invalid_input")
	_, err = server.communityCreateTask(ctx, communityTaskCreate{Title: "Find project", communityTaskFields: communityTaskFields{ProjectID: &contact.ID}})
	communityWantProblem(t, err, "permission_denied")

	entity := &communityEntity{ID: contact.ID, Kind: "contact"}
	project, err := server.communityCreateProject(ctx, communityProjectCreate{Name: "Launch", communityProjectFields: communityProjectFields{Member: entity}})
	if err != nil || project.Revision != 1 {
		t.Fatalf("create project: %+v (%v)", project, err)
	}
	projectName := "Launch two"
	_, err = server.communityUpdateProject(ctx, communityProjectUpdate{communityRevision: communityRevision{ID: project.ID, Revision: project.Revision}, Name: &projectName})
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.communityUpdateProject(ctx, communityProjectUpdate{communityRevision: communityRevision{ID: project.ID, Revision: project.Revision}, Name: &projectName})
	communityWantProblem(t, err, "conflict")

	task, err := server.communityCreateTask(ctx, communityTaskCreate{Title: "Call Alice", communityTaskFields: communityTaskFields{Entity: entity}})
	if err != nil {
		t.Fatal(err)
	}
	activity, err := server.communityCreateActivity(ctx, communityActivityCreate{Title: "Plan for call", TaskID: &task.ID})
	if err != nil {
		t.Fatal(err)
	}
	updatedActivityTitle := "Prepare for call"
	_, err = server.communityUpdateActivity(ctx, communityActivityUpdate{
		communityRevision: communityRevision{ID: activity.ID, Revision: activity.Revision},
		Title:             &updatedActivityTitle,
	})
	if err != nil {
		t.Fatal(err)
	}
	title := "Call Alice again"
	updated, err := server.communityUpdateTask(ctx, communityTaskUpdate{communityRevision: communityRevision{ID: task.ID, Revision: task.Revision}, Title: &title})
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := store.Task(ctx, task.ID)
	if err != nil || len(recorded.Activities) != 1 || recorded.Activities[0].ID != activity.ID {
		t.Fatalf("task update replaced timeline links: %+v (%v)", recorded, err)
	}
	_, err = server.communityCompleteTask(ctx, communityRevision{ID: task.ID, Revision: task.Revision})
	communityWantProblem(t, err, "conflict")
	completed, err := server.communityCompleteTask(ctx, communityRevision{ID: task.ID, Revision: updated.Revision})
	if err != nil || completed.Revision != updated.Revision+1 {
		t.Fatalf("complete task: %+v (%v)", completed, err)
	}
	recorded, err = store.Task(ctx, task.ID)
	if err != nil || recorded.Status != "completed" || len(recorded.Activities) != 2 {
		t.Fatalf("completion didn't retain links and add timeline entry: %+v (%v)", recorded, err)
	}
}

func TestCommunityFollowUpRevisionAndReadOnlyInbox(t *testing.T) {
	ctx, store, server := communityFixture(t, "contacts.read", "followups.read", "activities.read", "inbox.read", "relationships.read")
	c, err := server.communityCreateContact(ctx, communityContactCreate{Name: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	entity := communityEntity{Kind: "contact", ID: c.ID}
	before, err := server.communityFollowUp(ctx, entity)
	if err != nil || before.Revision == "" {
		t.Fatalf("read initial revision: %+v (%v)", before, err)
	}
	next := "2001-01-01"
	write, err := server.communitySetFollowUp(ctx, communityFollowUpSet{communityEntity: entity, Revision: before.Revision, NextDate: &next})
	if err != nil || write.Revision == before.Revision {
		t.Fatalf("set follow-up: %+v (%v)", write, err)
	}
	_, err = server.communitySetFollowUp(ctx, communityFollowUpSet{communityEntity: entity, Revision: before.Revision, NextDate: &next})
	communityWantProblem(t, err, "conflict")
	if err := store.SetFlag(ctx, c.ID, true, "PRIVATE flag explanation"); err != nil {
		t.Fatal(err)
	}
	items, err := server.communityInbox(ctx, communityCount{})
	if err != nil || len(items.Items) != 2 {
		t.Fatalf("unexpected inbox snapshot: %+v (%v)", items, err)
	}
	communityAssertPrivate(t, items)
	signals, err := server.communityRadar(ctx, communityCount{})
	if err != nil || len(signals.Items) != 1 || signals.Items[0].Kind != "followup" || signals.Items[0].Entity.ID != c.ID {
		t.Fatalf("unexpected radar signals: %+v (%v)", signals, err)
	}
	communityAssertPrivate(t, signals)
	if err := store.InboxAct(ctx, "flag:"+c.ID, "dismiss", ""); err == nil {
		t.Fatal("inbox snapshot persisted a new entry")
	}
	call := "call"
	if _, err := server.communityCreateActivity(ctx, communityActivityCreate{Title: "Call", communityActivityFields: communityActivityFields{Type: &call, Entity: &entity}}); err != nil {
		t.Fatal(err)
	}
	after, err := server.communityFollowUp(ctx, entity)
	if err != nil || after.NextDate != "" || after.Revision == write.Revision {
		t.Fatalf("interaction did not invalidate follow-up revision: %+v (%v)", after, err)
	}
}

func TestCommunityPipelineItemPage(t *testing.T) {
	ctx, store, server := communityFixture(t, "pipelines.read")
	p, err := store.SavePipeline(ctx, domain.Pipeline{Name: "Client intake", Stages: []domain.Stage{{Name: "New"}}})
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002"} {
		_, err := store.SaveItem(ctx, domain.PipelineItem{ID: id, PipelineID: p.ID, StageID: p.Stages[0].ID,
			Title: []string{"First", "Last"}[i], NextAction: "PRIVATE action", Notes: "PRIVATE notes"})
		if err != nil {
			t.Fatal(err)
		}
	}
	first, err := server.communityPipelineItems(ctx, communityPipelinePage{PipelineID: p.ID, Limit: 1})
	if err != nil || len(first.Items) != 1 || first.Items[0].Title != "First" || first.NextCursor == "" {
		t.Fatalf("unexpected first item page: %+v (%v)", first, err)
	}
	last, err := server.communityPipelineItems(ctx, communityPipelinePage{PipelineID: p.ID, Cursor: first.NextCursor, Limit: 1})
	if err != nil || len(last.Items) != 1 || last.Items[0].Title != "Last" || last.NextCursor != "" {
		t.Fatalf("unexpected last item page: %+v (%v)", last, err)
	}
	view, err := server.communityPipelineItem(ctx, communityKey{ID: first.Items[0].ID})
	if err != nil || view.Stage != "New" {
		t.Fatalf("unexpected pipeline item: %+v (%v)", view, err)
	}
	communityAssertPrivate(t, view)
	p.Archived = true
	if _, err := store.SavePipeline(ctx, p); err != nil {
		t.Fatal(err)
	}
	_, err = server.communityPipelineItem(ctx, communityKey{ID: view.ID})
	communityWantProblem(t, err, "not_found")
}
