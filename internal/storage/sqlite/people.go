package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/google/uuid"
)

func entityRef(ctx context.Context, q querier, id string) (domain.Ref, error) {
	r := domain.Ref{ID: id}
	e := q.QueryRowContext(ctx, "SELECT kind,display_name FROM entities WHERE id=?", id).Scan(&r.Kind, &r.Name)
	return r, e
}

const lastInteraction = `COALESCE((SELECT a.occurred_at||'|'||a.type FROM activities a JOIN activity_entities j ON j.activity_id=a.id JOIN activity_types y ON y.name=a.type
 WHERE j.entity_id=e.id AND a.deleted=0 AND y.interaction=1 AND a.occurred_at<=? ORDER BY a.occurred_at DESC LIMIT 1),'')`

func (s *Store) scanFollowUp(r interface{ Scan(...any) error }) (domain.FollowUp, error) {
	var f domain.FollowUp
	var last string
	e := r.Scan(&f.Entity.ID, &f.Entity.Kind, &f.Entity.Name, &f.Interval.Every, &f.Interval.Unit, &f.Never, &f.NextDate, &f.SnoozedUntil, &f.SetAt, &last)
	if at, typ, ok := strings.Cut(last, "|"); ok {
		f.LastInteraction, f.LastType = domain.Day(parseStamp(at).In(s.now().Location())), typ
	}
	f.Evaluate(s.today())
	return f, e
}

const followUpColumns = `e.id,e.kind,e.display_name,COALESCE(f.every,0),COALESCE(f.unit,'day'),COALESCE(f.never,0),COALESCE(f.next_date,''),COALESCE(f.snoozed_until,''),COALESCE(f.set_at,''),` + lastInteraction

// FollowUp returns one contact's schedule, last recorded interaction and computed due date.
func (s *Store) FollowUp(ctx context.Context, entityID string) (domain.FollowUp, error) {
	return s.followUp(ctx, s.db, entityID)
}

func (s *Store) followUp(ctx context.Context, q querier, entityID string) (domain.FollowUp, error) {
	return s.scanFollowUp(q.QueryRowContext(ctx, "SELECT "+followUpColumns+" FROM entities e LEFT JOIN followup_prefs f ON f.entity_id=e.id WHERE e.id=?", stamp(s.now()), entityID))
}

// FollowUps evaluates every configured schedule. The work is bounded by the number of
// contacts that have a schedule, not by the size of the address book.
func (s *Store) FollowUps(ctx context.Context) ([]domain.FollowUp, error) {
	return s.followUps(ctx, s.db)
}

func (s *Store) followUps(ctx context.Context, q querier) ([]domain.FollowUp, error) {
	list := []domain.FollowUp{}
	e := each(ctx, q, "SELECT "+followUpColumns+" FROM followup_prefs f JOIN entities e ON e.id=f.entity_id WHERE e.deleted=0 AND e.archived=0", []any{stamp(s.now())}, func(r *sql.Rows) error {
		f, e := s.scanFollowUp(r)
		list = append(list, f)
		return e
	})
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if (a.Due == "") != (b.Due == "") {
			return a.Due != ""
		}
		if a.Due != b.Due {
			return a.Due < b.Due
		}
		return strings.ToLower(a.Entity.Name) < strings.ToLower(b.Entity.Name)
	})
	return list, e
}

// SetFollowUp stores a contact's cadence, explicit follow-up date or "never".
// Changing the cadence restarts its clock from today when no interaction is recorded.
func (s *Store) SetFollowUp(ctx context.Context, f domain.FollowUp) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		if f.Interval.IsZero() {
			f.Interval = domain.Interval{Unit: "day"}
		}
		for _, d := range []string{f.NextDate, f.SnoozedUntil} {
			if _, ok := domain.ParseDay(d); d != "" && !ok {
				return fmt.Errorf("follow-up dates must be YYYY-MM-DD")
			}
		}
		now, today := stamp(s.now()), s.today()
		_, e := tx.ExecContext(ctx, `INSERT INTO followup_prefs(entity_id,every,unit,never,next_date,snoozed_until,set_at,updated_at) VALUES(?,?,?,?,?,?,?,?)
 ON CONFLICT(entity_id) DO UPDATE SET set_at=CASE WHEN every!=excluded.every OR unit!=excluded.unit THEN excluded.set_at ELSE set_at END,
 every=excluded.every,unit=excluded.unit,never=excluded.never,next_date=excluded.next_date,snoozed_until=excluded.snoozed_until,updated_at=excluded.updated_at`,
			f.Entity.ID, f.Interval.Every, f.Interval.Unit, f.Never, f.NextDate, f.SnoozedUntil, today, now)
		return e
	})
}

