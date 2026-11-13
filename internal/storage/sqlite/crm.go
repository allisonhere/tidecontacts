package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
)

var _ app.CRM = (*Store)(nil)

// SetClock replaces the time source. The clock's location defines the local calendar:
// "today", due dates and timeline days are evaluated in it.
func (s *Store) SetClock(clock func() time.Time) { s.clock = clock }

func (s *Store) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now()
}
func (s *Store) today() string { return domain.Day(s.now()) }

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func parseStamp(v string) time.Time {
	t, _ := time.Parse(time.RFC3339, v)
	return t
}

type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *Store) write(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	if e = fn(tx); e != nil {
		return e
	}
	return tx.Commit()
}

func (s *Store) read(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	return fn(tx)
}

func each(ctx context.Context, q querier, query string, args []any, scan func(*sql.Rows) error) error {
	rows, e := q.QueryContext(ctx, query, args...)
	if e != nil {
		return e
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if e = scan(rows); e != nil {
			return e
		}
	}
	return rows.Err()
}

// Link targets: junction suffix, table, display column and SQL for the reference kind.
var linkTargets = map[string][4]string{
	"entity":   {"entities", "entities", "display_name", "t.kind"},
	"project":  {"projects", "projects", "name", "'project'"},
	"task":     {"tasks", "tasks", "title", "'task'"},
	"activity": {"activities", "activities", "title", "'activity'"},
}

// ownerTables maps a record kind to its junction prefix.
var ownerTables = map[string]string{"activity": "activity", "task": "task", "project": "project", "item": "item"}

func targetOf(kind string) string {
	if kind == "person" || kind == "organization" {
		return "entity"
	}
	return kind
}

// junction returns the table and column names linking owner to target, in whichever
// direction the schema stores the pair.
func junction(owner, target string) (table, ownerCol, targetCol string, ok bool) {
	if t, found := linkTargets[target]; found {
		if _, isOwner := ownerTables[owner]; isOwner {
			table = owner + "_" + t[0]
			if junctions[table] {
				return table, owner + "_id", target + "_id", true
			}
		}
	}
	// Reverse direction, for example a task's activities live in activity_tasks.
	if t, found := linkTargets[owner]; found {
		table = target + "_" + t[0]
		if junctions[table] {
			return table, owner + "_id", target + "_id", true
		}
	}
	return "", "", "", false
}

var junctions = map[string]bool{"activity_entities": true, "activity_projects": true, "activity_tasks": true, "task_entities": true,
	"task_projects": true, "project_entities": true, "item_entities": true, "item_projects": true, "item_tasks": true, "item_activities": true}

// links reads the references of one kind attached to each owner ID.
func links(ctx context.Context, q querier, owner, target string, ids []string) (map[string][]domain.Ref, error) {
	out := map[string][]domain.Ref{}
	table, ownerCol, targetCol, ok := junction(owner, target)
	if !ok || len(ids) == 0 {
		return out, nil
	}
	t := linkTargets[target]
	for start := 0; start < len(ids); start += 500 {
		chunk := ids[start:min(len(ids), start+500)]
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		query := fmt.Sprintf(`SELECT j.%s, t.id, t.%s, %s, j.role FROM %s j JOIN %s t ON t.id=j.%s WHERE j.%s IN (?%s) ORDER BY t.%s COLLATE NOCASE`,
			ownerCol, t[2], t[3], table, t[1], targetCol, ownerCol, strings.Repeat(",?", len(chunk)-1), t[2])
		if e := each(ctx, q, query, args, func(r *sql.Rows) error {
			var id string
			var ref domain.Ref
			if e := r.Scan(&id, &ref.ID, &ref.Name, &ref.Kind, &ref.Role); e != nil {
				return e
			}
			out[id] = append(out[id], ref)
			return nil
		}); e != nil {
			return nil, e
		}
	}
	return out, nil
}

// resolve turns references given by ID or exact unique name into verified IDs.
func resolve(ctx context.Context, tx *sql.Tx, target string, refs []domain.Ref) ([]domain.Ref, error) {
	t := linkTargets[target]
	out := []domain.Ref{}
	seen := map[string]bool{}
	for _, ref := range refs {
		key := ref.ID
		if key == "" {
			key = strings.TrimSpace(ref.Name)
		}
		if key == "" {
			continue
		}
		var id string
		var e error
		if target == "entity" {
			id, e = resolveName(ctx, tx, key, "")
		} else {
			rows := []string{}
			e = each(ctx, tx, fmt.Sprintf(`SELECT id FROM %s WHERE (id=? OR %s=? COLLATE NOCASE) AND deleted=0`, t[1], t[2]), []any{key, key}, func(r *sql.Rows) error {
				var v string
				rows = append(rows, v)
				return r.Scan(&rows[len(rows)-1])
			})
			switch {
			case e != nil:
			case len(rows) == 0:
				e = sql.ErrNoRows
			case len(rows) > 1:
				e = fmt.Errorf("ambiguous name")
			default:
				id = rows[0]
			}
		}
		if e != nil {
			return nil, fmt.Errorf("%s %q: choose an existing unique name or ID: %w", target, key, e)
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, domain.Ref{ID: id, Role: ref.Role})
		}
	}
	return out, nil
}

