package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/google/uuid"
)

// TaskFilter is defined by the application layer.
type TaskFilter = app.TaskFilter

const taskColumns = `t.id,t.title,t.description,t.status,t.priority,t.due_date,t.due_time,t.start_date,t.timezone,t.snoozed_until,t.completed_at,
 COALESCE(t.series_id,''),t.occurrence,t.archived,t.deleted,t.created_at,t.updated_at,t.revision,
 COALESCE(r.freq,''),COALESCE(r.every,0),COALESCE(r.by_day,''),COALESCE(r.month_day,0),COALESCE(r.anchor,''),COALESCE(r.until,''),COALESCE(r.count,0),COALESCE(r.after_completion,0)`
const taskFrom = ` FROM tasks t LEFT JOIN recurrence_rules r ON r.id=t.series_id `

func scanRule(freq string, every int, byDay string, monthDay int, anchor, until string, count int, after bool) domain.Recurrence {
	r := domain.Recurrence{Freq: freq, MonthDay: monthDay, Anchor: anchor, Until: until, Count: count, AfterCompletion: after}
	if every > 1 {
		r.Interval = every
	}
	for _, d := range strings.Fields(byDay) {
		if n, e := strconv.Atoi(d); e == nil {
			r.ByDay = append(r.ByDay, n)
		}
	}
	return r
}

func scanTask(r interface{ Scan(...any) error }) (domain.Task, error) {
	var t domain.Task
	var freq, byDay, anchor, until string
	var every, monthDay, count int
	var after bool
	e := r.Scan(&t.ID, &t.Title, &t.Description, &t.Status, &t.Priority, &t.Due, &t.DueTime, &t.Start, &t.Timezone, &t.SnoozedUntil, &t.CompletedAt,
		&t.SeriesID, &t.Occurrence, &t.Archived, &t.Deleted, &t.CreatedAt, &t.UpdatedAt, &t.Revision, &freq, &every, &byDay, &monthDay, &anchor, &until, &count, &after)
	t.Recurrence = scanRule(freq, every, byDay, monthDay, anchor, until, count, after)
	return t, e
}

func saveRule(ctx context.Context, tx *sql.Tx, id string, r domain.Recurrence) error {
	days := make([]string, len(r.ByDay))
	for i, d := range r.ByDay {
		days[i] = strconv.Itoa(d)
	}
	_, e := tx.ExecContext(ctx, `INSERT INTO recurrence_rules(id,freq,every,by_day,month_day,anchor,until,count,after_completion) VALUES(?,?,?,?,?,?,?,?,?)
 ON CONFLICT(id) DO UPDATE SET freq=excluded.freq,every=excluded.every,by_day=excluded.by_day,month_day=excluded.month_day,anchor=excluded.anchor,
 until=excluded.until,count=excluded.count,after_completion=excluded.after_completion`,
		id, r.Freq, max(1, r.Interval), strings.Join(days, " "), r.MonthDay, r.Anchor, r.Until, r.Count, r.AfterCompletion)
	return e
}

func attachTaskLinks(ctx context.Context, q querier, list []domain.Task, full bool) error {
	ids := make([]string, len(list))
	for i, t := range list {
		ids[i] = t.ID
	}
	entities, e := links(ctx, q, "task", "entity", ids)
	if e != nil {
		return e
	}
	projects, e := links(ctx, q, "task", "project", ids)
	if e != nil {
		return e
	}
	tags, e := tagsOf(ctx, q, "task", ids)
	if e != nil {
		return e
	}
	var activities map[string][]domain.Ref
	if full {
		if activities, e = links(ctx, q, "task", "activity", ids); e != nil {
			return e
		}
	}
	for i := range list {
		id := list[i].ID
		list[i].Entities, list[i].Projects, list[i].Tags, list[i].Activities = entities[id], projects[id], tags[id], activities[id]
	}
	return nil
}