func (s *Store) scanDate(r interface{ Scan(...any) error }) (domain.ImportantDate, error) {
	var d domain.ImportantDate
	e := r.Scan(&d.ID, &d.Entity.ID, &d.Entity.Kind, &d.Entity.Name, &d.Label, &d.Date, &d.Annual, &d.Notes)
	d.Next = d.Date
	if d.Annual {
		d.Next = domain.NextAnnual(d.Date, s.today())
	}
	return d, e
}

const dateSelect = `SELECT d.id AS date_id,e.id AS entity_id,e.kind,e.display_name,d.label,d.date,d.annual,d.notes FROM important_dates d JOIN entities e ON e.id=d.entity_id WHERE e.deleted=0`

// builtinDates is the same projection for birthdays and anniversaries stored on people.
const builtinDates = `SELECT '',e.id,e.kind,e.display_name,'Birthday',p.birthday,1,'' FROM people p JOIN entities e ON e.id=p.id WHERE e.deleted=0 AND p.birthday!=''
 UNION ALL SELECT '',e.id,e.kind,e.display_name,'Anniversary',p.anniversary,1,'' FROM people p JOIN entities e ON e.id=p.id WHERE e.deleted=0 AND p.anniversary!=''`

// Dates lists a contact's important dates, including its birthday and anniversary.
func (s *Store) Dates(ctx context.Context, entityID string) ([]domain.ImportantDate, error) {
	return s.dates(ctx, s.db, "SELECT * FROM ("+dateSelect+" UNION ALL "+builtinDates+") WHERE entity_id=?", entityID, -1)
}

// UpcomingDates lists dates occurring within the next days days across all contacts.
func (s *Store) UpcomingDates(ctx context.Context, days int) ([]domain.ImportantDate, error) {
	return s.dates(ctx, s.db, "SELECT * FROM ("+dateSelect+" AND e.archived=0 UNION ALL "+builtinDates+" AND e.archived=0)", "", days)
}

func (s *Store) dates(ctx context.Context, q querier, query, entityID string, days int) ([]domain.ImportantDate, error) {
	list := []domain.ImportantDate{}
	args := []any{}
	if entityID != "" {
		args = append(args, entityID)
	}
	today := s.today()
	e := each(ctx, q, query, args, func(r *sql.Rows) error {
		d, e := s.scanDate(r)
		d.Builtin = d.ID == ""
		if days < 0 || d.Next != "" && d.Next >= today && domain.DaysBetween(today, d.Next) <= days {
			list = append(list, d)
		}
		return e
	})
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Next != list[j].Next {
			return list[i].Next < list[j].Next
		}
		return list[i].Entity.Name < list[j].Entity.Name
	})
	return list, e
}

func (s *Store) SaveDate(ctx context.Context, d domain.ImportantDate) error {
	d.Label = strings.TrimSpace(d.Label)
	if d.Label == "" || !domain.DateValid(d.Date) || d.Date == "" {
		return fmt.Errorf("an important date needs a label and a YYYY-MM-DD or --MM-DD date")
	}
	if strings.HasPrefix(d.Date, "--") {
		d.Annual = true
	}
	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	_, e := s.db.ExecContext(ctx, `INSERT INTO important_dates(id,entity_id,label,date,annual,notes,created_at) VALUES(?,?,?,?,?,?,?)
 ON CONFLICT(id) DO UPDATE SET label=excluded.label,date=excluded.date,annual=excluded.annual,notes=excluded.notes`, d.ID, d.Entity.ID, d.Label, d.Date, d.Annual, d.Notes, stamp(s.now()))
	return e
}

func (s *Store) DeleteDate(ctx context.Context, id string) error {
	_, e := s.db.ExecContext(ctx, "DELETE FROM important_dates WHERE id=?", id)
	return e
}