// setLinks replaces one owner's references of one kind.
func setLinks(ctx context.Context, tx *sql.Tx, owner, ownerID, target string, refs []domain.Ref) error {
	table, ownerCol, targetCol, ok := junction(owner, target)
	if !ok {
		return fmt.Errorf("no link between %s and %s", owner, target)
	}
	resolved, e := resolve(ctx, tx, target, refs)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s=?", table, ownerCol), ownerID); e != nil {
		return e
	}
	for _, r := range resolved {
		if _, e = tx.ExecContext(ctx, fmt.Sprintf("INSERT OR IGNORE INTO %s(%s,%s,role) VALUES(?,?,?)", table, ownerCol, targetCol), ownerID, r.ID, r.Role); e != nil {
			return e
		}
	}
	return nil
}

// Link adds or removes a single reference between two records without touching others.
func (s *Store) Link(ctx context.Context, owner, target domain.Ref, on bool) error {
	table, ownerCol, targetCol, ok := junction(owner.Kind, targetOf(target.Kind))
	if !ok {
		return fmt.Errorf("a %s cannot be linked to a %s", owner.Kind, target.Kind)
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		resolved, e := resolve(ctx, tx, targetOf(target.Kind), []domain.Ref{target})
		if e != nil || len(resolved) == 0 {
			return e
		}
		if on {
			_, e = tx.ExecContext(ctx, fmt.Sprintf("INSERT OR IGNORE INTO %s(%s,%s,role) VALUES(?,?,?)", table, ownerCol, targetCol), owner.ID, resolved[0].ID, target.Role)
		} else {
			_, e = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s=? AND %s=?", table, ownerCol, targetCol), owner.ID, resolved[0].ID)
		}
		if e != nil {
			return e
		}
		return reindexRecord(ctx, tx, owner.Kind, owner.ID)
	})
}

func setTags(ctx context.Context, tx *sql.Tx, owner, id string, tags []string) error {
	table, col := owner+"_tags", owner+"_id"
	if _, e := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s=?", table, col), id); e != nil {
		return e
	}
	for _, tag := range tags {
		if _, e := tx.ExecContext(ctx, "INSERT OR IGNORE INTO tags(name) VALUES(?)", tag); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, fmt.Sprintf("INSERT OR IGNORE INTO %s SELECT ?,id FROM tags WHERE name=? COLLATE NOCASE", table), id, tag); e != nil {
			return e
		}
	}
	return nil
}

func tagsOf(ctx context.Context, q querier, owner string, ids []string) (map[string][]string, error) {
	out := map[string][]string{}
	for start := 0; start < len(ids); start += 500 {
		chunk := ids[start:min(len(ids), start+500)]
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		query := fmt.Sprintf("SELECT j.%s_id, t.name FROM %s_tags j JOIN tags t ON t.id=j.tag_id WHERE j.%s_id IN (?%s) ORDER BY t.name", owner, owner, owner, strings.Repeat(",?", len(chunk)-1))
		if e := each(ctx, q, query, args, func(r *sql.Rows) error {
			var id, name string
			if e := r.Scan(&id, &name); e != nil {
				return e
			}
			out[id] = append(out[id], name)
			return nil
		}); e != nil {
			return nil, e
		}
	}
	return out, nil
}

