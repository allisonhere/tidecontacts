package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
)

// ExportCRM reads every live CRM record in one read transaction, so the archive is a
// consistent snapshot. Deleted records and dismissed reminders are not included.
func (s *Store) ExportCRM(ctx context.Context) (domain.Archive, error) {
	a := domain.Archive{Format: domain.ArchiveFormat, Version: domain.ArchiveVersion, ExportedAt: s.now().UTC(),
		Activities: []domain.Activity{}, Tasks: []domain.Task{}, Reminders: []domain.Reminder{}, Projects: []domain.Project{}, Pipelines: []domain.ArchivePipe{},
		Follow: []domain.FollowUp{}, Dates: []domain.ImportantDate{}, Meta: []domain.ArchiveMeta{}, Collection: []domain.Collection{}}
	loc := s.now().Location()
	e := s.read(ctx, func(tx *sql.Tx) error {
		if e := each(ctx, tx, "SELECT "+activityColumns+" FROM activities a WHERE a.deleted=0 ORDER BY a.occurred_at, a.id", nil, func(r *sql.Rows) error {
			x, e := scanActivity(r, loc)
			a.Activities = append(a.Activities, x)
			return e
		}); e != nil {
			return e
		}
		if e := attachActivityLinks(ctx, tx, a.Activities, true); e != nil {
			return e
		}
		if e := each(ctx, tx, "SELECT "+taskColumns+taskFrom+"WHERE t.deleted=0 ORDER BY t.created_at, t.id", nil, func(r *sql.Rows) error {
			t, e := scanTask(r)
			a.Tasks = append(a.Tasks, t)
			return e
		}); e != nil {
			return e
		}
		if e := attachTaskLinks(ctx, tx, a.Tasks, true); e != nil {
			return e
		}
		if e := each(ctx, tx, "SELECT "+projectColumns+" FROM projects p WHERE p.deleted=0 ORDER BY p.created_at, p.id", nil, func(r *sql.Rows) error {
			p, e := scanProject(r)
			a.Projects = append(a.Projects, p)
			return e
		}); e != nil {
			return e
		}
		if e := attachProjectLinks(ctx, tx, a.Projects); e != nil {
			return e
		}
		return nil
	})
	if e != nil {
		return a, e
	}
	pipes, e := s.Pipelines(ctx, true)
	if e != nil {
		return a, e
	}
	for _, p := range pipes {
		ap := domain.ArchivePipe{Pipeline: p, Items: []domain.PipelineItem{}}
		for _, flt := range []string{"", "is:archived"} {
			q, _ := search.ParseScope("item", flt)
			items, e := s.Items(ctx, p.ID, q)
			if e != nil {
				return a, e
			}
			for _, it := range items {
				full, e := s.Item(ctx, it.ID)
				if e != nil {
					return a, e
				}
				ap.Items = append(ap.Items, full)
			}
		}
		a.Pipelines = append(a.Pipelines, ap)
	}
	if a.Reminders, e = s.Reminders(ctx, false); e != nil {
		return a, e
	}
	if a.Collection, e = s.Collections(ctx); e != nil {
		return a, e
	}
	e = s.read(ctx, func(tx *sql.Tx) error {
		if e := each(ctx, tx, "SELECT entity_id,every,unit,never,next_date,snoozed_until,set_at FROM followup_prefs ORDER BY entity_id", nil, func(r *sql.Rows) error {
			var f domain.FollowUp
			e := r.Scan(&f.Entity.ID, &f.Interval.Every, &f.Interval.Unit, &f.Never, &f.NextDate, &f.SnoozedUntil, &f.SetAt)
			a.Follow = append(a.Follow, f)
			return e
		}); e != nil {
			return e
		}
		if e := each(ctx, tx, "SELECT id,entity_id,label,date,annual,notes FROM important_dates ORDER BY id", nil, func(r *sql.Rows) error {
			var d domain.ImportantDate
			e := r.Scan(&d.ID, &d.Entity.ID, &d.Label, &d.Date, &d.Annual, &d.Notes)
			a.Dates = append(a.Dates, d)
			return e
		}); e != nil {
			return e
		}
		return each(ctx, tx, "SELECT entity_id,notes,flagged,flag_note FROM relationship_meta ORDER BY entity_id", nil, func(r *sql.Rows) error {
			var m domain.ArchiveMeta
			e := r.Scan(&m.Entity.ID, &m.Notes, &m.Flagged, &m.FlagNote)
			a.Meta = append(a.Meta, m)
			return e
		})
	})
	return a, e
}

