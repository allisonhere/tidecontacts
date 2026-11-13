package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
	"github.com/google/uuid"
)

const projectColumns = `p.id,p.name,p.description,p.notes,p.status,p.priority,p.start_date,p.target_date,p.deleted,p.created_at,p.updated_at,p.revision,
 (SELECT count(*) FROM task_projects j JOIN tasks t ON t.id=j.task_id WHERE j.project_id=p.id AND t.deleted=0 AND t.status NOT IN ('completed','canceled')),
 (SELECT count(*) FROM task_projects j JOIN tasks t ON t.id=j.task_id WHERE j.project_id=p.id AND t.deleted=0 AND t.status='completed')`

func scanProject(r interface{ Scan(...any) error }) (domain.Project, error) {
	var p domain.Project
	e := r.Scan(&p.ID, &p.Name, &p.Description, &p.Notes, &p.Status, &p.Priority, &p.Start, &p.Target, &p.Deleted, &p.CreatedAt, &p.UpdatedAt, &p.Revision, &p.OpenTasks, &p.DoneTasks)
	return p, e
}

func attachProjectLinks(ctx context.Context, q querier, list []domain.Project) error {
	ids := make([]string, len(list))
	index := map[string]int{}
	for i, p := range list {
		ids[i] = p.ID
		index[p.ID] = i
	}
	members, e := links(ctx, q, "project", "entity", ids)
	if e != nil {
		return e
	}
	tags, e := tagsOf(ctx, q, "project", ids)
	if e != nil {
		return e
	}
	for i := range list {
		list[i].Members, list[i].Tags = members[list[i].ID], tags[list[i].ID]
	}
	for start := 0; start < len(ids); start += 500 {
		chunk := ids[start:min(len(ids), start+500)]
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		if e = each(ctx, q, "SELECT id,project_id,title,due_date,done_at,position FROM milestones WHERE project_id IN (?"+strings.Repeat(",?", len(chunk)-1)+") ORDER BY position, due_date, title", args, func(r *sql.Rows) error {
			var m domain.Milestone
			if e := r.Scan(&m.ID, &m.ProjectID, &m.Title, &m.Due, &m.DoneAt, &m.Position); e != nil {
				return e
			}
			i := index[m.ProjectID]
			list[i].Milestones = append(list[i].Milestones, m)
			return nil
		}); e != nil {
			return e
		}
	}
	return nil
}

// Projects lists projects. An empty status shows everything except archived projects.
func (s *Store) Projects(ctx context.Context, status string, q search.Query) ([]domain.Project, error) {
	w := where{}
	w.add("p.deleted=0")
	today := s.today()
	explicit := status != ""
	if status != "" && status != "all" {
		w.add("p.status=?", status)
	}
	for _, f := range q.Filters {
		v := strings.ToLower(f.Value)
		switch f.Key {
		case "status":
			explicit = true
			w.add("p.status=?", v)
		case "pri":
			p, _ := domain.ParsePriority(v)
			w.add("p.priority=?", p)
		case "is":
			switch v {
			case "open":
				w.add("p.status IN ('planned','active','on-hold')")
			case "archived":
				explicit = true
				w.add("p.status='archived'")
			case "overdue":
				w.add(`p.status IN ('planned','active','on-hold') AND ((p.target_date!='' AND p.target_date<?) OR
 EXISTS(SELECT 1 FROM milestones m WHERE m.project_id=p.id AND m.done_at='' AND m.due_date!='' AND m.due_date<?))`, today, today)
			case "waiting":
				w.add("EXISTS(SELECT 1 FROM task_projects j JOIN tasks t ON t.id=j.task_id WHERE j.project_id=p.id AND t.deleted=0 AND t.status='waiting')")
			}
		}
	}
	if !explicit {
		w.add("p.status!='archived'")
	}
	w.linked(q, "project", "p")
	w.terms(q, "project", "p", "name")
	list := []domain.Project{}
	e := s.read(ctx, func(tx *sql.Tx) error {
		if e := each(ctx, tx, "SELECT "+projectColumns+" FROM projects p WHERE "+w.sql()+
			" ORDER BY CASE p.status WHEN 'active' THEN 0 WHEN 'planned' THEN 1 WHEN 'on-hold' THEN 2 WHEN 'completed' THEN 3 ELSE 4 END, p.priority DESC, p.name COLLATE NOCASE, p.id LIMIT 2000", w.args, func(r *sql.Rows) error {
			p, e := scanProject(r)
			list = append(list, p)
			return e
		}); e != nil {
			return e
		}
		return attachProjectLinks(ctx, tx, list)
	})
	return list, e
}

