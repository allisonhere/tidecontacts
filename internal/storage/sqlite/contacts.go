package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
	"github.com/sahilm/fuzzy"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Store) SaveContacts(ctx context.Context, contacts []domain.Contact, action string) ([]domain.Contact, error) {
	return s.SaveContactsWithProgress(ctx, contacts, action, nil)
}

func (s *Store) SaveContactsWithProgress(ctx context.Context, contacts []domain.Contact, action string, progress func(int, int)) ([]domain.Contact, error) {
	// Own a deep copy: generated IDs and revision updates must not mutate an in-flight UI draft.
	b, e := json.Marshal(contacts)
	if e != nil {
		return nil, e
	}
	var batch []domain.Contact
	if e = json.Unmarshal(b, &batch); e != nil {
		return nil, e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer func() { _ = tx.Rollback() }()
	// Reserve new identities first so a batch can contain forward references or cycles.
	seen := map[string]bool{}
	for i := range batch {
		c := &batch[i]
		c.Normalize()
		if err := c.Validate(); err != nil {
			return nil, err
		}
		if seen[c.ID] {
			return nil, fmt.Errorf("duplicate identity in batch")
		}
		seen[c.ID] = true
		var n int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM entities WHERE id=?", c.ID).Scan(&n); err != nil {
			return nil, err
		}
		if n == 0 && c.Revision == 0 {
			now := time.Now().UTC().Format(time.RFC3339Nano)
			if _, err := tx.ExecContext(ctx, "INSERT INTO entities(id,kind,display_name,created_at,updated_at,revision) VALUES(?,?,?,?,?,0)", c.ID, c.Kind, c.DisplayName, now, now); err != nil {
				return nil, err
			}
			if c.Kind == "organization" {
				if _, err := tx.ExecContext(ctx, "INSERT INTO organizations VALUES(?,?,?)", c.ID, c.Website, c.Industry); err != nil {
					return nil, err
				}
			}
		}
	}
	// Resolve names and insert newly named organizations in this same transaction.
	for i := range batch {
		c := &batch[i]
		c.Normalize()
		if e = c.Validate(); e != nil {
			return nil, e
		}
		for j := range c.Memberships {
			m := &c.Memberships[j]
			if m.OrganizationID == "" {
				id, err := resolveName(ctx, tx, m.Organization, "organization")
				if err == sql.ErrNoRows {
					org := domain.NewContact("organization")
					org.DisplayName = m.Organization
					org.Normalize()
					if err = saveContact(ctx, tx, &org, "create organization"); err != nil {
						return nil, err
					}
					id = org.ID
				} else if err != nil {
					return nil, err
				}
				m.OrganizationID = id
			}
		}
		for j := range c.Relationships {
			r := &c.Relationships[j]
			if r.TargetID == "" {
				id, err := resolveName(ctx, tx, r.TargetName, "")
				if err != nil {
					return nil, fmt.Errorf("relationship target %q: choose an existing unique name or UUID: %w", r.TargetName, err)
				}
				r.TargetID = id
			}
		}
		if e = c.Validate(); e != nil {
			return nil, e
		}
		if e = saveContact(ctx, tx, c, action); e != nil {
			return nil, e
		}
		if progress != nil {
			progress(i+1, len(batch))
		}
	}
	// Rebuild affected documents, including people whose organization name changed.
	for _, c := range batch {
		if e = reindex(ctx, tx, c.ID); e != nil {
			return nil, e
		}
		if c.Kind == "organization" {
			rows, err := tx.QueryContext(ctx, "SELECT person_id FROM memberships WHERE organization_id=?", c.ID)
			if err != nil {
				return nil, err
			}
			ids := []string{}
			for rows.Next() {
				var id string
				if err = rows.Scan(&id); err != nil {
					_ = rows.Close()
					return nil, err
				}
				ids = append(ids, id)
			}
			err = rows.Err()
			_ = rows.Close()
			if err != nil {
				return nil, err
			}
			for _, id := range ids {
				if e = reindex(ctx, tx, id); e != nil {
					return nil, e
				}
			}
		}
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return batch, nil
}
func resolveName(ctx context.Context, tx *sql.Tx, name, kind string) (string, error) {
	rows, e := tx.QueryContext(ctx, `SELECT id FROM entities WHERE (id=? OR display_name=? COLLATE NOCASE) AND deleted=0 AND (?='' OR kind=?)`, name, name, kind, kind)
	if e != nil {
		return "", e
	}
	defer func() { _ = rows.Close() }()
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return "", e
		}
		ids = append(ids, id)
	}
	if e = rows.Err(); e != nil {
		return "", e
	}
	if len(ids) == 0 {
		return "", sql.ErrNoRows
	}
	if len(ids) > 1 {
		return "", fmt.Errorf("ambiguous name")
	}
	return ids[0], nil
}
func saveContact(ctx context.Context, tx *sql.Tx, c *domain.Contact, action string) error {
	var revision int
	var created, kind string
	e := tx.QueryRowContext(ctx, "SELECT revision,created_at,kind FROM entities WHERE id=?", c.ID).Scan(&revision, &created, &kind)
	exists := e == nil
	if e != nil && e != sql.ErrNoRows {
		return e
	}
	if exists && (revision != c.Revision || kind != c.Kind) {
		return fmt.Errorf("record changed since it was opened; reload before saving")
	}
	if !exists && c.Revision != 0 {
		return fmt.Errorf("record no longer exists")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if !exists {
		created = now
	}
	c.CreatedAt, c.UpdatedAt, c.Revision = created, now, revision+1
	ext, e := json.Marshal(c.Extensions)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO entities VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET display_name=excluded.display_name,notes=excluded.notes,biography=excluded.biography,avatar=excluded.avatar,source=excluded.source,favorite=excluded.favorite,archived=excluded.archived,deleted=excluded.deleted,updated_at=excluded.updated_at,revision=excluded.revision,extensions=excluded.extensions`, c.ID, c.Kind, c.DisplayName, c.Notes, c.Biography, c.Avatar, c.Source, c.Favorite, c.Archived, c.Deleted, c.CreatedAt, c.UpdatedAt, c.Revision, string(ext))
	if e != nil {
		return e
	}
	if c.Kind == "person" {
		_, e = tx.ExecContext(ctx, `INSERT INTO people VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET given_name=excluded.given_name,middle_name=excluded.middle_name,family_name=excluded.family_name,prefix=excluded.prefix,suffix=excluded.suffix,nickname=excluded.nickname,phonetic=excluded.phonetic,birthday=excluded.birthday,anniversary=excluded.anniversary`, c.ID, c.Given, c.Middle, c.Family, c.Prefix, c.Suffix, c.Nickname, c.Phonetic, c.Birthday, c.Anniversary)
	} else {
		_, e = tx.ExecContext(ctx, `INSERT INTO organizations VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET website=excluded.website,industry=excluded.industry`, c.ID, c.Website, c.Industry)
	}
	if e != nil {
		return e
	}
	// Only aggregate-owned rows are replaced. Incoming memberships and relationships survive.
	for _, q := range []string{"DELETE FROM aliases WHERE entity_id=?", "DELETE FROM methods WHERE entity_id=?", "DELETE FROM memberships WHERE person_id=?", "DELETE FROM relationships WHERE source_id=?", "DELETE FROM entity_tags WHERE entity_id=?", "DELETE FROM field_values WHERE entity_id=?"} {
		if _, e = tx.ExecContext(ctx, q, c.ID); e != nil {
			return e
		}
	}
	for _, a := range c.Aliases {
		if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO aliases VALUES(?,?)", c.ID, a); e != nil {
			return e
		}
	}
	for _, m := range c.Methods {
		a := m.Address
		_, e = tx.ExecContext(ctx, `INSERT INTO methods VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, m.ID, c.ID, m.Kind, m.Label, m.Value, domain.NormalizeMatch(m.Value), m.Preferred, m.Notes, m.Source, m.Verification, a.POBox, a.Extended, a.Street, a.City, a.Region, a.PostalCode, a.Country)
		if e != nil {
			return e
		}
	}
	for _, m := range c.Memberships {
		_, e = tx.ExecContext(ctx, "INSERT INTO memberships VALUES(?,?,?,?,?,?,?,?)", m.ID, c.ID, m.OrganizationID, m.Title, m.Department, m.Start, m.End, m.Current)
		if e != nil {
			return e
		}
	}
	for _, r := range c.Relationships {
		if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO relationship_types VALUES(?)", r.Type); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO relationships VALUES(?,?,?,?,?)", r.ID, c.ID, r.TargetID, r.Type, r.Notes); e != nil {
			return e
		}
	}
	for _, tag := range c.Tags {
		if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO tags(name) VALUES(?)", tag); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO entity_tags SELECT ?,id FROM tags WHERE name=? COLLATE NOCASE", c.ID, tag); e != nil {
			return e
		}
	}
	for _, f := range c.Fields {
		options, _ := json.Marshal(f.Options)
		var typ, opts string
		e = tx.QueryRowContext(ctx, "SELECT type,options FROM field_definitions WHERE name=? COLLATE NOCASE", f.Name).Scan(&typ, &opts)
		if e == nil && (typ != f.Type || opts != string(options)) {
			return fmt.Errorf("custom field %q already has a different type or choices", f.Name)
		}
		if e != nil && e != sql.ErrNoRows {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO field_definitions(name,type,options) VALUES(?,?,?)", f.Name, f.Type, string(options)); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO field_values SELECT ?,id,? FROM field_definitions WHERE name=? COLLATE NOCASE", c.ID, f.Value, f.Name); e != nil {
			return e
		}
	}
	snapshot, e := json.Marshal(c)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO history VALUES(?,?,?,?,?)", c.ID, c.Revision, now, action, string(snapshot))
	if e != nil {
		return e
	}
	return reindex(ctx, tx, c.ID)
}
func reindex(ctx context.Context, tx *sql.Tx, id string) error {
	var text string
	e := tx.QueryRowContext(ctx, `SELECT e.display_name||' '||e.notes||' '||e.biography||' '||COALESCE((SELECT given_name||' '||middle_name||' '||family_name||' '||nickname||' '||phonetic FROM people WHERE id=e.id),'')||' '||COALESCE((SELECT group_concat(value,' ') FROM aliases WHERE entity_id=e.id),'')||' '||COALESCE((SELECT group_concat(value||' '||normalized||' '||street||' '||city||' '||country,' ') FROM methods WHERE entity_id=e.id),'')||' '||COALESCE((SELECT group_concat(o.display_name||' '||m.title,' ') FROM memberships m JOIN entities o ON o.id=m.organization_id WHERE m.person_id=e.id),'')||' '||COALESCE((SELECT group_concat(t.name,' ') FROM tags t JOIN entity_tags et ON t.id=et.tag_id WHERE et.entity_id=e.id),'') FROM entities e WHERE e.id=?`, id).Scan(&text)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM contact_search WHERE entity_id=?", id); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO contact_search(entity_id,text) VALUES(?,?)", id, text)
	return e
}
func (s *Store) List(ctx context.Context, kind string, q search.Query, order string) ([]domain.Summary, error) {
	where := []string{"e.kind=?"}
	args := []any{kind}
	archived, deleted := false, false
	today := s.today()
	var allow []map[string]bool // post-filters computed from evaluated schedules and dates
	for _, f := range q.Filters {
		switch f.Key {
		case "project":
			where = append(where, "EXISTS(SELECT 1 FROM project_entities j JOIN projects p ON p.id=j.project_id WHERE j.entity_id=e.id AND p.deleted=0 AND (p.id=? OR instr(lower(p.name),lower(?))>0))")
			args = append(args, f.Value, f.Value)
		case "seen":
			const seen = "EXISTS(SELECT 1 FROM activity_entities j JOIN activities a ON a.id=j.activity_id JOIN activity_types y ON y.name=a.type WHERE j.entity_id=e.id AND a.deleted=0 AND y.interaction=1 AND a.occurred_at<=? AND a.occurred_at>=?)"
			if f.Value == "never" {
				where = append(where, "NOT "+seen)
				args = append(args, stamp(s.now()), "")
				break
			}
			n, _ := strconv.Atoi(f.Value[:len(f.Value)-1])
			unit := map[byte]string{'d': "day", 'w': "week", 'm': "month"}[f.Value[len(f.Value)-1]]
			from, _ := domain.At(domain.AddInterval(today, -n, unit), "", s.now().Location())
			where = append(where, seen)
			args = append(args, stamp(s.now()), stamp(from))
		case "followup":
			if f.Value == "none" {
				where = append(where, "NOT EXISTS(SELECT 1 FROM followup_prefs f WHERE f.entity_id=e.id AND (f.every>0 OR f.next_date!=''))")
				break
			}
			list, err := s.followUps(ctx, s.db)
			if err != nil {
				return nil, err
			}
			limit := map[string]string{"due": today, "week": domain.AddDays(today, 7), "month": domain.AddInterval(today, 1, "month"), "set": "9999-12-31"}[f.Value]
			set := map[string]bool{}
			for _, x := range list {
				if x.Due != "" && x.Due <= limit {
					set[x.Entity.ID] = true
				}
			}
			allow = append(allow, set)
		case "date", "birthday":
			days := map[string]int{"today": 0, "week": 7, "month": 31}[f.Value]
			list, err := s.UpcomingDates(ctx, days)
			if err != nil {
				return nil, err
			}
			set := map[string]bool{}
			for _, x := range list {
				if f.Key == "date" || x.Label == "Birthday" {
					set[x.Entity.ID] = true
				}
			}
			allow = append(allow, set)
		case "is":
			switch f.Value {
			case "flagged":
				where = append(where, "EXISTS(SELECT 1 FROM relationship_meta r WHERE r.entity_id=e.id AND r.flagged=1)")
			case "favorite":
				where = append(where, "e.favorite=1")
			case "archived":
				archived = true
				where = append(where, "e.archived=1")
			case "active":
				where = append(where, "e.archived=0")
			case "deleted":
				deleted = true
				where = append(where, "e.deleted=1")
			}
		case "tag":
			where = append(where, "EXISTS(SELECT 1 FROM entity_tags et JOIN tags t ON t.id=et.tag_id WHERE et.entity_id=e.id AND t.name=? COLLATE NOCASE)")
			args = append(args, f.Value)
		case "org":
			where = append(where, "EXISTS(SELECT 1 FROM memberships m JOIN entities o ON o.id=m.organization_id WHERE m.person_id=e.id AND instr(lower(o.display_name),lower(?))>0)")
			args = append(args, f.Value)
		case "has":
			switch f.Value {
			case "tasks":
				where = append(where, "EXISTS(SELECT 1 FROM task_entities j JOIN tasks t ON t.id=j.task_id WHERE j.entity_id=e.id AND t.deleted=0 AND t.status NOT IN ('completed','canceled'))")
			case "followup":
				where = append(where, "EXISTS(SELECT 1 FROM followup_prefs f WHERE f.entity_id=e.id AND (f.every>0 OR f.next_date!=''))")
			case "dates":
				where = append(where, "(EXISTS(SELECT 1 FROM important_dates d WHERE d.entity_id=e.id) OR EXISTS(SELECT 1 FROM people p WHERE p.id=e.id AND (p.birthday!='' OR p.anniversary!='')))")
			default:
				where = append(where, "EXISTS(SELECT 1 FROM methods m WHERE m.entity_id=e.id AND m.kind=?)")
				args = append(args, f.Value)
			}
		}
	}
	if !archived {
		hasActive := false
		for _, f := range q.Filters {
			if f.Key == "is" && f.Value == "active" {
				hasActive = true
			}
		}
		if !hasActive {
			where = append(where, "e.archived=0")
		}
	}
	if !deleted {
		where = append(where, "e.deleted=0")
	}
	// FTS supplies exact/prefix candidates. A compact summary pass provides typo fallback and normalized phone/domain matching.
	fts := map[string]bool{}
	if len(q.Terms) > 0 {
		rows, e := s.db.QueryContext(ctx, "SELECT entity_id FROM contact_search WHERE contact_search MATCH ?", q.FTS())
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				_ = rows.Close()
				return nil, e
			}
			fts[id] = true
		}
		e = rows.Err()
		_ = rows.Close()
		if e != nil {
			return nil, e
		}
	}
	documents := map[string]string{}
	if len(q.Terms) > 0 {
		docs, err := s.db.QueryContext(ctx, "SELECT entity_id,text FROM contact_search")
		if err != nil {
			return nil, err
		}
		for docs.Next() {
			var id, text string
			if err = docs.Scan(&id, &text); err != nil {
				_ = docs.Close()
				return nil, err
			}
			documents[id] = text
		}
		err = docs.Err()
		_ = docs.Close()
		if err != nil {
			return nil, err
		}
	}
	rows, e := s.db.QueryContext(ctx, `SELECT e.id,e.kind,e.display_name,e.favorite,e.archived,e.updated_at,e.created_at,e.revision,
 COALESCE((SELECT group_concat(o.display_name,', ') FROM memberships m JOIN entities o ON o.id=m.organization_id WHERE m.person_id=e.id),''),
 COALESCE((SELECT group_concat(t.name,', ') FROM entity_tags et JOIN tags t ON t.id=et.tag_id WHERE et.entity_id=e.id),'') FROM entities e WHERE `+strings.Join(where, " AND "), args...)
	if e != nil {
		return nil, e
	}
	defer func() { _ = rows.Close() }()
	result := []domain.Summary{}
	scores := map[string]int{}
	for rows.Next() {
		var c domain.Summary
		if e = rows.Scan(&c.ID, &c.Kind, &c.Name, &c.Favorite, &c.Archived, &c.UpdatedAt, &c.CreatedAt, &c.Revision, &c.Organization, &c.Tags); e != nil {
			return nil, e
		}
		c.SearchText = documents[c.ID]
		score := 0
		match := true
		for _, set := range allow {
			match = match && set[c.ID]
		}
		for _, term := range q.Terms {
			if !match {
				break
			}
			normalized := domain.NormalizeMatch(term)
			if strings.Contains(strings.ToLower(c.SearchText), strings.ToLower(term)) || normalized != "" && strings.Contains(domain.NormalizeMatch(c.SearchText), normalized) {
				score += 100
			} else if m := fuzzy.Find(term, []string{c.Name + " " + c.Organization + " " + c.Tags}); len(m) > 0 {
				score += m[0].Score
			} else if !fts[c.ID] {
				match = false
				break
			}
		}
		if match {
			if fts[c.ID] {
				score += 200
			}
			scores[c.ID] = score
			c.SearchText = ""
			result = append(result, c)
		}
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if len(q.Terms) > 0 && scores[a.ID] != scores[b.ID] {
			return scores[a.ID] > scores[b.ID]
		}
		switch order {
		case "updated":
			if a.UpdatedAt != b.UpdatedAt {
				return a.UpdatedAt > b.UpdatedAt
			}
		case "added":
			if a.CreatedAt != b.CreatedAt {
				return a.CreatedAt > b.CreatedAt
			}
		case "organization":
			if a.Organization != b.Organization {
				return strings.ToLower(a.Organization) < strings.ToLower(b.Organization)
			}
		}
		if !strings.EqualFold(a.Name, b.Name) {
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}
		return a.ID < b.ID
	})
	return result, nil
}
func (s *Store) Contact(ctx context.Context, id string) (domain.Contact, error) {
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return domain.Contact{}, e
	}
	defer func() { _ = tx.Rollback() }()
	c, e := readContact(ctx, tx, id)
	if e != nil {
		return c, e
	}
	return c, tx.Commit()
}
func readContact(ctx context.Context, tx *sql.Tx, id string) (domain.Contact, error) {
	var c domain.Contact
	var ext string
	e := tx.QueryRowContext(ctx, `SELECT id,kind,display_name,notes,biography,avatar,source,favorite,archived,deleted,created_at,updated_at,revision,extensions FROM entities WHERE id=?`, id).Scan(&c.ID, &c.Kind, &c.DisplayName, &c.Notes, &c.Biography, &c.Avatar, &c.Source, &c.Favorite, &c.Archived, &c.Deleted, &c.CreatedAt, &c.UpdatedAt, &c.Revision, &ext)
	if e != nil {
		return c, e
	}
	if e = json.Unmarshal([]byte(ext), &c.Extensions); e != nil {
		return c, e
	}
	if c.Kind == "person" {
		e = tx.QueryRowContext(ctx, "SELECT given_name,middle_name,family_name,prefix,suffix,nickname,phonetic,birthday,anniversary FROM people WHERE id=?", id).Scan(&c.Given, &c.Middle, &c.Family, &c.Prefix, &c.Suffix, &c.Nickname, &c.Phonetic, &c.Birthday, &c.Anniversary)
	} else {
		e = tx.QueryRowContext(ctx, "SELECT website,industry FROM organizations WHERE id=?", id).Scan(&c.Website, &c.Industry)
	}
	if e != nil {
		return c, e
	}
	read := func(query string, scan func(*sql.Rows) error) error {
		rows, err := tx.QueryContext(ctx, query, id)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			if err = scan(rows); err != nil {
				return err
			}
		}
		return rows.Err()
	}
	if e = read("SELECT value FROM aliases WHERE entity_id=? ORDER BY value", func(r *sql.Rows) error { var a string; err := r.Scan(&a); c.Aliases = append(c.Aliases, a); return err }); e != nil {
		return c, e
	}
	if e = read("SELECT id,kind,label,value,preferred,notes,source,verification,po_box,extended,street,city,region,postal_code,country FROM methods WHERE entity_id=? ORDER BY rowid", func(r *sql.Rows) error {
		var m domain.Method
		a := &m.Address
		err := r.Scan(&m.ID, &m.Kind, &m.Label, &m.Value, &m.Preferred, &m.Notes, &m.Source, &m.Verification, &a.POBox, &a.Extended, &a.Street, &a.City, &a.Region, &a.PostalCode, &a.Country)
		c.Methods = append(c.Methods, m)
		return err
	}); e != nil {
		return c, e
	}
	if e = read("SELECT m.id,m.organization_id,o.display_name,m.title,m.department,m.start_date,m.end_date,m.current FROM memberships m JOIN entities o ON o.id=m.organization_id WHERE m.person_id=? ORDER BY m.rowid", func(r *sql.Rows) error {
		var m domain.Membership
		err := r.Scan(&m.ID, &m.OrganizationID, &m.Organization, &m.Title, &m.Department, &m.Start, &m.End, &m.Current)
		c.Memberships = append(c.Memberships, m)
		return err
	}); e != nil {
		return c, e
	}
	if e = read("SELECT r.id,r.target_id,e.display_name,r.type,r.notes FROM relationships r JOIN entities e ON e.id=r.target_id WHERE r.source_id=? ORDER BY r.rowid", func(rows *sql.Rows) error {
		var r domain.Relationship
		err := rows.Scan(&r.ID, &r.TargetID, &r.TargetName, &r.Type, &r.Notes)
		c.Relationships = append(c.Relationships, r)
		return err
	}); e != nil {
		return c, e
	}
	if e = read("SELECT t.name FROM entity_tags et JOIN tags t ON t.id=et.tag_id WHERE et.entity_id=? ORDER BY t.name", func(r *sql.Rows) error { var v string; err := r.Scan(&v); c.Tags = append(c.Tags, v); return err }); e != nil {
		return c, e
	}
	if e = read("SELECT d.name,d.type,d.options,v.value FROM field_values v JOIN field_definitions d ON d.id=v.field_id WHERE v.entity_id=? ORDER BY d.name", func(r *sql.Rows) error {
		var f domain.CustomField
		var opts string
		if err := r.Scan(&f.Name, &f.Type, &opts, &f.Value); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(opts), &f.Options); err != nil {
			return err
		}
		c.Fields = append(c.Fields, f)
		return nil
	}); e != nil {
		return c, e
	}
	if e = read("SELECT r.id,r.source_id,e.display_name,r.type,r.notes FROM relationships r JOIN entities e ON e.id=r.source_id WHERE r.target_id=? AND e.deleted=0 ORDER BY e.display_name", func(rows *sql.Rows) error {
		var r domain.Relationship
		err := rows.Scan(&r.ID, &r.TargetID, &r.TargetName, &r.Type, &r.Notes)
		c.Incoming = append(c.Incoming, r)
		return err
	}); e != nil {
		return c, e
	}
	if c.Kind == "organization" {
		if e = read("SELECT m.id,m.person_id,e.display_name,m.title,m.department FROM memberships m JOIN entities e ON e.id=m.person_id WHERE m.organization_id=? AND e.deleted=0 ORDER BY e.display_name", func(rows *sql.Rows) error {
			var r domain.Relationship
			err := rows.Scan(&r.ID, &r.TargetID, &r.TargetName, &r.Type, &r.Notes)
			c.Members = append(c.Members, r)
			return err
		}); e != nil {
			return c, e
		}
	}
	return c, nil
}
func (s *Store) History(ctx context.Context, id string) ([]domain.History, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT revision,at,action,snapshot FROM history WHERE entity_id=? ORDER BY revision DESC", id)
	if e != nil {
		return nil, e
	}
	defer func() { _ = rows.Close() }()
	result := []domain.History{}
	for rows.Next() {
		var h domain.History
		if e = rows.Scan(&h.Revision, &h.At, &h.Action, &h.Snapshot); e != nil {
			return nil, e
		}
		result = append(result, h)
	}
	return result, rows.Err()
}
