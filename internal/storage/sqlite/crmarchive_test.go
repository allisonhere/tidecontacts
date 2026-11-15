package sqlite

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
)

func TestRestoreBringsTombstonesBack(t *testing.T) {
	s, _ := crmStore(t)
	alex := person(t, s, "Alex Rivera")
	task := must[domain.Task](t)(s.SaveTask(bg, domain.Task{Title: "Draft plan", Entities: []domain.Ref{{ID: alex.ID}}}))
	act := must[domain.Activity](t)(s.SaveActivity(bg, domain.Activity{Type: "call", Title: "Intro call", Entities: []domain.Ref{{ID: alex.ID}}}))
	proj := must[domain.Project](t)(s.SaveProject(bg, domain.Project{Name: "Harbor"}))
	pipe := must[domain.Pipeline](t)(s.SavePipeline(bg, domain.Pipeline{Name: "P", Stages: []domain.Stage{{Name: "S"}}}))
	item := must[domain.PipelineItem](t)(s.SaveItem(bg, domain.PipelineItem{PipelineID: pipe.ID, Title: "Deal"}))
	for _, e := range []error{s.DeleteTask(bg, task.ID), s.DeleteActivity(bg, act.ID), s.DeleteProject(bg, proj.ID), s.DeleteItem(bg, item.ID)} {
		if e != nil {
			t.Fatal(e)
		}
	}
	deleted := func() [4]int {
		tasks := must[[]domain.Task](t)(s.Tasks(bg, TaskFilter{Query: q(t, "task", "is:deleted")}))
		acts := must[[]domain.Activity](t)(s.Activities(bg, ActivityFilter{Query: q(t, "activity", "is:deleted")}))
		projects := must[[]domain.Project](t)(s.Projects(bg, "", q(t, "project", "is:deleted")))
		items := must[[]domain.PipelineItem](t)(s.Items(bg, "", q(t, "item", "is:deleted")))
		return [4]int{len(tasks), len(acts), len(projects), len(items)}
	}
	live := func() [4]int {
		tasks := must[[]domain.Task](t)(s.Tasks(bg, TaskFilter{}))
		acts := must[[]domain.Activity](t)(s.Activities(bg, ActivityFilter{}))
		projects := must[[]domain.Project](t)(s.Projects(bg, "all", search.Query{}))
		items := must[[]domain.PipelineItem](t)(s.Items(bg, "", search.Query{}))
		return [4]int{len(tasks), len(acts), len(projects), len(items)}
	}
	if deleted() != [4]int{1, 1, 1, 1} || live() != [4]int{} {
		t.Fatal("tombstones not listed apart from live records", deleted(), live())
	}
	for kind, id := range map[string]string{"task": task.ID, "activity": act.ID, "project": proj.ID, "item": item.ID} {
		for i := 0; i < 2; i++ { // restoring twice is harmless
			if e := s.Restore(bg, kind, id); e != nil {
				t.Fatal(kind, e)
			}
		}
	}
	if deleted() != [4]int{} || live() != [4]int{1, 1, 1, 1} {
		t.Fatal("restore incomplete", deleted(), live())
	}
	// Restored records are searchable and keep their links.
	if got := must[[]domain.Task](t)(s.Tasks(bg, TaskFilter{Query: q(t, "task", "draft with:alex")})); len(got) != 1 {
		t.Fatal("restored task not searchable")
	}
	if got := must[[]domain.Activity](t)(s.Activities(bg, ActivityFilter{EntityID: alex.ID})); len(got) != 1 {
		t.Fatal("restored activity lost its person")
	}
	if e := s.Restore(bg, "task", "missing"); e == nil {
		t.Fatal("restored a record that never existed")
	}
	if e := s.Restore(bg, "contact", alex.ID); e == nil {
		t.Fatal("contacts have their own restore")
	}
}