// ImportCRM adds the archive's records in one transaction. Records whose ID already
// exists are skipped, never overwritten. Links to records that do not exist here are
// dropped and reported. A failure anywhere rolls the whole import back.
func (s *Store) ImportCRM(ctx context.Context, a domain.Archive) (domain.ImportReport, error) {
	rep := domain.NewImportReport()
	if a.Format != domain.ArchiveFormat {
		return rep, fmt.Errorf("this is not a TideContacts CRM export")
	}
	if a.Version != domain.ArchiveVersion {
		return rep, fmt.Errorf("export version %d is not supported by this build (supports %d)", a.Version, domain.ArchiveVersion)
	}
	now := s.now()
	e := s.write(ctx, func(tx *sql.Tx) error {
		exists := func(table, id string) bool {
			var n int
			return tx.QueryRowContext(ctx, "SELECT count(*) FROM "+table+" WHERE id=?", id).Scan(&n) == nil && n > 0
		}
		warned := map[string]bool{}
		warn := func(msg string) {
			if !warned[msg] && len(rep.Warnings) < 50 {
				warned[msg] = true
				rep.Warnings = append(rep.Warnings, msg)
			}
		}
		// keep returns the references that resolve locally, by ID only.
		keep := func(table string, refs []domain.Ref, owner string) []domain.Ref {
			out := []domain.Ref{}
			for _, r := range refs {
				if r.ID != "" && exists(table, r.ID) {
					out = append(out, domain.Ref{ID: r.ID, Role: r.Role})
				} else {
					name := orDefault(r.Name, r.ID)
					warn(fmt.Sprintf("%s: link to %s %q dropped (not in this database)", owner, map[string]string{"entities": "contact", "projects": "project", "tasks": "task", "activities": "activity"}[table], name))
				}
			}
			return out
		}
		for _, p := range a.Projects {
			if exists("projects", p.ID) {
				rep.Skipped["projects"]++
				continue
			}
			p.Revision, p.Members = 0, keep("entities", p.Members, "project "+p.Name)
			if e := saveProject(ctx, tx, &p, now); e != nil {
				return fmt.Errorf("project %q: %w", p.Name, e)
			}
			rep.Added["projects"]++
		}
		for _, t := range a.Tasks {
			if exists("tasks", t.ID) {
				rep.Skipped["tasks"]++
				continue
			}
			t.Revision, t.Activities = 0, nil
			t.Entities, t.Projects = keep("entities", t.Entities, "task "+t.Title), keep("projects", t.Projects, "task "+t.Title)
			if e := saveTask(ctx, tx, &t, now, true); e != nil {
				return fmt.Errorf("task %q: %w", t.Title, e)
			}
			rep.Added["tasks"]++
		}
		for _, x := range a.Activities {
			dup := exists("activities", x.ID)
			if !dup && x.ExternalRef != "" {
				var n int
				dup = tx.QueryRowContext(ctx, "SELECT count(*) FROM activities WHERE source_app=? AND external_ref=?", x.SourceApp, x.ExternalRef).Scan(&n) == nil && n > 0
			}
			if dup {
				rep.Skipped["activities"]++
				continue
			}
			x.Revision = 0
			x.Entities, x.Projects, x.Tasks = keep("entities", x.Entities, "activity "+x.Title), keep("projects", x.Projects, "activity "+x.Title), keep("tasks", x.Tasks, "activity "+x.Title)
			if e := saveActivity(ctx, tx, &x, now, true); e != nil {
				return fmt.Errorf("activity %q: %w", x.Title, e)
			}
			rep.Added["activities"]++
		}
		for _, p := range a.Pipelines {
			if exists("pipelines", p.ID) {
				rep.Skipped["pipelines"]++
				rep.Skipped["items"] += len(p.Items)
				continue
			}
			if p.Name == "" || len(p.Stages) == 0 {
				return fmt.Errorf("pipeline %q has no name or stages", p.Name)
			}
			at := stamp(now)
			if _, e := tx.ExecContext(ctx, "INSERT INTO pipelines(id,name,description,archived,position,created_at,updated_at) VALUES(?,?,?,?,(SELECT COALESCE(MAX(position),-1)+1 FROM pipelines),?,?)", p.ID, p.Name, p.Description, p.Archived, at, at); e != nil {
				return fmt.Errorf("pipeline %q: %w", p.Name, e)
			}
			for i, g := range p.Stages {
				if _, e := tx.ExecContext(ctx, "INSERT INTO pipeline_stages(id,pipeline_id,name,position) VALUES(?,?,?,?)", g.ID, p.ID, g.Name, i); e != nil {
					return fmt.Errorf("pipeline %q stage %q: %w", p.Name, g.Name, e)
				}
			}
			rep.Added["pipelines"]++
			for _, it := range p.Items {
				if _, e := tx.ExecContext(ctx, `INSERT INTO pipeline_items(id,pipeline_id,stage_id,title,notes,next_action,stage_entered_at,archived,deleted,created_at,updated_at,revision) VALUES(?,?,?,?,?,?,?,?,0,?,?,1)`,
					it.ID, p.ID, it.StageID, it.Title, it.Notes, it.NextAction, orDefault(it.StageEnteredAt, at), it.Archived, orDefault(it.CreatedAt, at), at); e != nil {
					return fmt.Errorf("item %q: %w", it.Title, e)
				}
				for _, h := range it.History {
					if _, e := tx.ExecContext(ctx, "INSERT INTO pipeline_stage_history(item_id,from_stage,to_stage,at) VALUES(?,?,?,?)", it.ID, h.From, h.To, h.At); e != nil {
						return e
					}
				}
				owner := "item " + it.Title
				for target, refs := range map[string][]domain.Ref{"entity": keep("entities", it.Entities, owner), "project": keep("projects", it.Projects, owner), "task": keep("tasks", it.Tasks, owner), "activity": keep("activities", it.Activities, owner)} {
					if e := setLinks(ctx, tx, "item", it.ID, target, refs); e != nil {
						return e
					}
				}
				if e := setTags(ctx, tx, "item", it.ID, it.Tags); e != nil {
					return e
				}
				if e := reindexRecord(ctx, tx, "item", it.ID); e != nil {
					return e
				}
				rep.Added["items"]++
			}
		}
		for _, m := range a.Reminders {
			if exists("reminders", m.ID) {
				rep.Skipped["reminders"]++
				continue
			}
			var rule, task, entity any
			if !m.Recurrence.IsZero() {
				rule = m.ID
				if e := saveRule(ctx, tx, m.ID, m.Recurrence); e != nil {
					return e
				}
			}
			if m.Task.ID != "" && exists("tasks", m.Task.ID) {
				task = m.Task.ID
			}
			if m.Entity.ID != "" && exists("entities", m.Entity.ID) {
				entity = m.Entity.ID
			} else if m.Entity.ID != "" {
				warn(fmt.Sprintf("reminder %q: link to contact %q dropped (not in this database)", m.Title, orDefault(m.Entity.Name, m.Entity.ID)))
			}
			snoozed := ""
			if !m.SnoozedUntil.IsZero() {
				snoozed = stamp(m.SnoozedUntil)
			}
			if _, e := tx.ExecContext(ctx, `INSERT INTO reminders(id,title,note,remind_at,timezone,recurrence_id,task_id,entity_id,status,snoozed_until,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,'pending',?,?,?)`,
				m.ID, m.Title, m.Note, stamp(m.At), m.Timezone, rule, task, entity, snoozed, stamp(now), stamp(now)); e != nil {
				return fmt.Errorf("reminder %q: %w", m.Title, e)
			}
			rep.Added["reminders"]++
		}
		for _, f := range a.Follow {
			if !exists("entities", f.Entity.ID) {
				warn("follow-up schedule for a contact not in this database skipped")
				continue
			}
			unit := orDefault(f.Interval.Unit, "day")
			r, e := tx.ExecContext(ctx, "INSERT OR IGNORE INTO followup_prefs(entity_id,every,unit,never,next_date,snoozed_until,set_at,updated_at) VALUES(?,?,?,?,?,?,?,?)",
				f.Entity.ID, f.Interval.Every, unit, f.Never, f.NextDate, f.SnoozedUntil, orDefault(f.SetAt, domain.Day(now)), stamp(now))
			if e != nil {
				return e
			}
			if n, _ := r.RowsAffected(); n > 0 {
				rep.Added["follow-ups"]++
			} else {
				rep.Skipped["follow-ups"]++
			}
		}
		for _, d := range a.Dates {
			if exists("important_dates", d.ID) {
				rep.Skipped["dates"]++
				continue
			}
			if !exists("entities", d.Entity.ID) || !domain.DateValid(d.Date) || d.Label == "" || d.Date == "" {
				warn("an important date for a missing contact or with an invalid date was skipped")
				continue
			}
			if _, e := tx.ExecContext(ctx, "INSERT INTO important_dates(id,entity_id,label,date,annual,notes,created_at) VALUES(?,?,?,?,?,?,?)", d.ID, d.Entity.ID, d.Label, d.Date, d.Annual, d.Notes, stamp(now)); e != nil {
				return e
			}
			rep.Added["dates"]++
		}
		for _, m := range a.Meta {
			if !exists("entities", m.Entity.ID) {
				continue
			}
			r, e := tx.ExecContext(ctx, "INSERT OR IGNORE INTO relationship_meta(entity_id,notes,flagged,flag_note,flagged_at,updated_at) VALUES(?,?,?,?,?,?)", m.Entity.ID, m.Notes, m.Flagged, m.FlagNote, stamp(now), stamp(now))
			if e != nil {
				return e
			}
			if n, _ := r.RowsAffected(); n > 0 {
				rep.Added["relationship notes"]++
			} else {
				rep.Skipped["relationship notes"]++
			}
		}
		for _, c := range a.Collection {
			c.Normalize()
			if e := c.Validate(); e != nil {
				warn(fmt.Sprintf("collection %q skipped: %v", c.Name, e))
				continue
			}
			if _, e := search.ParseScope(c.Entity, c.Query); e != nil {
				warn(fmt.Sprintf("collection %q skipped: %v", c.Name, e))
				continue
			}
			r, e := tx.ExecContext(ctx, `INSERT OR IGNORE INTO collections(id,name,description,entity,query,sort,grouping,pinned,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
				c.ID, c.Name, c.Description, c.Entity, c.Query, c.Sort, c.Group, c.Pinned, stamp(now), stamp(now))
			if e != nil {
				return e
			}
			if n, _ := r.RowsAffected(); n > 0 {
				rep.Added["collections"]++
			} else {
				rep.Skipped["collections"]++
			}
		}
		return nil
	})
	if e != nil {
		return domain.NewImportReport(), e
	}
	return rep, nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