// SetFlag marks or clears a contact as needing review, with an optional note.
func (s *Store) SetFlag(ctx context.Context, entityID string, on bool, note string) error {
	now := stamp(s.now())
	_, e := s.db.ExecContext(ctx, `INSERT INTO relationship_meta(entity_id,flagged,flag_note,flagged_at,updated_at) VALUES(?,?,?,?,?)
 ON CONFLICT(entity_id) DO UPDATE SET flagged=excluded.flagged,flag_note=excluded.flag_note,flagged_at=excluded.flagged_at,updated_at=excluded.updated_at`, entityID, on, note, now, now)
	return e
}

// SetRelationshipNotes stores free-form notes about your relationship with a contact.
func (s *Store) SetRelationshipNotes(ctx context.Context, entityID, notes string) error {
	_, e := s.db.ExecContext(ctx, `INSERT INTO relationship_meta(entity_id,notes,updated_at) VALUES(?,?,?)
 ON CONFLICT(entity_id) DO UPDATE SET notes=excluded.notes,updated_at=excluded.updated_at`, entityID, notes, stamp(s.now()))
	return e
}

// Connections builds the relationship-explorer projection for one contact. Each list is capped.
func (s *Store) Connections(ctx context.Context, id string) (domain.Connections, error) {
	var c domain.Connections
	e := s.read(ctx, func(tx *sql.Tx) error {
		var e error
		c, e = s.connections(ctx, tx, id)
		return e
	})
	return c, e
}

func (s *Store) connections(ctx context.Context, tx *sql.Tx, id string) (domain.Connections, error) {
	var c domain.Connections
	var e error
	if c.Root, e = entityRef(ctx, tx, id); e != nil {
		return c, e
	}
	_ = tx.QueryRowContext(ctx, "SELECT notes,flagged,flag_note FROM relationship_meta WHERE entity_id=?", id).Scan(&c.Notes, &c.Flagged, &c.FlagNote)
	edges := func(query string, limit int, args ...any) ([]domain.Edge, error) {
		list := []domain.Edge{}
		e := each(ctx, tx, query+fmt.Sprintf(" LIMIT %d", limit), args, func(r *sql.Rows) error {
			var x domain.Edge
			e := r.Scan(&x.Person.ID, &x.Person.Kind, &x.Person.Name, &x.Type, &x.Notes, &x.Count)
			list = append(list, x)
			return e
		})
		return list, e
	}
	const out = "SELECT e.id,e.kind,e.display_name,r.type,r.notes,0 FROM relationships r JOIN entities e ON e.id=r.target_id WHERE r.source_id=? AND e.deleted=0 ORDER BY r.type, e.display_name COLLATE NOCASE"
	const in = "SELECT e.id,e.kind,e.display_name,r.type,r.notes,0 FROM relationships r JOIN entities e ON e.id=r.source_id WHERE r.target_id=? AND e.deleted=0 ORDER BY r.type, e.display_name COLLATE NOCASE"
	if c.Outgoing, e = edges(out, 100, id); e != nil {
		return c, e
	}
	if c.Incoming, e = edges(in, 100, id); e != nil {
		return c, e
	}
	for _, list := range [][]domain.Edge{c.Outgoing, c.Incoming} {
		for i := range list {
			if i >= 40 {
				break
			}
			children, err := edges("SELECT * FROM ("+out+") WHERE id!=?", 8, list[i].Person.ID, id)
			if err != nil {
				return c, err
			}
			list[i].Children = children
		}
	}
	if c.Shared, e = edges(`SELECT e.id,e.kind,e.display_name,'shared activities','',count(*) FROM activity_entities a JOIN activity_entities b ON b.activity_id=a.activity_id AND b.entity_id!=a.entity_id
 JOIN entities e ON e.id=b.entity_id JOIN activities x ON x.id=a.activity_id WHERE a.entity_id=? AND x.deleted=0 AND e.deleted=0 GROUP BY e.id ORDER BY count(*) DESC, e.display_name COLLATE NOCASE`, 25, id); e != nil {
		return c, e
	}
	people := func(query string, args ...any) ([]domain.Ref, error) {
		list := []domain.Ref{}
		e := each(ctx, tx, query, args, func(r *sql.Rows) error {
			var x domain.Ref
			e := r.Scan(&x.ID, &x.Kind, &x.Name, &x.Role)
			list = append(list, x)
			return e
		})
		return list, e
	}
	const members = "SELECT e.id,e.kind,e.display_name,m.title FROM memberships m JOIN entities e ON e.id=m.person_id WHERE m.organization_id=? AND e.id!=? AND e.deleted=0 ORDER BY m.current DESC, e.display_name COLLATE NOCASE LIMIT 15"
	if c.Root.Kind == "organization" {
		list, err := people(members, id, id)
		if err != nil {
			return c, err
		}
		c.Organizations = []domain.OrgLink{{Organization: c.Root, Current: true, People: list}}
	} else if e = each(ctx, tx, "SELECT o.id,o.display_name,m.title,m.current FROM memberships m JOIN entities o ON o.id=m.organization_id WHERE m.person_id=? AND o.deleted=0 ORDER BY m.current DESC, o.display_name COLLATE NOCASE LIMIT 20", []any{id}, func(r *sql.Rows) error {
		o := domain.OrgLink{Organization: domain.Ref{Kind: "organization"}}
		e := r.Scan(&o.Organization.ID, &o.Organization.Name, &o.Title, &o.Current)
		c.Organizations = append(c.Organizations, o)
		return e
	}); e != nil {
		return c, e
	}
	if c.Root.Kind == "person" {
		for i := range c.Organizations {
			if c.Organizations[i].People, e = people(members, c.Organizations[i].Organization.ID, id); e != nil {
				return c, e
			}
		}
	}
	if e = each(ctx, tx, "SELECT p.id,p.name,p.status,j.role FROM project_entities j JOIN projects p ON p.id=j.project_id WHERE j.entity_id=? AND p.deleted=0 ORDER BY (p.status IN ('completed','archived')), p.name COLLATE NOCASE LIMIT 30", []any{id}, func(r *sql.Rows) error {
		l := domain.ProjectLink{Project: domain.Ref{Kind: "project"}}
		e := r.Scan(&l.Project.ID, &l.Project.Name, &l.Status, &l.Project.Role)
		c.Projects = append(c.Projects, l)
		return e
	}); e != nil {
		return c, e
	}
	for i := range c.Projects {
		if c.Projects[i].People, e = people("SELECT e.id,e.kind,e.display_name,j.role FROM project_entities j JOIN entities e ON e.id=j.entity_id WHERE j.project_id=? AND e.id!=? AND e.deleted=0 ORDER BY e.display_name COLLATE NOCASE LIMIT 15", c.Projects[i].Project.ID, id); e != nil {
			return c, e
		}
	}
	if c.Dates, e = s.dates(ctx, tx, "SELECT * FROM ("+dateSelect+" UNION ALL "+builtinDates+") WHERE entity_id=?", id, -1); e != nil {
		return c, e
	}
	if c.FollowUp, e = s.followUp(ctx, tx, id); e != nil {
		return c, e
	}
	loc := s.now().Location()
	e = each(ctx, tx, "SELECT "+activityColumns+" FROM activities a JOIN activity_entities j ON j.activity_id=a.id WHERE j.entity_id=? AND a.deleted=0 ORDER BY a.occurred_at DESC LIMIT 12", []any{id}, func(r *sql.Rows) error {
		a, e := scanActivity(r, loc)
		c.Recent = append(c.Recent, a)
		return e
	})
	return c, e
}