// seedCRM builds a database that touches every archived record type.
func seedCRM(t *testing.T, s *Store, now time.Time) (domain.Contact, domain.Contact) {
	t.Helper()
	alex, sam := person(t, s, "Alex Rivera"), person(t, s, "Sam Lee")
	project := must[domain.Project](t)(s.SaveProject(bg, domain.Project{Name: "Harbor", Members: []domain.Ref{{ID: alex.ID, Role: "lead"}}, Tags: []string{"client"},
		Milestones: []domain.Milestone{{Title: "Brief", Due: "2026-11-20"}}}))
	rule, _ := domain.ParseRecurrence("weekly")
	task := must[domain.Task](t)(s.SaveTask(bg, domain.Task{Title: "Weekly review", Status: "scheduled", Due: "2026-11-20", DueTime: "09:00", Recurrence: rule,
		Entities: []domain.Ref{{ID: alex.ID}}, Projects: []domain.Ref{{ID: project.ID}}, Tags: []string{"routine"}}))
	must[domain.Task](t)(s.SaveTask(bg, domain.Task{Title: "Unsorted"}))
	must[domain.Activity](t)(s.SaveActivity(bg, domain.Activity{Type: "meeting", Title: "Kickoff", Body: "Notes\nsecond line", OccurredAt: now.Add(-time.Hour),
		Entities: []domain.Ref{{ID: alex.ID}, {ID: sam.ID}}, Projects: []domain.Ref{{ID: project.ID}}, Tasks: []domain.Ref{{ID: task.ID}}, Tags: []string{"planning"}}))
	must[domain.Activity](t)(s.UpsertExternalActivity(bg, domain.Activity{Type: "email", Title: "Re: proposal", SourceApp: "tidemail", ExternalRef: "<m1@x>", ExternalURL: "https://mail.example/1", OccurredAt: now.Add(-2 * time.Hour)}, nil))
	pipe := must[domain.Pipeline](t)(s.SavePipeline(bg, domain.Pipeline{Name: "Applications", Stages: []domain.Stage{{Name: "Applied"}, {Name: "Interview"}}}))
	item := must[domain.PipelineItem](t)(s.SaveItem(bg, domain.PipelineItem{PipelineID: pipe.ID, Title: "Riverside", NextAction: "Send portfolio", Entities: []domain.Ref{{ID: alex.ID}}, Tasks: []domain.Ref{{ID: task.ID}}, Tags: []string{"design"}}))
	if e := s.MoveItem(bg, item.ID, pipe.Stages[1].ID); e != nil {
		t.Fatal(e)
	}
	must[domain.Reminder](t)(s.SaveReminder(bg, domain.Reminder{Title: "Ping Alex", At: now.Add(24 * time.Hour), Entity: domain.Ref{ID: alex.ID}, Recurrence: rule}))
	if e := s.SetFollowUp(bg, domain.FollowUp{Entity: domain.Ref{ID: alex.ID}, Interval: domain.Interval{Every: 2, Unit: "week"}}); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveDate(bg, domain.ImportantDate{Entity: domain.Ref{ID: sam.ID}, Label: "First met", Date: "2019-03-03", Annual: true}); e != nil {
		t.Fatal(e)
	}
	if e := s.SetFlag(bg, sam.ID, true, "verify"); e != nil {
		t.Fatal(e)
	}
	if e := s.SetRelationshipNotes(bg, alex.ID, "Met at the harbor meetup"); e != nil {
		t.Fatal(e)
	}
	must[domain.Collection](t)(s.SaveCollection(bg, domain.Collection{Name: "Open Harbor", Entity: "task", Query: "is:open project:Harbor", Pinned: true}))
	// Deleted records must not travel.
	gone := must[domain.Task](t)(s.SaveTask(bg, domain.Task{Title: "Deleted task"}))
	if e := s.DeleteTask(bg, gone.ID); e != nil {
		t.Fatal(e)
	}
	return alex, sam
}

