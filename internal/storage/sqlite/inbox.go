package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/domain"
)

// RadarConfig loads the user's Relationship Radar thresholds and switches.
func (s *Store) RadarConfig(ctx context.Context) (domain.RadarConfig, error) {
	c := domain.DefaultRadar()
	raw, e := s.Get(ctx, "radar")
	if e == nil && raw != "" {
		e = json.Unmarshal([]byte(raw), &c)
	}
	c.Normalize()
	return c, e
}

func (s *Store) SaveRadarConfig(ctx context.Context, c domain.RadarConfig) error {
	c.Normalize()
	raw, e := json.Marshal(c)
	if e != nil {
		return e
	}
	return s.Put(ctx, "radar", string(raw))
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func inDays(today, day string) string {
	switch n := domain.DaysBetween(today, day); {
	case n == 0:
		return "today"
	case n == 1:
		return "tomorrow"
	case n > 0:
		return fmt.Sprintf("in %d days", n)
	default:
		return plural(-n, "day") + " ago"
	}
}

// candidates derives every inbox entry from current data. It never invents history:
// each entry points at a stored record and states the fact that produced it.
func (s *Store) candidates(ctx context.Context, q querier, cfg domain.RadarConfig) ([]domain.InboxItem, error) {
	items := []domain.InboxItem{}
	now, today := s.now(), s.today()
	reminders := []domain.Reminder{}
	if e := each(ctx, q, "SELECT "+reminderColumns+reminderFrom+"WHERE m.status='pending' AND MAX(m.remind_at,m.snoozed_until)<=? ORDER BY m.remind_at LIMIT 500", []any{stamp(now)}, func(r *sql.Rows) error {
		m, e := s.scanReminder(r)
		reminders = append(reminders, m)
		return e
	}); e != nil {
		return nil, e
	}
	for _, m := range reminders {
		it := domain.InboxItem{Key: "reminder:" + m.ID, Kind: "reminder", Title: m.Title, Source: domain.Ref{Kind: "reminder", ID: m.ID, Name: m.Title}, Person: m.Entity,
			Due: domain.Day(m.At), Urgency: 2, Fingerprint: stamp(m.At), Reason: "Reminder set for " + m.At.Format("2006-01-02 15:04")}
		if now.Sub(m.At).Hours() > 24 {
			it.Urgency = 3
		}
		if m.Task.ID != "" {
			it.Reason += " · task: " + m.Task.Name
			it.Source = m.Task
			it.Source.Role = m.ID
		}
		if !m.Recurrence.IsZero() {
			it.Reason += " · repeats " + m.Recurrence.String()
		}
		items = append(items, it)
	}
	tasks := []domain.Task{}
	waitingSince := domain.AddDays(today, -cfg.WaitingDays)
	if e := each(ctx, q, "SELECT "+taskColumns+taskFrom+`WHERE t.deleted=0 AND t.archived=0 AND t.status NOT IN ('completed','canceled') AND (t.snoozed_until='' OR t.snoozed_until<=?)
 AND ((t.due_date!='' AND t.due_date<?) OR t.status='inbox' OR (t.status='waiting' AND substr(t.updated_at,1,10)<=?)) ORDER BY t.due_date LIMIT 1000`, []any{today, today, waitingSince}, func(r *sql.Rows) error {
		t, e := scanTask(r)
		tasks = append(tasks, t)
		return e
	}); e != nil {
		return nil, e
	}
	if e := attachTaskLinks(ctx, q, tasks, false); e != nil {
		return nil, e
	}
	for _, t := range tasks {
		it := domain.InboxItem{Key: "task:" + t.ID, Title: t.Title, Source: domain.Ref{Kind: "task", ID: t.ID, Name: t.Title}, Due: t.Due, Urgency: 1}
		for _, r := range t.Entities {
			if r.Kind == "person" {
				it.Person = r
				break
			}
		}
		people := domain.RefNames(t.Entities)
		switch {
		case t.Due != "" && t.Due < today:
			it.Kind, it.Urgency = "task-overdue", 3
			it.Reason = fmt.Sprintf("Due %s, %s overdue", t.Due, plural(domain.DaysBetween(t.Due, today), "day"))
			if t.Status == "waiting" {
				it.Reason += "; waiting on " + orSomeone(people)
			} else if people != "" {
				it.Reason += "; involves " + people
			}
		case t.Status == "waiting":
			it.Kind = "task-waiting"
			it.Reason = fmt.Sprintf("Waiting on %s since %s", orSomeone(people), t.UpdatedAt[:10])
		default:
			it.Kind = "task-inbox"
			it.Reason = "Captured " + t.CreatedAt[:10] + "; not yet given a status, date or person"
		}
		it.Fingerprint = it.Kind + "|" + t.Due + "|" + t.Status
		items = append(items, it)
	}
	follow, e := s.followUps(ctx, q)
	if e != nil {
		return nil, e
	}
	for _, f := range follow {
		if f.Due == "" || f.Due > today {
			continue
		}
		it := domain.InboxItem{Key: "followup:" + f.Entity.ID, Kind: "followup-suggested", Title: f.Entity.Name, Source: f.Entity, Person: f.Entity, Due: f.Due, Urgency: 2,
			Reason: f.Reason, Fingerprint: f.Due + "|" + f.LastInteraction}
		if f.NextDate != "" && f.NextDate <= today {
			it.Kind = "followup-due"
		}
		if domain.DaysBetween(f.Due, today) > 7 {
			it.Urgency = 3
		}
		items = append(items, it)
	}
	dates, e := s.dates(ctx, q, "SELECT * FROM ("+dateSelect+" AND e.archived=0 UNION ALL "+builtinDates+" AND e.archived=0)", "", cfg.UpcomingDays)
	if e != nil {
		return nil, e
	}
	for _, d := range dates {
		it := domain.InboxItem{Key: "date:" + d.Entity.ID + ":" + strings.ToLower(d.Label), Kind: "date", Title: d.Entity.Name, Source: d.Entity, Person: d.Entity, Due: d.Next, Urgency: 1,
			Reason: fmt.Sprintf("%s on %s (%s)", d.Label, d.Next, inDays(today, d.Next)), Fingerprint: d.Next}
		if domain.DaysBetween(today, d.Next) <= 2 {
			it.Urgency = 2
		}
		items = append(items, it)
	}
	e = each(ctx, q, "SELECT e.id,e.kind,e.display_name,r.flag_note,r.flagged_at FROM relationship_meta r JOIN entities e ON e.id=r.entity_id WHERE r.flagged=1 AND e.deleted=0 LIMIT 500", nil, func(r *sql.Rows) error {
		var ref domain.Ref
		var note, at string
		if e := r.Scan(&ref.ID, &ref.Kind, &ref.Name, &note, &at); e != nil {
			return e
		}
		reason := "You flagged this contact for review on " + at[:min(10, len(at))]
		if note != "" {
			reason += ": " + note
		}
		items = append(items, domain.InboxItem{Key: "flag:" + ref.ID, Kind: "flagged", Title: ref.Name, Source: ref, Person: ref, Urgency: 1, Reason: reason, Fingerprint: at})
		return nil
	})
	return items, e
}

func orSomeone(names string) string {
	if names == "" {
		return "someone (no person linked)"
	}
	return names
}

// Inbox returns the People Inbox: one entry per source record, with dismissed and
// snoozed entries removed. An entry that was dismissed returns only when the facts
// behind it change (its fingerprint), for example a new due date or a later interaction.
func (s *Store) Inbox(ctx context.Context) ([]domain.InboxItem, error) {
	var out []domain.InboxItem
	cfg, e := s.RadarConfig(ctx)
	if e != nil {
		return nil, e
	}
	e = s.write(ctx, func(tx *sql.Tx) error {
		items, e := s.candidates(ctx, tx, cfg)
		if e != nil {
			return e
		}
		type state struct{ fingerprint, status, snoozed, first string }
		states := map[string]state{}
		if e = each(ctx, tx, "SELECT key,fingerprint,status,snoozed_until,first_seen FROM inbox_state", nil, func(r *sql.Rows) error {
			var k string
			var st state
			e := r.Scan(&k, &st.fingerprint, &st.status, &st.snoozed, &st.first)
			states[k] = st
			return e
		}); e != nil {
			return e
		}
		now, today := stamp(s.now()), s.today()
		seen := map[string]bool{}
		out = []domain.InboxItem{}
		for _, it := range items {
			if seen[it.Key] {
				continue
			}
			seen[it.Key] = true
			st, known := states[it.Key]
			switch {
			case !known || st.fingerprint != it.Fingerprint:
				st = state{it.Fingerprint, "open", "", now}
				if _, e = tx.ExecContext(ctx, `INSERT INTO inbox_state(key,kind,fingerprint,status,snoozed_until,first_seen,updated_at) VALUES(?,?,?,'open','',?,?)
 ON CONFLICT(key) DO UPDATE SET kind=excluded.kind,fingerprint=excluded.fingerprint,status='open',snoozed_until='',first_seen=excluded.first_seen,updated_at=excluded.updated_at`, it.Key, it.Kind, it.Fingerprint, now, now); e != nil {
					return e
				}
			case st.status == "dismissed", st.status == "snoozed" && st.snoozed > today:
				continue
			case st.status == "snoozed":
				st.status = "open"
				if _, e = tx.ExecContext(ctx, "UPDATE inbox_state SET status='open',snoozed_until='',updated_at=? WHERE key=?", now, it.Key); e != nil {
					return e
				}
			}
			it.Status, it.FirstSeen = st.status, st.first
			out = append(out, it)
		}
		// Forget state for records that stopped qualifying long ago, so the table stays bounded.
		cutoff := stamp(s.now().AddDate(0, 0, -180))
		for k := range states {
			if !seen[k] {
				if _, e = tx.ExecContext(ctx, "DELETE FROM inbox_state WHERE key=? AND updated_at<?", k, cutoff); e != nil {
					return e
				}
			}
		}
		return nil
	})
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.Status == "reviewed") != (b.Status == "reviewed") {
			return b.Status == "reviewed"
		}
		if a.Urgency != b.Urgency {
			return a.Urgency > b.Urgency
		}
		if a.Due != b.Due {
			return a.Due != "" && (b.Due == "" || a.Due < b.Due)
		}
		return strings.ToLower(a.Title) < strings.ToLower(b.Title)
	})
	return out, e
}

