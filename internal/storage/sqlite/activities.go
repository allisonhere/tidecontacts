package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/google/uuid"
)

// ActivityFilter is defined by the application layer.
type ActivityFilter = app.ActivityFilter

const activityColumns = `a.id,a.type,a.title,a.body,a.occurred_at,a.timezone,a.provenance,a.source_app,a.external_ref,a.external_url,a.visibility,a.deleted,a.created_at,a.updated_at,a.revision`

func scanActivity(r interface{ Scan(...any) error }, loc *time.Location) (domain.Activity, error) {
	var a domain.Activity
	var at string
	e := r.Scan(&a.ID, &a.Type, &a.Title, &a.Body, &at, &a.Timezone, &a.Provenance, &a.SourceApp, &a.ExternalRef, &a.ExternalURL, &a.Visibility, &a.Deleted, &a.CreatedAt, &a.UpdatedAt, &a.Revision)
	a.OccurredAt = parseStamp(at).In(loc)
	return a, e
}

func attachActivityLinks(ctx context.Context, q querier, list []domain.Activity, full bool) error {
	ids := make([]string, len(list))
	for i, a := range list {
		ids[i] = a.ID
	}
	entities, e := links(ctx, q, "activity", "entity", ids)
	if e != nil {
		return e
	}
	projects, e := links(ctx, q, "activity", "project", ids)
	if e != nil {
		return e
	}
	var tasks map[string][]domain.Ref
	var tags map[string][]string
	if full {
		if tasks, e = links(ctx, q, "activity", "task", ids); e != nil {
			return e
		}
		if tags, e = tagsOf(ctx, q, "activity", ids); e != nil {
			return e
		}
	}
	for i := range list {
		id := list[i].ID
		list[i].Entities, list[i].Projects, list[i].Tasks, list[i].Tags = entities[id], projects[id], tasks[id], tags[id]
	}
	return nil
}

// Activities returns one bounded page; callers page with Before rather than loading a history.
func (s *Store) Activities(ctx context.Context, f ActivityFilter) ([]domain.Activity, error) {
	w := where{}
	w.add("a.deleted=0")
	today := s.today()
	loc := s.now().Location()
	dayStart := func(word string, plus int) (string, error) {
		day, e := domain.ParseWhen(word, today)
		if e != nil {
			return "", e
		}
		t, _ := domain.At(domain.AddDays(day, plus), "", loc)
		return stamp(t), nil
	}
	for _, flt := range f.Query.Filters {
		var e error
		var from, to string
		switch flt.Key {
		case "type":
			w.add("a.type=?", strings.ToLower(flt.Value))
		case "source":
			w.add("a.provenance=?", strings.ToLower(flt.Value))
		case "app":
			w.add("a.source_app=? COLLATE NOCASE", flt.Value)
		case "after":
			if from, e = dayStart(flt.Value, 0); e == nil {
				w.add("a.occurred_at>=?", from)
			}
		case "before":
			if to, e = dayStart(flt.Value, 0); e == nil {
				w.add("a.occurred_at<?", to)
			}
		case "on":
			if from, e = dayStart(flt.Value, 0); e == nil {
				to, _ = dayStart(flt.Value, 1)
				w.add("a.occurred_at>=? AND a.occurred_at<?", from, to)
			}
		}
		if e != nil {
			return nil, e
		}
	}
	w.linked(f.Query, "activity", "a")
	w.terms(f.Query, "activity", "a", "title")
	if f.EntityID != "" {
		w.add("EXISTS(SELECT 1 FROM activity_entities j WHERE j.activity_id=a.id AND j.entity_id=?)", f.EntityID)
	}
	if f.ProjectID != "" {
		w.add("EXISTS(SELECT 1 FROM activity_projects j WHERE j.activity_id=a.id AND j.project_id=?)", f.ProjectID)
	}
	if f.TaskID != "" {
		w.add("EXISTS(SELECT 1 FROM activity_tasks j WHERE j.activity_id=a.id AND j.task_id=?)", f.TaskID)
	}
	if !f.Before.IsZero() {
		w.add("a.occurred_at<?", stamp(f.Before))
	}
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	list := []domain.Activity{}
	e := s.read(ctx, func(tx *sql.Tx) error {
		found := f.Include == ""
		if e := each(ctx, tx, "SELECT "+activityColumns+" FROM activities a WHERE "+w.sql()+" ORDER BY a.occurred_at DESC, a.id LIMIT ?", append(w.args, limit), func(r *sql.Rows) error {
			a, e := scanActivity(r, loc)
			found = found || a.ID == f.Include
			list = append(list, a)
			return e
		}); e != nil {
			return e
		}
		if !found {
			if a, e := scanActivity(tx.QueryRowContext(ctx, "SELECT "+activityColumns+" FROM activities a WHERE a.id=?", f.Include), loc); e == nil {
				i := 0
				for i < len(list) && list[i].OccurredAt.After(a.OccurredAt) {
					i++
				}
				list = append(list[:i], append([]domain.Activity{a}, list[i:]...)...)
			}
		}
		return attachActivityLinks(ctx, tx, list, false)
	})
	return list, e
}