func (s *Store) Project(ctx context.Context, id string) (domain.Project, error) {
	var p domain.Project
	e := s.read(ctx, func(tx *sql.Tx) error {
		var e error
		if p, e = scanProject(tx.QueryRowContext(ctx, "SELECT "+projectColumns+" FROM projects p WHERE p.id=?", id)); e != nil {
			return e
		}
		list := []domain.Project{p}
		e = attachProjectLinks(ctx, tx, list)
		p = list[0]
		return e
	})
	return p, e
}

// SaveProject writes a project with its members, tags and milestones atomically.
// A status change is recorded on the project timeline.
func (s *Store) SaveProject(ctx context.Context, p domain.Project) (domain.Project, error) {
	e := s.write(ctx, func(tx *sql.Tx) error {
		p.Normalize()
		if e := p.Validate(); e != nil {
			return e
		}
		now := s.now()
		var revision int
		var created, oldStatus string
		e := tx.QueryRowContext(ctx, "SELECT revision,created_at,status FROM projects WHERE id=?", p.ID).Scan(&revision, &created, &oldStatus)
		if e != nil && e != sql.ErrNoRows {
			return e
		}
		if e == nil && p.Revision != revision {
			return fmt.Errorf("project changed since it was opened; reload before saving")
		}
		if e == sql.ErrNoRows {
			created = stamp(now)
		}
		p.CreatedAt, p.UpdatedAt, p.Revision = created, stamp(now), revision+1
		if _, e = tx.ExecContext(ctx, `INSERT INTO projects(id,name,description,notes,status,priority,start_date,target_date,deleted,created_at,updated_at,revision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
 ON CONFLICT(id) DO UPDATE SET name=excluded.name,description=excluded.description,notes=excluded.notes,status=excluded.status,priority=excluded.priority,
 start_date=excluded.start_date,target_date=excluded.target_date,deleted=excluded.deleted,updated_at=excluded.updated_at,revision=excluded.revision`,
			p.ID, p.Name, p.Description, p.Notes, p.Status, p.Priority, p.Start, p.Target, p.Deleted, p.CreatedAt, p.UpdatedAt, p.Revision); e != nil {
			return e
		}
		if e = setLinks(ctx, tx, "project", p.ID, "entity", p.Members); e != nil {
			return e
		}
		if e = setTags(ctx, tx, "project", p.ID, p.Tags); e != nil {
			return e
		}
		keep := []any{p.ID}
		for i, m := range p.Milestones {
			keep = append(keep, m.ID)
			if _, e = tx.ExecContext(ctx, `INSERT INTO milestones(id,project_id,title,due_date,done_at,position) VALUES(?,?,?,?,?,?)
 ON CONFLICT(id) DO UPDATE SET title=excluded.title,due_date=excluded.due_date,done_at=excluded.done_at,position=excluded.position`, m.ID, p.ID, strings.TrimSpace(m.Title), m.Due, m.DoneAt, i); e != nil {
				return e
			}
		}
		if _, e = tx.ExecContext(ctx, "DELETE FROM milestones WHERE project_id=? AND id NOT IN (''"+strings.Repeat(",?", len(keep)-1)+")", keep...); e != nil {
			return e
		}
		if oldStatus != "" && oldStatus != p.Status {
			a := domain.Activity{ID: uuid.NewString(), Type: "project-update", Title: fmt.Sprintf("%s: %s → %s", p.Name, oldStatus, p.Status), OccurredAt: now, Projects: []domain.Ref{{ID: p.ID}}}
			if e = saveActivity(ctx, tx, &a, now, true); e != nil {
				return e
			}
		}
		return reindexRecord(ctx, tx, "project", p.ID)
	})
	if e != nil {
		return p, e
	}
	return s.Project(ctx, p.ID)
}

// DeleteProject tombstones a project. Tasks and activities keep existing without it.
func (s *Store) DeleteProject(ctx context.Context, id string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(ctx, "UPDATE projects SET deleted=1,updated_at=?,revision=revision+1 WHERE id=?", stamp(s.now()), id); e != nil {
			return e
		}
		return reindexRecord(ctx, tx, "project", id)
	})
}