// InboxAct records dismiss, review, reopen or snooze (until a YYYY-MM-DD date) for one entry.
func (s *Store) InboxAct(ctx context.Context, key, action, until string) error {
	status := map[string]string{"dismiss": "dismissed", "review": "reviewed", "reopen": "open", "snooze": "snoozed"}[action]
	if status == "" {
		return fmt.Errorf("unknown inbox action %q", action)
	}
	if _, ok := domain.ParseDay(until); action == "snooze" && !ok {
		return fmt.Errorf("snooze needs a date")
	}
	if action != "snooze" {
		until = ""
	}
	r, e := s.db.ExecContext(ctx, "UPDATE inbox_state SET status=?,snoozed_until=?,updated_at=? WHERE key=?", status, until, stamp(s.now()), key)
	if e == nil {
		if n, _ := r.RowsAffected(); n == 0 {
			e = fmt.Errorf("that inbox entry no longer exists")
		}
	}
	return e
}

// Radar evaluates the enabled deterministic signals. Every row carries its reason.
func (s *Store) Radar(ctx context.Context, cfg domain.RadarConfig) ([]domain.Signal, error) {
	cfg.Normalize()
	out := []domain.Signal{}
	e := s.read(ctx, func(tx *sql.Tx) error {
		today, now := s.today(), s.now()
		add := func(kind string, list []domain.Signal) {
			if len(list) > cfg.Limit {
				list = list[:cfg.Limit]
			}
			for i := range list {
				list[i].Kind = kind
			}
			out = append(out, list...)
		}
		on := func(kind string) bool { return !cfg.Disabled[kind] }
		var follow []domain.FollowUp
		var e error
		if on("followup") || on("cadence") {
			if follow, e = s.followUps(ctx, tx); e != nil {
				return e
			}
		}
		due, cadence := []domain.Signal{}, []domain.Signal{}
		horizon := domain.AddDays(today, cfg.LeadDays)
		for _, f := range follow {
			if f.Due != "" && f.Due <= horizon {
				due = append(due, domain.Signal{Person: f.Entity, Date: f.Due, Reason: f.Reason + "; due " + inDays(today, f.Due)})
			} else if !f.Interval.IsZero() {
				cadence = append(cadence, domain.Signal{Person: f.Entity, Date: f.Due, Reason: f.Reason + "; next due " + f.Due})
			}
		}
		if on("followup") {
			add("followup", due)
		}
		if on("commitments") {
			list := []domain.Signal{}
			if e = each(ctx, tx, `SELECT e.id,e.kind,e.display_name,count(*),sum(t.status='waiting'),COALESCE(min(NULLIF(t.due_date,'')),'')
 FROM task_entities j JOIN tasks t ON t.id=j.task_id JOIN entities e ON e.id=j.entity_id
 WHERE t.deleted=0 AND t.archived=0 AND t.status NOT IN ('completed','canceled') AND e.deleted=0 AND e.kind='person'
 GROUP BY e.id ORDER BY (min(NULLIF(t.due_date,'')) IS NULL), min(NULLIF(t.due_date,'')), e.display_name COLLATE NOCASE LIMIT ?`, []any{cfg.Limit}, func(r *sql.Rows) error {
				var sig domain.Signal
				var n, waiting int
				if e := r.Scan(&sig.Person.ID, &sig.Person.Kind, &sig.Person.Name, &n, &waiting, &sig.Date); e != nil {
					return e
				}
				sig.Reason = plural(n, "open task")
				if waiting > 0 {
					sig.Reason += fmt.Sprintf(" (%d waiting on them)", waiting)
				}
				if sig.Date != "" {
					sig.Reason += "; earliest due " + sig.Date
				}
				list = append(list, sig)
				return nil
			}); e != nil {
				return e
			}
			add("commitments", list)
		}
		if on("dates") {
			dates, e := s.dates(ctx, tx, "SELECT * FROM ("+dateSelect+" AND e.archived=0 UNION ALL "+builtinDates+" AND e.archived=0)", "", cfg.UpcomingDays)
			if e != nil {
				return e
			}
			list := []domain.Signal{}
			for _, d := range dates {
				list = append(list, domain.Signal{Person: d.Entity, Date: d.Next, Reason: fmt.Sprintf("%s on %s (%s)", d.Label, d.Next, inDays(today, d.Next))})
			}
			add("dates", list)
		}
		if on("cadence") {
			add("cadence", cadence)
		}
		if on("active") {
			list := []domain.Signal{}
			from := stamp(now.AddDate(0, 0, -cfg.ActiveDays))
			if e = each(ctx, tx, `SELECT e.id,e.kind,e.display_name,count(*),max(a.occurred_at) FROM activity_entities j JOIN activities a ON a.id=j.activity_id
 JOIN activity_types y ON y.name=a.type JOIN entities e ON e.id=j.entity_id
 WHERE a.deleted=0 AND y.interaction=1 AND a.occurred_at>=? AND a.occurred_at<=? AND e.deleted=0 AND e.kind='person'
 GROUP BY e.id HAVING count(*)>=? ORDER BY count(*) DESC, max(a.occurred_at) DESC LIMIT ?`, []any{from, stamp(now), cfg.ActiveMinimum, cfg.Limit}, func(r *sql.Rows) error {
				var sig domain.Signal
				var n int
				var last string
				if e := r.Scan(&sig.Person.ID, &sig.Person.Kind, &sig.Person.Name, &n, &last); e != nil {
					return e
				}
				sig.Date = domain.Day(parseStamp(last).In(now.Location()))
				sig.Reason = fmt.Sprintf("%s recorded in the last %d days; latest on %s", plural(n, "interaction"), cfg.ActiveDays, sig.Date)
				list = append(list, sig)
				return nil
			}); e != nil {
				return e
			}
			add("active", list)
		}
		if on("overdue") {
			list := []domain.Signal{}
			if e = each(ctx, tx, `SELECT * FROM (
 SELECT e.id,e.kind,e.display_name,'task' AS what,t.title AS title,t.due_date AS due FROM task_entities j JOIN tasks t ON t.id=j.task_id JOIN entities e ON e.id=j.entity_id
  WHERE t.deleted=0 AND t.archived=0 AND t.status NOT IN ('completed','canceled') AND t.due_date!='' AND t.due_date<?1 AND e.deleted=0 AND e.kind='person'
 UNION ALL
 SELECT e.id,e.kind,e.display_name,'project',p.name,p.target_date FROM project_entities j JOIN projects p ON p.id=j.project_id JOIN entities e ON e.id=j.entity_id
  WHERE p.deleted=0 AND p.status IN ('planned','active','on-hold') AND p.target_date!='' AND p.target_date<?1 AND e.deleted=0 AND e.kind='person'
 ) ORDER BY due LIMIT ?2`, []any{today, cfg.Limit}, func(r *sql.Rows) error {
				var sig domain.Signal
				var what, title string
				if e := r.Scan(&sig.Person.ID, &sig.Person.Kind, &sig.Person.Name, &what, &title, &sig.Date); e != nil {
					return e
				}
				sig.Reason = fmt.Sprintf("Linked to overdue %s “%s” (due %s)", what, title, sig.Date)
				if what == "project" {
					sig.Reason = fmt.Sprintf("Linked to project “%s”, whose target date %s has passed", title, sig.Date)
				}
				list = append(list, sig)
				return nil
			}); e != nil {
				return e
			}
			add("overdue", list)
		}
		return nil
	})
	return out, e
}