func (s *Store) taskWhere(f TaskFilter) (where, error) {
	w := where{}
	today := s.today()
	w.add("t.deleted=0")
	open := "t.status NOT IN ('completed','canceled')"
	awake := "(t.snoozed_until='' OR t.snoozed_until<=?)"
	archived := false
	switch f.View {
	case "inbox":
		w.add("t.status='inbox' AND "+awake, today)
	case "today":
		w.add(open+" AND "+awake+" AND ((t.due_date!='' AND t.due_date<=?) OR (t.start_date!='' AND t.start_date<=? AND t.status='scheduled'))", today, today, today)
	case "upcoming":
		w.add(open+" AND (t.due_date>? OR (t.due_date='' AND t.start_date>?) OR t.snoozed_until>?)", today, today, today)
	case "waiting":
		w.add("t.status='waiting' AND "+awake, today)
	case "open":
		w.add(open)
	}
	for _, flt := range f.Query.Filters {
		v := strings.ToLower(flt.Value)
		switch flt.Key {
		case "status":
			w.add("t.status=?", v)
		case "pri":
			p, _ := domain.ParsePriority(v)
			w.add("t.priority=?", p)
		case "due":
			if e := w.dateFilter("t.due_date", v, today); e != nil {
				return w, e
			}
		case "is":
			switch v {
			case "open":
				w.add(open)
			case "done":
				w.add("t.status='completed'")
			case "overdue":
				w.add(open+" AND t.due_date!='' AND t.due_date<?", today)
			case "today":
				w.add(open+" AND t.due_date=?", today)
			case "recurring":
				w.add("t.series_id IS NOT NULL")
			case "waiting":
				w.add("t.status='waiting'")
			case "snoozed":
				w.add("t.snoozed_until>?", today)
			case "unlinked":
				w.add("NOT EXISTS(SELECT 1 FROM task_entities j WHERE j.task_id=t.id)")
			case "archived":
				archived = true
			}
		}
	}
	if archived {
		w.add("t.archived=1")
	} else {
		w.add("t.archived=0")
	}
	w.linked(f.Query, "task", "t")
	w.terms(f.Query, "task", "t", "title")
	if f.EntityID != "" {
		w.add("EXISTS(SELECT 1 FROM task_entities j WHERE j.task_id=t.id AND j.entity_id=?)", f.EntityID)
	}
	if f.ProjectID != "" {
		w.add("EXISTS(SELECT 1 FROM task_projects j WHERE j.task_id=t.id AND j.project_id=?)", f.ProjectID)
	}
	return w, nil
}

// Tasks returns a bounded, sorted task list with people, projects and tags attached.
func (s *Store) Tasks(ctx context.Context, f TaskFilter) ([]domain.Task, error) {
	w, e := s.taskWhere(f)
	if e != nil {
		return nil, e
	}
	// Open work first; undated tasks after dated ones.
	order := "(t.status IN ('completed','canceled')), (t.due_date=''), t.due_date, t.due_time, t.priority DESC, t.title COLLATE NOCASE, t.id"
	switch f.Sort {
	case "priority":
		order = "(t.status IN ('completed','canceled')), t.priority DESC, (t.due_date=''), t.due_date, t.title COLLATE NOCASE, t.id"
	case "title":
		order = "t.title COLLATE NOCASE, t.id"
	case "updated":
		order = "t.updated_at DESC, t.id"
	case "created":
		order = "t.created_at DESC, t.id"
	}
	if f.View == "upcoming" {
		order = "MAX(t.snoozed_until, CASE WHEN t.due_date!='' THEN t.due_date ELSE t.start_date END), t.due_time, t.priority DESC, t.title COLLATE NOCASE, t.id"
	}
	limit := f.Limit
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	list := []domain.Task{}
	e = s.read(ctx, func(tx *sql.Tx) error {
		if e := each(ctx, tx, "SELECT "+taskColumns+taskFrom+"WHERE "+w.sql()+" ORDER BY "+order+" LIMIT ?", append(w.args, limit), func(r *sql.Rows) error {
			t, e := scanTask(r)
			list = append(list, t)
			return e
		}); e != nil {
			return e
		}
		return attachTaskLinks(ctx, tx, list, false)
	})
	return list, e
}

