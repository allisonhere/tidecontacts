package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
)

// SQLite's trim takes an explicit code-point set; use the same whitespace as
// Go's strings.TrimSpace when detecting titles derived from activity bodies.
const crmWhitespace = "\t\n\v\f\r \u0085\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000"

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
		err = each(ctx, s.db, `SELECT id,
 CASE WHEN trim(title,?1)=trim(substr(content,1,instr(content||char(10),char(10))-1),?1) THEN '' ELSE title END,
 type,occurred_at,revision FROM (
 SELECT id,title,type,occurred_at,revision,ltrim(body,?1) AS content
 FROM activities WHERE id>?2 AND deleted=0 ORDER BY id LIMIT ?3
 ) ORDER BY id`, []any{crmWhitespace, after, limit}, func(row *sql.Rows) error {
			r := app.RuleRecord{Kind: kind, Status: "recorded"}
			var occurred string
			if err := row.Scan(&r.ID, &r.Name, &r.Detail, &occurred, &r.Revision); err != nil {
				return err
			}
			r.When = parseStamp(occurred).In(s.now().Location()).Format(time.DateOnly)
			return add(r, nil)
		})
	case "project":
		err = each(ctx, s.db, `SELECT id,name,status,priority,target_date,revision FROM projects
 WHERE id>? AND deleted=0 AND status!='archived' ORDER BY id LIMIT ?`, []any{after, limit}, func(row *sql.Rows) error {
			r := app.RuleRecord{Kind: kind}
			err := row.Scan(&r.ID, &r.Name, &r.Status, &r.Priority, &r.Due, &r.Revision)
			return add(r, err)
		})
	case "followup", "followup-contact", "followup-organization":
		entity := ""
		if kind == "followup-contact" {
			entity = "person"
		} else if kind == "followup-organization" {
			entity = "organization"
		}
		err = each(ctx, s.db, `SELECT `+followUpColumns+`,e.revision FROM followup_prefs f JOIN entities e ON e.id=f.entity_id
 WHERE e.id>? AND (?='' OR e.kind=?) AND e.deleted=0 AND e.archived=0 AND (f.every>0 OR f.next_date!='' OR f.never=1)
 ORDER BY e.id LIMIT ?`, []any{stamp(s.now()), after, entity, entity, limit}, func(row *sql.Rows) error {
			var revision int
			f, err := s.scanFollowUp(followUpWithRevision{row, &revision})
			detail := "contact"
			if f.Entity.Kind == "organization" {
				detail = "organization"
			}
			return add(app.RuleRecord{ID: f.Entity.ID, Kind: "followup", Name: f.Entity.Name, Detail: detail, Due: f.Due,
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