// Collections lists saved filters, pinned first.
func (s *Store) Collections(ctx context.Context) ([]domain.Collection, error) {
	list := []domain.Collection{}
	e := each(ctx, s.db, "SELECT id,name,description,entity,query,sort,grouping,pinned,created_at,updated_at FROM collections ORDER BY pinned DESC, name COLLATE NOCASE", nil, func(r *sql.Rows) error {
		var c domain.Collection
		e := r.Scan(&c.ID, &c.Name, &c.Description, &c.Entity, &c.Query, &c.Sort, &c.Group, &c.Pinned, &c.CreatedAt, &c.UpdatedAt)
		list = append(list, c)
		return e
	})
	return list, e
}

// SaveCollection stores a named filter. Only the query text is saved, so results are
// always computed from current data.
func (s *Store) SaveCollection(ctx context.Context, c domain.Collection) (domain.Collection, error) {
	c.Normalize()
	if e := c.Validate(); e != nil {
		return c, e
	}
	now := stamp(s.now())
	_, e := s.db.ExecContext(ctx, `INSERT INTO collections(id,name,description,entity,query,sort,grouping,pinned,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)
 ON CONFLICT(id) DO UPDATE SET name=excluded.name,description=excluded.description,entity=excluded.entity,query=excluded.query,sort=excluded.sort,
 grouping=excluded.grouping,pinned=excluded.pinned,updated_at=excluded.updated_at`, c.ID, c.Name, c.Description, c.Entity, c.Query, c.Sort, c.Group, c.Pinned, now, now)
	if e != nil && strings.Contains(e.Error(), "UNIQUE") {
		e = fmt.Errorf("a collection named %q already exists", c.Name)
	}
	return c, e
}

func (s *Store) DeleteCollection(ctx context.Context, id string) error {
	_, e := s.db.ExecContext(ctx, "DELETE FROM collections WHERE id=?", id)
	return e
}