func (s *Store) Task(ctx context.Context, id string) (domain.Task, error) {
	var t domain.Task
	e := s.read(ctx, func(tx *sql.Tx) error {
		var e error
		t, e = readTask(ctx, tx, id)
		return e
	})
	return t, e
}

func readTask(ctx context.Context, tx *sql.Tx, id string) (domain.Task, error) {
	t, e := scanTask(tx.QueryRowContext(ctx, "SELECT "+taskColumns+taskFrom+"WHERE t.id=?", id))
	if e != nil {
		return t, e
	}
	list := []domain.Task{t}
	e = attachTaskLinks(ctx, tx, list, true)
	return list[0], e
}

// SaveTask creates or updates a task with its links, tags and recurrence rule in one transaction.
func (s *Store) SaveTask(ctx context.Context, t domain.Task) (domain.Task, error) {
	e := s.write(ctx, func(tx *sql.Tx) error { return saveTask(ctx, tx, &t, s.now(), true) })
	if e != nil {
		return t, e
	}
	return s.Task(ctx, t.ID)
}

func saveTask(ctx context.Context, tx *sql.Tx, t *domain.Task, now time.Time, replaceLinks bool) error {
	t.Normalize()
	if t.Status == "completed" && t.CompletedAt == "" {
		t.CompletedAt = stamp(now)
	}
	if t.Open() {
		t.CompletedAt = ""
	}
	if e := t.Validate(); e != nil {
		return e
	}
	var revision int
	var created string
	e := tx.QueryRowContext(ctx, "SELECT revision,created_at FROM tasks WHERE id=?", t.ID).Scan(&revision, &created)
	if e != nil && e != sql.ErrNoRows {
		return e
	}
	if e == nil && t.Revision != revision {
		return fmt.Errorf("task changed since it was opened; reload before saving")
	}
	if e == sql.ErrNoRows {
		created = stamp(now)
	}
	t.CreatedAt, t.UpdatedAt, t.Revision = created, stamp(now), revision+1
	var series any
	if t.SeriesID != "" {
		series = t.SeriesID
		if !t.Recurrence.IsZero() {
			if e = saveRule(ctx, tx, t.SeriesID, t.Recurrence); e != nil {
				return e
			}
		}
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO tasks(id,title,description,status,priority,due_date,due_time,start_date,timezone,snoozed_until,completed_at,series_id,occurrence,archived,deleted,created_at,updated_at,revision)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET title=excluded.title,description=excluded.description,status=excluded.status,priority=excluded.priority,
 due_date=excluded.due_date,due_time=excluded.due_time,start_date=excluded.start_date,timezone=excluded.timezone,snoozed_until=excluded.snoozed_until,completed_at=excluded.completed_at,
 series_id=excluded.series_id,occurrence=excluded.occurrence,archived=excluded.archived,deleted=excluded.deleted,updated_at=excluded.updated_at,revision=excluded.revision`,
		t.ID, t.Title, t.Description, t.Status, t.Priority, t.Due, t.DueTime, t.Start, t.Timezone, t.SnoozedUntil, t.CompletedAt, series, t.Occurrence, t.Archived, t.Deleted, t.CreatedAt, t.UpdatedAt, t.Revision); e != nil {
		return e
	}
	if replaceLinks {
		for target, refs := range map[string][]domain.Ref{"entity": t.Entities, "project": t.Projects, "activity": t.Activities} {
			if e = setLinks(ctx, tx, "task", t.ID, target, refs); e != nil {
				return e
			}
		}
		if e = setTags(ctx, tx, "task", t.ID, t.Tags); e != nil {
			return e
		}
	}
	return reindexRecord(ctx, tx, "task", t.ID)
}

var completionNamespace = uuid.MustParse("6f1d5f0e-7a0b-4c53-9f0e-1c2a7d0b9e11")

// CompleteTask marks a task done. It is idempotent: completing twice changes nothing.
// A task linked to people, organizations or projects gets one timeline entry; a
// recurring task gets exactly one successor, guarded by a unique (series, occurrence) key.
func (s *Store) CompleteTask(ctx context.Context, id string) (domain.Task, error) {
	e := s.write(ctx, func(tx *sql.Tx) error {
		t, e := readTask(ctx, tx, id)
		if e != nil {
			return e
		}
		if t.Status == "completed" {
			return nil
		}
		now := s.now()
		t.Status, t.CompletedAt, t.SnoozedUntil = "completed", stamp(now), ""
		if e = saveTask(ctx, tx, &t, now, false); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE reminders SET status='dismissed',updated_at=? WHERE task_id=? AND status='pending'", stamp(now), id); e != nil {
			return e
		}
		if len(t.Entities)+len(t.Projects) > 0 {
			a := domain.Activity{ID: uuid.NewSHA1(completionNamespace, []byte(id)).String(), Type: "task-completion", Title: "Completed: " + t.Title,
				OccurredAt: now, Entities: t.Entities, Projects: t.Projects, Tasks: []domain.Ref{{ID: id}}}
			_ = tx.QueryRowContext(ctx, "SELECT revision FROM activities WHERE id=?", a.ID).Scan(&a.Revision)
			if e = saveActivity(ctx, tx, &a, now, true); e != nil {
				return e
			}
		}
		next, ok := t.Recurrence.Advance(t.Due, domain.Day(now), domain.Day(now), t.Occurrence)
		if !ok || t.SeriesID == "" {
			return nil
		}
		var exists int
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM tasks WHERE series_id=? AND occurrence=?", t.SeriesID, t.Occurrence+1).Scan(&exists); e != nil || exists > 0 {
			return e
		}
		successor := domain.Task{ID: uuid.NewString(), Title: t.Title, Description: t.Description, Status: "scheduled", Priority: t.Priority, Due: next, DueTime: t.DueTime,
			Timezone: t.Timezone, Recurrence: t.Recurrence, SeriesID: t.SeriesID, Occurrence: t.Occurrence + 1, Entities: t.Entities, Projects: t.Projects, Tags: t.Tags}
		if t.Start != "" && t.Due != "" {
			successor.Start = domain.AddDays(next, domain.DaysBetween(t.Due, t.Start))
		}
		return saveTask(ctx, tx, &successor, now, true)
	})
	if e != nil {
		return domain.Task{}, e
	}
	return s.Task(ctx, id)
}

// ReopenTask returns a completed or canceled task to active work and withdraws its
// completion entry from the timeline. An already-created recurring successor is kept.
func (s *Store) ReopenTask(ctx context.Context, id string) (domain.Task, error) {
	e := s.write(ctx, func(tx *sql.Tx) error {
		t, e := readTask(ctx, tx, id)
		if e != nil || t.Open() {
			return e
		}
		t.Status = "next"
		if t.Due != "" {
			t.Status = "scheduled"
		}
		if e = saveTask(ctx, tx, &t, s.now(), false); e != nil {
			return e
		}
		completion := uuid.NewSHA1(completionNamespace, []byte(id)).String()
		if _, e = tx.ExecContext(ctx, "UPDATE activities SET deleted=1,revision=revision+1,updated_at=? WHERE id=? AND deleted=0", stamp(s.now()), completion); e != nil {
			return e
		}
		return reindexRecord(ctx, tx, "activity", completion)
	})
	if e != nil {
		return domain.Task{}, e
	}
	return s.Task(ctx, id)
}

// DuplicateTask copies a task as a fresh, non-recurring inbox-free task with the same links.
func (s *Store) DuplicateTask(ctx context.Context, id string) (domain.Task, error) {
	var copy domain.Task
	e := s.write(ctx, func(tx *sql.Tx) error {
		t, e := readTask(ctx, tx, id)
		if e != nil {
			return e
		}
		copy = t
		copy.ID, copy.Revision, copy.SeriesID, copy.Occurrence, copy.Recurrence = uuid.NewString(), 0, "", 0, domain.Recurrence{}
		copy.CompletedAt, copy.SnoozedUntil, copy.Archived, copy.Activities = "", "", false, nil
		if !t.Open() {
			copy.Status = "next"
		}
		return saveTask(ctx, tx, &copy, s.now(), true)
	})
	if e != nil {
		return copy, e
	}
	return s.Task(ctx, copy.ID)
}

// DeleteTask tombstones a task; its reminders are dismissed.
func (s *Store) DeleteTask(ctx context.Context, id string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		now := stamp(s.now())
		if _, e := tx.ExecContext(ctx, "UPDATE tasks SET deleted=1,updated_at=?,revision=revision+1 WHERE id=?", now, id); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, "UPDATE reminders SET status='dismissed',updated_at=? WHERE task_id=? AND status='pending'", now, id); e != nil {
			return e
		}
		return reindexRecord(ctx, tx, "task", id)
	})
}

const reminderColumns = `m.id,m.title,m.note,m.remind_at,m.timezone,m.status,m.snoozed_until,m.created_at,m.updated_at,COALESCE(m.task_id,''),COALESCE(k.title,''),
 COALESCE(m.entity_id,''),COALESCE(e.display_name,''),COALESCE(e.kind,''),
 COALESCE(r.freq,''),COALESCE(r.every,0),COALESCE(r.by_day,''),COALESCE(r.month_day,0),COALESCE(r.anchor,''),COALESCE(r.until,''),COALESCE(r.count,0)`
const reminderFrom = ` FROM reminders m LEFT JOIN recurrence_rules r ON r.id=m.recurrence_id LEFT JOIN tasks k ON k.id=m.task_id LEFT JOIN entities e ON e.id=m.entity_id `

func (s *Store) scanReminder(r interface{ Scan(...any) error }) (domain.Reminder, error) {
	var m domain.Reminder
	var at, snoozed, freq, byDay, anchor, until string
	var every, monthDay, count int
	e := r.Scan(&m.ID, &m.Title, &m.Note, &at, &m.Timezone, &m.Status, &snoozed, &m.CreatedAt, &m.UpdatedAt, &m.Task.ID, &m.Task.Name,
		&m.Entity.ID, &m.Entity.Name, &m.Entity.Kind, &freq, &every, &byDay, &monthDay, &anchor, &until, &count)
	loc := s.now().Location()
	m.At = parseStamp(at).In(loc)
	if snoozed != "" {
		m.SnoozedUntil = parseStamp(snoozed).In(loc)
	}
	if m.Task.ID != "" {
		m.Task.Kind = "task"
	}
	m.Recurrence = scanRule(freq, every, byDay, monthDay, anchor, until, count, false)
	return m, e
}

// Reminders lists pending reminders ordered by when they are next due.
// With dueOnly, only reminders whose time has arrived are returned.
func (s *Store) Reminders(ctx context.Context, dueOnly bool) ([]domain.Reminder, error) {
	list := []domain.Reminder{}
	query := "SELECT " + reminderColumns + reminderFrom + "WHERE m.status='pending'"
	args := []any{}
	if dueOnly {
		query += " AND MAX(m.remind_at,m.snoozed_until)<=?"
		args = append(args, stamp(s.now()))
	}
	e := each(ctx, s.db, query+" ORDER BY MAX(m.remind_at,m.snoozed_until), m.id LIMIT 1000", args, func(r *sql.Rows) error {
		m, e := s.scanReminder(r)
		list = append(list, m)
		return e
	})
	return list, e
}

func (s *Store) SaveReminder(ctx context.Context, m domain.Reminder) (domain.Reminder, error) {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	m.Title = strings.TrimSpace(m.Title)
	if m.Title == "" {
		return m, fmt.Errorf("a reminder needs a title")
	}
	if m.At.IsZero() {
		return m, fmt.Errorf("a reminder needs a date")
	}
	if m.Status == "" {
		m.Status = "pending"
	}
	if m.Timezone == "" {
		m.Timezone = m.At.Location().String()
	}
	e := s.write(ctx, func(tx *sql.Tx) error {
		now := stamp(s.now())
		var rule, task, entity any
		if !m.Recurrence.IsZero() {
			if m.Recurrence.Anchor == "" {
				m.Recurrence.Anchor = domain.Day(m.At.In(domain.Zone(m.Timezone, s.now().Location())))
			}
			rule = m.ID
			if e := saveRule(ctx, tx, m.ID, m.Recurrence); e != nil {
				return e
			}
		}
		if m.Task.ID != "" {
			task = m.Task.ID
		}
		if m.Entity.ID != "" || m.Entity.Name != "" {
			refs, e := resolve(ctx, tx, "entity", []domain.Ref{m.Entity})
			if e != nil {
				return e
			}
			entity = refs[0].ID
		}
		snoozed := ""
		if !m.SnoozedUntil.IsZero() {
			snoozed = stamp(m.SnoozedUntil)
		}
		_, e := tx.ExecContext(ctx, `INSERT INTO reminders(id,title,note,remind_at,timezone,recurrence_id,task_id,entity_id,status,snoozed_until,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
 ON CONFLICT(id) DO UPDATE SET title=excluded.title,note=excluded.note,remind_at=excluded.remind_at,timezone=excluded.timezone,recurrence_id=excluded.recurrence_id,
 task_id=excluded.task_id,entity_id=excluded.entity_id,status=excluded.status,snoozed_until=excluded.snoozed_until,updated_at=excluded.updated_at`,
			m.ID, m.Title, m.Note, stamp(m.At), m.Timezone, rule, task, entity, m.Status, snoozed, now, now)
		return e
	})
	return m, e
}

// SnoozeReminder hides a reminder until the given instant.
func (s *Store) SnoozeReminder(ctx context.Context, id string, until time.Time) error {
	_, e := s.db.ExecContext(ctx, "UPDATE reminders SET snoozed_until=?,updated_at=? WHERE id=?", stamp(until), stamp(s.now()), id)
	return e
}

// DismissReminder acknowledges a reminder. A recurring reminder moves to its next
// occurrence after now, keeping its wall-clock time in its own zone, in the same row:
// repeating the call cannot create duplicates or a backlog.
func (s *Store) DismissReminder(ctx context.Context, id string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		m, e := s.scanReminder(tx.QueryRowContext(ctx, "SELECT "+reminderColumns+reminderFrom+"WHERE m.id=?", id))
		if e != nil {
			return e
		}
		now := s.now()
		if !m.Recurrence.IsZero() && m.Status == "pending" {
			loc := domain.Zone(m.Timezone, now.Location())
			local := m.At.In(loc)
			day, clock := domain.Day(local), local.Format("15:04")
			for i := 0; i < 5000; i++ {
				next, ok := m.Recurrence.Next(day)
				if !ok || m.Recurrence.Until != "" && next > m.Recurrence.Until {
					break
				}
				day = next
				if at, _ := domain.At(day, clock, loc); at.After(now) {
					_, e = tx.ExecContext(ctx, "UPDATE reminders SET remind_at=?,snoozed_until='',updated_at=? WHERE id=?", stamp(at), stamp(now), id)
					return e
				}
			}
		}
		_, e = tx.ExecContext(ctx, "UPDATE reminders SET status='dismissed',updated_at=? WHERE id=?", stamp(now), id)
		return e
	})
}