func (s *Store) Activity(ctx context.Context, id string) (domain.Activity, error) {
	var a domain.Activity
	e := s.read(ctx, func(tx *sql.Tx) error {
		var e error
		if a, e = scanActivity(tx.QueryRowContext(ctx, "SELECT "+activityColumns+" FROM activities a WHERE a.id=?", id), s.now().Location()); e != nil {
			return e
		}
		list := []domain.Activity{a}
		e = attachActivityLinks(ctx, tx, list, true)
		a = list[0]
		return e
	})
	return a, e
}

// ActivityTypes lists built-in and custom types that are in use or defined.
func (s *Store) ActivityTypes(ctx context.Context) ([]string, error) {
	out := []string{}
	e := each(ctx, s.db, "SELECT name FROM activity_types ORDER BY rowid", nil, func(r *sql.Rows) error {
		var v string
		e := r.Scan(&v)
		out = append(out, v)
		return e
	})
	return out, e
}

// SaveActivity creates or edits a locally owned activity. Records owned by another
// application are read-only here; connectors use UpsertExternalActivity.
func (s *Store) SaveActivity(ctx context.Context, a domain.Activity) (domain.Activity, error) {
	e := s.write(ctx, func(tx *sql.Tx) error {
		var provenance string
		err := tx.QueryRowContext(ctx, "SELECT provenance FROM activities WHERE id=?", a.ID).Scan(&provenance)
		if err == nil && provenance != domain.Local {
			return fmt.Errorf("this activity is owned by its source application; link or annotate it, but edit the original there")
		}
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if e := saveActivity(ctx, tx, &a, s.now(), true); e != nil {
			return e
		}
		if !domain.IsInteraction(a.Type) || a.Deleted {
			return nil
		}
		// A recorded interaction satisfies an explicit follow-up date that has arrived.
		day := domain.Day(a.OccurredAt.In(s.now().Location()))
		_, e := tx.ExecContext(ctx, `UPDATE followup_prefs SET next_date='',snoozed_until='' WHERE next_date!='' AND next_date<=?
 AND entity_id IN (SELECT entity_id FROM activity_entities WHERE activity_id=?)`, day, a.ID)
		return e
	})
	if e != nil {
		return a, e
	}
	return s.Activity(ctx, a.ID)
}

