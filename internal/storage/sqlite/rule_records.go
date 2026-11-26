package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
)

// RuleRecords returns live records ordered by stable ID. Only fixed, typed
// projections are available; criteria are evaluated by the caller.
func (s *Store) RuleRecords(ctx context.Context, kind, after string, limit int) ([]app.RuleRecord, error) {
	out := []app.RuleRecord{}
	limit = min(max(limit, 1), 500)
	add := func(r app.RuleRecord, err error) error {
		if err == nil {
			out = append(out, r)
		}
		return err
	}
	var err error
	switch kind {
	case "contact", "organization":
		entity := "person"
		if kind == "organization" {
			entity = kind
		}
		err = each(ctx, s.db, `SELECT e.id,e.display_name,e.revision,(
 SELECT json_group_array(t.name) FROM entity_tags x JOIN tags t ON t.id=x.tag_id WHERE x.entity_id=e.id)
 FROM entities e WHERE e.id>? AND e.kind=? AND e.archived=0 AND e.deleted=0 ORDER BY e.id LIMIT ?`, []any{after, entity, limit}, func(row *sql.Rows) error {
			r := app.RuleRecord{Kind: kind, Status: "active"}
			var tags string
			if err := row.Scan(&r.ID, &r.Name, &r.Revision, &tags); err != nil {
				return err
			}
			err := json.Unmarshal([]byte(tags), &r.Tags)
			return add(r, err)
		})
	case "task":
		err = each(ctx, s.db, `SELECT id,title,status,priority,due_date,revision FROM tasks
 WHERE id>? AND archived=0 AND deleted=0 ORDER BY id LIMIT ?`, []any{after, limit}, func(row *sql.Rows) error {
			r := app.RuleRecord{Kind: kind}
			err := row.Scan(&r.ID, &r.Name, &r.Status, &r.Priority, &r.Due, &r.Revision)
			return add(r, err)
		})
	case "activity":
		err = each(ctx, s.db, `SELECT id,title,type,occurred_at,revision FROM activities
 WHERE id>? AND deleted=0 ORDER BY id LIMIT ?`, []any{after, limit}, func(row *sql.Rows) error {
			r := app.RuleRecord{Kind: kind, Status: "recorded"}
			var occurred string
			if err := row.Scan(&r.ID, &r.Name, &r.Detail, &occurred, &r.Revision); err != nil {
				return err
			}
			r.When = parseStamp(occurred).In(s.now().Location()).Format(time.DateOnly)
			return add(r, nil)
		})
	case "followup":
		err = each(ctx, s.db, `SELECT `+followUpColumns+`,e.revision FROM followup_prefs f JOIN entities e ON e.id=f.entity_id
 WHERE e.id>? AND e.deleted=0 AND e.archived=0 AND (f.every>0 OR f.next_date!='' OR f.never=1)
 ORDER BY e.id LIMIT ?`, []any{stamp(s.now()), after, limit}, func(row *sql.Rows) error {
			var revision int
			f, err := s.scanFollowUp(followUpWithRevision{row, &revision})
			detail := "contact"
			if f.Entity.Kind == "organization" {
				detail = "organization"
			}
			return add(app.RuleRecord{ID: f.Entity.ID, Kind: kind, Name: f.Entity.Name, Detail: detail, Due: f.Due,
				Status: app.FollowUpState(f.Never, f.Due, s.today()), Revision: revision}, err)
		})
	}
	return out, err
}

type followUpWithRevision struct {
	row      *sql.Rows
	revision *int
}

func (r followUpWithRevision) Scan(values ...any) error {
	return r.row.Scan(append(values, r.revision)...)
}