// reindexRecord rebuilds one record's full-text document, including linked names.
func reindexRecord(ctx context.Context, tx *sql.Tx, kind, id string) error {
	var text string
	names := func(owner, target string) string {
		table, ownerCol, targetCol, ok := junction(owner, target)
		if !ok {
			return "''"
		}
		t := linkTargets[target]
		return fmt.Sprintf("COALESCE((SELECT group_concat(t.%s,' ') FROM %s j JOIN %s t ON t.id=j.%s WHERE j.%s=r.id),'')", t[2], table, t[1], targetCol, ownerCol)
	}
	tags := func(owner string) string {
		return fmt.Sprintf("COALESCE((SELECT group_concat(t.name,' ') FROM %s_tags j JOIN tags t ON t.id=j.tag_id WHERE j.%s_id=r.id),'')", owner, owner)
	}
	var query string
	switch kind {
	case "activity":
		query = "SELECT r.title||' '||r.body||' '||r.type||' '||" + names("activity", "entity") + "||' '||" + names("activity", "project") + "||' '||" + tags("activity") + " FROM activities r WHERE r.id=? AND r.deleted=0"
	case "task":
		query = "SELECT r.title||' '||r.description||' '||" + names("task", "entity") + "||' '||" + names("task", "project") + "||' '||" + tags("task") + " FROM tasks r WHERE r.id=? AND r.deleted=0"
	case "project":
		query = "SELECT r.name||' '||r.description||' '||r.notes||' '||" + names("project", "entity") + "||' '||" + tags("project") + " FROM projects r WHERE r.id=? AND r.deleted=0"
	case "item":
		query = "SELECT r.title||' '||r.notes||' '||r.next_action||' '||" + names("item", "entity") + "||' '||" + names("item", "project") + "||' '||" + tags("item") + " FROM pipeline_items r WHERE r.id=? AND r.deleted=0"
	default:
		return nil
	}
	e := tx.QueryRowContext(ctx, query, id).Scan(&text)
	if e != nil && e != sql.ErrNoRows {
		return e
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM crm_search WHERE kind=? AND record_id=?", kind, id); err != nil {
		return err
	}
	if e == sql.ErrNoRows {
		return nil
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO crm_search(kind,record_id,text) VALUES(?,?,?)", kind, id, text)
	return e
}

// where accumulates SQL conditions with bound arguments.
type where struct {
	parts []string
	args  []any
}

func (w *where) add(cond string, args ...any) {
	w.parts = append(w.parts, cond)
	w.args = append(w.args, args...)
}
func (w where) sql() string {
	if len(w.parts) == 0 {
		return "1=1"
	}
	return strings.Join(w.parts, " AND ")
}

// terms adds full-text matching for free words, plus a title substring fallback.
func (w *where) terms(q search.Query, kind, alias, title string) {
	for _, term := range q.Terms {
		w.add(fmt.Sprintf("(%s.id IN (SELECT record_id FROM crm_search WHERE kind='%s' AND crm_search MATCH ?) OR instr(lower(%s.%s),lower(?))>0)", alias, kind, alias, title),
			`"`+strings.ReplaceAll(term, `"`, `""`)+`"*`, term)
	}
}

// linked adds shared with:/org:/project:/tag: filters for a record kind.
func (w *where) linked(q search.Query, owner, alias string) {
	for _, f := range q.Filters {
		switch f.Key {
		case "with":
			if table, ownerCol, _, ok := junction(owner, "entity"); ok {
				w.add(fmt.Sprintf("EXISTS(SELECT 1 FROM %s j JOIN entities e ON e.id=j.entity_id WHERE j.%s=%s.id AND (e.id=? OR instr(lower(e.display_name),lower(?))>0))", table, ownerCol, alias), f.Value, f.Value)
			}
		case "org":
			if table, ownerCol, _, ok := junction(owner, "entity"); ok {
				w.add(fmt.Sprintf(`EXISTS(SELECT 1 FROM %s j JOIN entities e ON e.id=j.entity_id WHERE j.%s=%s.id AND (
 (e.kind='organization' AND (e.id=? OR instr(lower(e.display_name),lower(?))>0)) OR
 EXISTS(SELECT 1 FROM memberships m JOIN entities o ON o.id=m.organization_id WHERE m.person_id=e.id AND (o.id=? OR instr(lower(o.display_name),lower(?))>0))))`, table, ownerCol, alias), f.Value, f.Value, f.Value, f.Value)
			}
		case "project":
			if table, ownerCol, _, ok := junction(owner, "project"); ok {
				w.add(fmt.Sprintf("EXISTS(SELECT 1 FROM %s j JOIN projects pj ON pj.id=j.project_id WHERE j.%s=%s.id AND (pj.id=? OR instr(lower(pj.name),lower(?))>0))", table, ownerCol, alias), f.Value, f.Value)
			}
		case "tag":
			w.add(fmt.Sprintf("EXISTS(SELECT 1 FROM %s_tags j JOIN tags tg ON tg.id=j.tag_id WHERE j.%s_id=%s.id AND tg.name=? COLLATE NOCASE)", owner, owner, alias), f.Value)
		}
	}
}

// dateFilter converts a date word filter into a condition on a date column.
func (w *where) dateFilter(col, value, today string) error {
	v := strings.ToLower(value)
	switch v {
	case "none":
		w.add(col + "=''")
	case "any":
		w.add(col + "!=''")
	case "overdue":
		w.add(col+"!='' AND "+col+"<?", today)
	case "week":
		w.add(col+" BETWEEN ? AND ?", today, domain.AddDays(today, 7))
	case "month":
		w.add(col+" BETWEEN ? AND ?", today, domain.AddInterval(today, 1, "month"))
	default:
		op := "="
		if strings.HasPrefix(v, "<") || strings.HasPrefix(v, ">") {
			op, v = v[:1], v[1:]
		}
		day, e := domain.ParseWhen(v, today)
		if e != nil {
			return e
		}
		w.add(col+"!='' AND "+col+op+"?", day)
	}
	return nil
}