func saveActivity(ctx context.Context, tx *sql.Tx, a *domain.Activity, now time.Time, replaceLinks bool) error {
	a.Normalize(now)
	if e := a.Validate(); e != nil {
		return e
	}
	var revision int
	var created string
	e := tx.QueryRowContext(ctx, "SELECT revision,created_at FROM activities WHERE id=?", a.ID).Scan(&revision, &created)
	if e != nil && e != sql.ErrNoRows {
		return e
	}
	if e == nil && a.Revision != revision {
		return fmt.Errorf("activity changed since it was opened; reload before saving")
	}
	if e == sql.ErrNoRows {
		created = stamp(now)
	}
	a.CreatedAt, a.UpdatedAt, a.Revision = created, stamp(now), revision+1
	if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO activity_types(name,interaction) VALUES(?,?)", a.Type, domain.IsInteraction(a.Type)); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO activities(id,type,title,body,occurred_at,timezone,provenance,source_app,external_ref,external_url,visibility,deleted,created_at,updated_at,revision)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET type=excluded.type,title=excluded.title,body=excluded.body,occurred_at=excluded.occurred_at,
 timezone=excluded.timezone,provenance=excluded.provenance,source_app=excluded.source_app,external_ref=excluded.external_ref,external_url=excluded.external_url,
 visibility=excluded.visibility,deleted=excluded.deleted,updated_at=excluded.updated_at,revision=excluded.revision`,
		a.ID, a.Type, a.Title, a.Body, stamp(a.OccurredAt), a.Timezone, a.Provenance, a.SourceApp, a.ExternalRef, a.ExternalURL, a.Visibility, a.Deleted, a.CreatedAt, a.UpdatedAt, a.Revision); e != nil {
		return e
	}
	if replaceLinks {
		for target, refs := range map[string][]domain.Ref{"entity": a.Entities, "project": a.Projects, "task": a.Tasks} {
			if e = setLinks(ctx, tx, "activity", a.ID, target, refs); e != nil {
				return e
			}
		}
		if e = setTags(ctx, tx, "activity", a.ID, a.Tags); e != nil {
			return e
		}
	}
	return reindexRecord(ctx, tx, "activity", a.ID)
}

// UpsertExternalActivity is the idempotent entry point for imports and connectors.
// The pair (SourceApp, ExternalRef) identifies the record: repeating a call updates
// the same row and never duplicates it. Links supplied by the source are added to,
// never removed from, the links a user made locally, and nothing is ever deleted
// because a source is unavailable. Participants given as email addresses are linked
// when exactly one live contact owns that address; others are left unlinked.
func (s *Store) UpsertExternalActivity(ctx context.Context, a domain.Activity, emails []string) (domain.Activity, error) {
	if a.Provenance == "" || a.Provenance == domain.Local {
		a.Provenance = domain.External
	}
	e := s.write(ctx, func(tx *sql.Tx) error {
		if a.SourceApp == "" || a.ExternalRef == "" {
			return fmt.Errorf("external activities need a source application and reference")
		}
		existing := false
		err := tx.QueryRowContext(ctx, "SELECT id,revision FROM activities WHERE source_app=? AND external_ref=?", a.SourceApp, a.ExternalRef).Scan(&a.ID, &a.Revision)
		if err == nil {
			existing = true
		} else if err != sql.ErrNoRows {
			return err
		} else {
			a.ID, a.Revision = uuid.NewString(), 0
		}
		refs := append([]domain.Ref{}, a.Entities...)
		for _, addr := range emails {
			ids := []string{}
			if e := each(ctx, tx, "SELECT DISTINCT m.entity_id FROM methods m JOIN entities e ON e.id=m.entity_id WHERE m.kind='email' AND m.normalized=? AND e.deleted=0", []any{domain.NormalizeMatch(addr)}, func(r *sql.Rows) error {
				var id string
				e := r.Scan(&id)
				ids = append(ids, id)
				return e
			}); e != nil {
				return e
			}
			if len(ids) == 1 {
				refs = append(refs, domain.Ref{ID: ids[0]})
			}
		}
		if e := saveActivity(ctx, tx, &a, s.now(), !existing); e != nil {
			return e
		}
		for target, list := range map[string][]domain.Ref{"entity": refs, "project": a.Projects, "task": a.Tasks} {
			resolved, e := resolve(ctx, tx, target, list)
			if e != nil {
				return e
			}
			table, ownerCol, targetCol, _ := junction("activity", target)
			for _, r := range resolved {
				if _, e = tx.ExecContext(ctx, fmt.Sprintf("INSERT OR IGNORE INTO %s(%s,%s,role) VALUES(?,?,?)", table, ownerCol, targetCol), a.ID, r.ID, r.Role); e != nil {
					return e
				}
			}
		}
		return reindexRecord(ctx, tx, "activity", a.ID)
	})
	if e != nil {
		return a, e
	}
	return s.Activity(ctx, a.ID)
}

// DeleteActivity tombstones an activity after explicit user confirmation.
func (s *Store) DeleteActivity(ctx context.Context, id string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(ctx, "UPDATE activities SET deleted=1,updated_at=?,revision=revision+1 WHERE id=?", stamp(s.now()), id); e != nil {
			return e
		}
		return reindexRecord(ctx, tx, "activity", id)
	})
}