func TestExportImportRoundTrip(t *testing.T) {
	a, now := crmStore(t)
	alex, sam := seedCRM(t, a, *now)
	archive := must[domain.Archive](t)(a.ExportCRM(bg))
	if len(archive.Tasks) != 2 || len(archive.Activities) != 2 || len(archive.Projects) != 1 || len(archive.Pipelines) != 1 || len(archive.Pipelines[0].Items) != 1 ||
		len(archive.Reminders) != 1 || len(archive.Follow) != 1 || len(archive.Dates) != 1 || len(archive.Meta) != 2 || len(archive.Collection) != 1 {
		t.Fatalf("export counts: %+v", archive)
	}
	// The file form survives a JSON round trip unchanged.
	raw, e := json.Marshal(archive)
	if e != nil {
		t.Fatal(e)
	}
	var decoded domain.Archive
	if e = json.Unmarshal(raw, &decoded); e != nil {
		t.Fatal(e)
	}

	// A second database that already has the same contacts (same IDs).
	b, clockB := crmStore(t)
	*clockB = *now
	for _, c := range []domain.Contact{alex, sam} {
		c.Revision = 0
		if _, e := b.SaveContacts(bg, []domain.Contact{c}, "create"); e != nil {
			t.Fatal(e)
		}
	}
	rep := must[domain.ImportReport](t)(b.ImportCRM(bg, decoded))
	if len(rep.Warnings) != 0 || rep.Added["tasks"] != 2 || rep.Added["activities"] != 2 || rep.Added["projects"] != 1 || rep.Added["pipelines"] != 1 || rep.Added["items"] != 1 ||
		rep.Added["reminders"] != 1 || rep.Added["follow-ups"] != 1 || rep.Added["dates"] != 1 || rep.Added["collections"] != 1 {
		t.Fatalf("%+v", rep)
	}
	again := must[domain.Archive](t)(b.ExportCRM(bg))
	if len(again.Tasks) != 2 || len(again.Activities) != 2 {
		t.Fatal("imported data not exported again")
	}
	if got := must[[]domain.Task](t)(b.Tasks(bg, TaskFilter{Query: q(t, "task", "is:recurring with:alex project:harbor tag:routine")})); len(got) != 1 || got[0].Recurrence.String() != "weekly" || got[0].DueTime != "09:00" {
		t.Fatalf("task fidelity: %+v", got)
	}
	acts := must[[]domain.Activity](t)(b.Activities(bg, ActivityFilter{Query: q(t, "activity", "source:external")}))
	if len(acts) != 1 || acts[0].SourceApp != "tidemail" || acts[0].ExternalRef != "<m1@x>" {
		t.Fatalf("provenance lost: %+v", acts)
	}
	kick := must[[]domain.Activity](t)(b.Activities(bg, ActivityFilter{Query: q(t, "activity", "kickoff")}))
	if len(kick) != 1 || !kick[0].OccurredAt.Equal(now.Add(-time.Hour)) || len(kick[0].Entities) != 2 || len(kick[0].Tasks) != 0 {
		t.Fatalf("%+v", kick)
	}
	full := must[domain.Activity](t)(b.Activity(bg, kick[0].ID))
	if len(full.Tasks) != 1 || full.Body != "Notes\nsecond line" || full.Tags[0] != "planning" {
		t.Fatalf("%+v", full)
	}
	items := must[[]domain.PipelineItem](t)(b.Items(bg, "", q(t, "item", "stage:interview with:alex tag:design")))
	if len(items) != 1 {
		t.Fatal("item missing")
	}
	if it := must[domain.PipelineItem](t)(b.Item(bg, items[0].ID)); len(it.History) != 2 || it.History[1].To != "Interview" || len(it.Tasks) != 1 {
		t.Fatalf("history lost: %+v", it)
	}
	if f := must[domain.FollowUp](t)(b.FollowUp(bg, alex.ID)); f.Interval.String() != "every two weeks" {
		t.Fatal(f)
	}
	if c := must[domain.Connections](t)(b.Connections(bg, sam.ID)); !c.Flagged || len(c.Dates) != 1 {
		t.Fatal("meta/dates lost", c)
	}
	if list := must[[]domain.Collection](t)(b.Collections(bg)); len(list) != 1 || !list[0].Pinned {
		t.Fatal(list)
	}
	if e := b.db.QueryRow("PRAGMA foreign_key_check").Scan(new(string), new(string), new(string), new(string)); e == nil {
		t.Fatal("foreign key violations after import")
	}

	// Importing the same file again changes nothing and overwrites nothing.
	local := must[[]domain.Task](t)(b.Tasks(bg, TaskFilter{Query: q(t, "task", "unsorted")}))[0]
	local.Title = "Edited locally"
	local = must[domain.Task](t)(b.SaveTask(bg, local))
	rep = must[domain.ImportReport](t)(b.ImportCRM(bg, decoded))
	for _, k := range []string{"tasks", "activities", "projects", "pipelines", "items", "reminders", "collections"} {
		if rep.Added[k] != 0 || rep.Skipped[k] == 0 {
			t.Errorf("%s: %+v", k, rep)
		}
	}
	if got := must[domain.Task](t)(b.Task(bg, local.ID)); got.Title != "Edited locally" || got.Revision != local.Revision {
		t.Fatal("import overwrote an existing record")
	}
	if n := len(must[domain.Archive](t)(b.ExportCRM(bg)).Tasks); n != 2 {
		t.Fatal("duplicates after re-import", n)
	}

	// A database without those contacts still gets the records; the links are reported.
	c, _ := crmStore(t)
	rep = must[domain.ImportReport](t)(c.ImportCRM(bg, decoded))
	if rep.Added["tasks"] != 2 || len(rep.Warnings) == 0 || !strings.Contains(strings.Join(rep.Warnings, "\n"), "not in this database") {
		t.Fatalf("%+v", rep)
	}
	if got := must[[]domain.Task](t)(c.Tasks(bg, TaskFilter{Query: q(t, "task", "weekly")})); len(got) != 1 || len(got[0].Entities) != 0 {
		t.Fatal("dangling link imported", got)
	}
	if e := c.db.QueryRow("PRAGMA foreign_key_check").Scan(new(string), new(string), new(string), new(string)); e == nil {
		t.Fatal("foreign key violations after import without contacts")
	}
}

func TestImportIsAllOrNothing(t *testing.T) {
	s, now := crmStore(t)
	good := domain.Task{ID: "11111111-1111-1111-1111-111111111111", Title: "Would be added", Status: "next"}
	broken := domain.ArchivePipe{Pipeline: domain.Pipeline{ID: "p1", Name: "Broken", Stages: []domain.Stage{{ID: "s1", Name: "A"}, {ID: "s1", Name: "B"}}}}
	a := domain.Archive{Format: domain.ArchiveFormat, Version: domain.ArchiveVersion, ExportedAt: *now, Tasks: []domain.Task{good}, Pipelines: []domain.ArchivePipe{broken}}
	if _, e := s.ImportCRM(bg, a); e == nil {
		t.Fatal("import with a broken pipeline succeeded")
	}
	if n := len(must[domain.Archive](t)(s.ExportCRM(bg)).Tasks); n != 0 {
		t.Fatal("failed import left records behind", n)
	}
	for _, bad := range []domain.Archive{{}, {Format: domain.ArchiveFormat, Version: 7}} {
		if _, e := s.ImportCRM(bg, bad); e == nil {
			t.Fatal("accepted an invalid archive header")
		}
	}
}