// Dossier gathers the bounded CRM context for a contact profile in one read transaction.
func (s *Store) Dossier(ctx context.Context, id string) (domain.Dossier, error) {
	var d domain.Dossier
	e := s.read(ctx, func(tx *sql.Tx) error {
		var e error
		if d.Connections, e = s.connections(ctx, tx, id); e != nil {
			return e
		}
		tasks := func(cond string, limit int, args ...any) ([]domain.Task, error) {
			list := []domain.Task{}
			e := each(ctx, tx, "SELECT "+taskColumns+taskFrom+"JOIN task_entities j ON j.task_id=t.id WHERE j.entity_id=? AND t.deleted=0 AND t.archived=0 AND "+cond+fmt.Sprintf(" LIMIT %d", limit), append([]any{id}, args...), func(r *sql.Rows) error {
				t, e := scanTask(r)
				list = append(list, t)
				return e
			})
			return list, e
		}
		if d.OpenTasks, e = tasks("t.status NOT IN ('completed','canceled','waiting') ORDER BY (t.due_date=''), t.due_date, t.priority DESC, t.title", 15); e != nil {
			return e
		}
		recent := stamp(s.now().AddDate(0, 0, -30))
		if d.Commitments, e = tasks("(t.status='waiting' OR (t.status='completed' AND t.completed_at>=?)) ORDER BY (t.status='completed'), t.due_date, t.completed_at DESC", 10, recent); e != nil {
			return e
		}
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM activity_entities j JOIN activities a ON a.id=j.activity_id WHERE j.entity_id=? AND a.deleted=0", id).Scan(&d.Activities); e != nil {
			return e
		}
		return tx.QueryRowContext(ctx, "SELECT count(*) FROM task_entities j JOIN tasks t ON t.id=j.task_id WHERE j.entity_id=? AND t.deleted=0 AND t.status NOT IN ('completed','canceled')", id).Scan(&d.OpenTaskCount)
	})
	return d, e
}

// MergeContacts folds the contact mergeID into keepID in one transaction. Contact
// methods, tags, fields, notes, memberships and relationships are combined; every CRM
// link (activities, tasks, projects, pipeline items, reminders, dates) is re-pointed;
// and the merged record becomes a tombstone that names its successor. No link is left
// pointing at the merged record.
func (s *Store) MergeContacts(ctx context.Context, keepID, mergeID string) (domain.Contact, error) {
	var keep domain.Contact
	e := s.write(ctx, func(tx *sql.Tx) error {
		if keepID == mergeID {
			return fmt.Errorf("choose two different contacts to merge")
		}
		var e error
		if keep, e = readContact(ctx, tx, keepID); e != nil {
			return e
		}
		gone, e := readContact(ctx, tx, mergeID)
		if e != nil {
			return e
		}
		if keep.Kind != gone.Kind || keep.Deleted || gone.Deleted {
			return fmt.Errorf("only two live contacts of the same kind can be merged")
		}
		fill := func(dst *string, src string) {
			if *dst == "" {
				*dst = src
			}
		}
		for _, p := range [][2]*string{{&keep.Given, &gone.Given}, {&keep.Middle, &gone.Middle}, {&keep.Family, &gone.Family}, {&keep.Prefix, &gone.Prefix}, {&keep.Suffix, &gone.Suffix},
			{&keep.Nickname, &gone.Nickname}, {&keep.Phonetic, &gone.Phonetic}, {&keep.Birthday, &gone.Birthday}, {&keep.Anniversary, &gone.Anniversary},
			{&keep.Biography, &gone.Biography}, {&keep.Avatar, &gone.Avatar}, {&keep.Website, &gone.Website}, {&keep.Industry, &gone.Industry}} {
			fill(p[0], *p[1])
		}
		if gone.Notes != "" && gone.Notes != keep.Notes {
			keep.Notes = strings.TrimSpace(keep.Notes + "\n\n" + gone.Notes)
		}
		keep.Favorite = keep.Favorite || gone.Favorite
		keep.Tags = append(keep.Tags, gone.Tags...)
		aliases := map[string]bool{strings.ToLower(keep.DisplayName): true}
		for _, a := range keep.Aliases {
			aliases[strings.ToLower(a)] = true
		}
		for _, a := range append([]string{gone.DisplayName}, gone.Aliases...) {
			if !aliases[strings.ToLower(a)] {
				aliases[strings.ToLower(a)] = true
				keep.Aliases = append(keep.Aliases, a)
			}
		}
		methods := map[string]bool{}
		preferred := map[string]bool{}
		for _, m := range keep.Methods {
			methods[m.Kind+"|"+domain.NormalizeMatch(m.Value)+"|"+m.Address.String()] = true
			preferred[m.Kind] = preferred[m.Kind] || m.Preferred
		}
		for _, m := range gone.Methods {
			if key := m.Kind + "|" + domain.NormalizeMatch(m.Value) + "|" + m.Address.String(); !methods[key] {
				methods[key] = true
				m.ID = ""
				m.Preferred = m.Preferred && !preferred[m.Kind]
				preferred[m.Kind] = preferred[m.Kind] || m.Preferred
				keep.Methods = append(keep.Methods, m)
			}
		}
		orgs := map[string]bool{}
		for _, m := range keep.Memberships {
			orgs[m.OrganizationID] = true
		}
		for _, m := range gone.Memberships {
			if !orgs[m.OrganizationID] {
				m.ID = ""
				keep.Memberships = append(keep.Memberships, m)
			}
		}
		related := map[string]bool{}
		own := keep.Relationships[:0]
		for _, r := range keep.Relationships {
			if r.TargetID != mergeID {
				related[r.TargetID+"|"+r.Type] = true
				own = append(own, r)
			}
		}
		keep.Relationships = own
		for _, r := range gone.Relationships {
			if r.TargetID != keepID && !related[r.TargetID+"|"+r.Type] {
				r.ID = ""
				keep.Relationships = append(keep.Relationships, r)
			}
		}
		fields := map[string]bool{}
		for _, f := range keep.Fields {
			fields[strings.ToLower(f.Name)] = true
		}
		for _, f := range gone.Fields {
			if !fields[strings.ToLower(f.Name)] {
				keep.Fields = append(keep.Fields, f)
			}
		}
		// Remove the merged record's own outgoing rows first so nothing can point back at the survivor twice.
		for _, q := range []string{"DELETE FROM relationships WHERE source_id=?", "DELETE FROM memberships WHERE person_id=?"} {
			if _, e = tx.ExecContext(ctx, q, mergeID); e != nil {
				return e
			}
		}
		for _, q := range []string{
			"DELETE FROM relationships WHERE target_id=?2 AND (source_id=?1 OR EXISTS(SELECT 1 FROM relationships k WHERE k.source_id=relationships.source_id AND k.target_id=?1 AND k.type=relationships.type))",
			"UPDATE relationships SET target_id=?1 WHERE target_id=?2",
			"UPDATE memberships SET organization_id=?1 WHERE organization_id=?2",
			"UPDATE reminders SET entity_id=?1 WHERE entity_id=?2",
			"UPDATE important_dates SET entity_id=?1 WHERE entity_id=?2",
			"INSERT OR IGNORE INTO followup_prefs(entity_id,every,unit,never,next_date,snoozed_until,set_at,updated_at) SELECT ?1,every,unit,never,next_date,snoozed_until,set_at,updated_at FROM followup_prefs WHERE entity_id=?2",
			"DELETE FROM followup_prefs WHERE entity_id=?2",
			"INSERT OR IGNORE INTO relationship_meta(entity_id,notes,flagged,flag_note,flagged_at,updated_at) SELECT ?1,notes,flagged,flag_note,flagged_at,updated_at FROM relationship_meta WHERE entity_id=?2",
			"DELETE FROM relationship_meta WHERE entity_id=?2",
			"DELETE FROM inbox_state WHERE instr(key,?2)>0 AND ?1!=''",
		} {
			if _, e = tx.ExecContext(ctx, q, keepID, mergeID); e != nil {
				return e
			}
		}
		touched := map[string][]string{}
		for owner := range ownerTables {
			table := owner + "_entities"
			if e = each(ctx, tx, fmt.Sprintf("SELECT %s_id FROM %s WHERE entity_id=?", owner, table), []any{mergeID}, func(r *sql.Rows) error {
				var id string
				e := r.Scan(&id)
				touched[owner] = append(touched[owner], id)
				return e
			}); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, fmt.Sprintf("INSERT OR IGNORE INTO %s(%s_id,entity_id,role) SELECT %s_id,?,role FROM %s WHERE entity_id=?", table, owner, owner, table), keepID, mergeID); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE entity_id=?", table), mergeID); e != nil {
				return e
			}
		}
		keep.Normalize()
		if e = keep.Validate(); e != nil {
			return e
		}
		if e = saveContact(ctx, tx, &keep, "merge"); e != nil {
			return e
		}
		gone.Deleted, gone.Memberships, gone.Relationships = true, nil, nil
		if gone.Extensions == nil {
			gone.Extensions = map[string]json.RawMessage{}
		}
		gone.Extensions["merged_into"], _ = json.Marshal(keepID)
		if e = saveContact(ctx, tx, &gone, "merged into "+keep.DisplayName); e != nil {
			return e
		}
		for owner, ids := range touched {
			for _, id := range ids {
				if e = reindexRecord(ctx, tx, owner, id); e != nil {
					return e
				}
			}
		}
		members := []string{}
		if e = each(ctx, tx, "SELECT person_id FROM memberships WHERE organization_id=?", []any{keepID}, func(r *sql.Rows) error {
			var id string
			e := r.Scan(&id)
			members = append(members, id)
			return e
		}); e != nil {
			return e
		}
		for _, id := range members {
			if e = reindex(ctx, tx, id); e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return keep, e
	}
	return s.Contact(ctx, keepID)
}
