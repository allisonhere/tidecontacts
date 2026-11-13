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

// Pipelines lists pipelines in user order with their stages and open item counts.
func (s *Store) Pipelines(ctx context.Context, archived bool) ([]domain.Pipeline, error) {
	list := []domain.Pipeline{}
	e := s.read(ctx, func(tx *sql.Tx) error {
		index := map[string]int{}
		if e := each(ctx, tx, "SELECT id,name,description,archived,position FROM pipelines WHERE archived=0 OR ? ORDER BY archived, position, name COLLATE NOCASE", []any{archived}, func(r *sql.Rows) error {
			var p domain.Pipeline
			e := r.Scan(&p.ID, &p.Name, &p.Description, &p.Archived, &p.Position)
			index[p.ID] = len(list)
			list = append(list, p)
			return e
		}); e != nil {
			return e
		}
		return each(ctx, tx, `SELECT g.id,g.pipeline_id,g.name,g.position,(SELECT count(*) FROM pipeline_items i WHERE i.stage_id=g.id AND i.deleted=0 AND i.archived=0)
 FROM pipeline_stages g ORDER BY g.pipeline_id, g.position`, nil, func(r *sql.Rows) error {
			var g domain.Stage
			if e := r.Scan(&g.ID, &g.PipelineID, &g.Name, &g.Position, &g.Count); e != nil {
				return e
			}
			if i, ok := index[g.PipelineID]; ok {
				list[i].Stages = append(list[i].Stages, g)
			}
			return nil
		})
	})
	return list, e
}

// SavePipeline creates, renames, reorders or archives a pipeline and rewrites its
// stage list. Stage order follows the slice. A stage that still holds items cannot
// be removed; move or archive its items first.
func (s *Store) SavePipeline(ctx context.Context, p domain.Pipeline) (domain.Pipeline, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return p, fmt.Errorf("a pipeline needs a name")
	}
	if len(p.Stages) == 0 {
		return p, fmt.Errorf("a pipeline needs at least one stage")
	}
	e := s.write(ctx, func(tx *sql.Tx) error {
		now := stamp(s.now())
		if p.ID == "" {
			p.ID = uuid.NewString()
			if e := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position),-1)+1 FROM pipelines").Scan(&p.Position); e != nil {
				return e
			}
		}
		if _, e := tx.ExecContext(ctx, `INSERT INTO pipelines(id,name,description,archived,position,created_at,updated_at) VALUES(?,?,?,?,?,?,?)
 ON CONFLICT(id) DO UPDATE SET name=excluded.name,description=excluded.description,archived=excluded.archived,position=excluded.position,updated_at=excluded.updated_at`,
			p.ID, p.Name, p.Description, p.Archived, p.Position, now, now); e != nil {
			return e
		}
		keep := map[string]bool{}
		names := map[string]bool{}
		for i := range p.Stages {
			g := &p.Stages[i]
			g.Name = strings.TrimSpace(g.Name)
			if g.Name == "" || names[strings.ToLower(g.Name)] {
				return fmt.Errorf("stage names must be non-empty and unique within a pipeline")
			}
			names[strings.ToLower(g.Name)] = true
			if g.ID == "" {
				g.ID = uuid.NewString()
			}
			g.PipelineID, g.Position = p.ID, i
			keep[g.ID] = true
			if _, e := tx.ExecContext(ctx, `INSERT INTO pipeline_stages(id,pipeline_id,name,position) VALUES(?,?,?,?)
 ON CONFLICT(id) DO UPDATE SET name=excluded.name,position=excluded.position`, g.ID, p.ID, g.Name, i); e != nil {
				return e
			}
		}
		type old struct {
			id, name string
			items    int
		}
		stale := []old{}
		if e := each(ctx, tx, "SELECT g.id,g.name,(SELECT count(*) FROM pipeline_items i WHERE i.stage_id=g.id) FROM pipeline_stages g WHERE g.pipeline_id=?", []any{p.ID}, func(r *sql.Rows) error {
			var o old
			e := r.Scan(&o.id, &o.name, &o.items)
			if !keep[o.id] {
				stale = append(stale, o)
			}
			return e
		}); e != nil {
			return e
		}
		for _, o := range stale {
			if o.items > 0 {
				return fmt.Errorf("stage %q still holds %d item(s); move them to another stage first", o.name, o.items)
			}
			if _, e := tx.ExecContext(ctx, "DELETE FROM pipeline_stages WHERE id=?", o.id); e != nil {
				return e
			}
		}
		return nil
	})
	return p, e
}

// MovePipeline changes a pipeline's position in the list by delta places.
func (s *Store) MovePipeline(ctx context.Context, id string, delta int) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		ids := []string{}
		if e := each(ctx, tx, "SELECT id FROM pipelines ORDER BY position, name COLLATE NOCASE", nil, func(r *sql.Rows) error {
			var v string
			e := r.Scan(&v)
			ids = append(ids, v)
			return e
		}); e != nil {
			return e
		}
		for i, v := range ids {
			if j := i + delta; v == id && j >= 0 && j < len(ids) {
				ids[i], ids[j] = ids[j], ids[i]
				break
			}
		}
		for i, v := range ids {
			if _, e := tx.ExecContext(ctx, "UPDATE pipelines SET position=? WHERE id=?", i, v); e != nil {
				return e
			}
		}
		return nil
	})
}

const itemColumns = `i.id,i.pipeline_id,i.stage_id,g.name,p.name,i.title,i.notes,i.next_action,i.stage_entered_at,i.archived,i.deleted,i.created_at,i.updated_at,i.revision`
const itemFrom = ` FROM pipeline_items i JOIN pipeline_stages g ON g.id=i.stage_id JOIN pipelines p ON p.id=i.pipeline_id `

func scanItem(r interface{ Scan(...any) error }) (domain.PipelineItem, error) {
	var i domain.PipelineItem
	e := r.Scan(&i.ID, &i.PipelineID, &i.StageID, &i.Stage, &i.Pipeline, &i.Title, &i.Notes, &i.NextAction, &i.StageEnteredAt, &i.Archived, &i.Deleted, &i.CreatedAt, &i.UpdatedAt, &i.Revision)
	return i, e
}

func attachItemLinks(ctx context.Context, q querier, list []domain.PipelineItem, full bool) error {
	ids := make([]string, len(list))
	for i, it := range list {
		ids[i] = it.ID
	}
	sets := map[string]map[string][]domain.Ref{}
	targets := []string{"entity"}
	if full {
		targets = []string{"entity", "project", "task", "activity"}
	}
	for _, target := range targets {
		m, e := links(ctx, q, "item", target, ids)
		if e != nil {
			return e
		}
		sets[target] = m
	}
	tags, e := tagsOf(ctx, q, "item", ids)
	if e != nil {
		return e
	}
	for i := range list {
		id := list[i].ID
		list[i].Entities, list[i].Projects, list[i].Tasks, list[i].Activities, list[i].Tags = sets["entity"][id], sets["project"][id], sets["task"][id], sets["activity"][id], tags[id]
	}
	return nil
}

// Items lists pipeline items oldest-in-stage first. pipelineID may be empty to search every pipeline.
func (s *Store) Items(ctx context.Context, pipelineID string, q search.Query) ([]domain.PipelineItem, error) {
	w := where{}
	w.add("i.deleted=0")
	if pipelineID != "" {
		w.add("i.pipeline_id=?", pipelineID)
	}
	archived := false
	for _, f := range q.Filters {
		switch f.Key {
		case "pipeline":
			w.add("instr(lower(p.name),lower(?))>0", f.Value)
		case "stage":
			w.add("instr(lower(g.name),lower(?))>0", f.Value)
		case "is":
			archived = strings.EqualFold(f.Value, "archived")
		}
	}
	if archived {
		w.add("i.archived=1")
	} else {
		w.add("i.archived=0")
	}
	w.linked(q, "item", "i")
	w.terms(q, "item", "i", "title")
	list := []domain.PipelineItem{}
	e := s.read(ctx, func(tx *sql.Tx) error {
		if e := each(ctx, tx, "SELECT "+itemColumns+itemFrom+"WHERE "+w.sql()+" ORDER BY p.position, g.position, i.stage_entered_at, i.rowid LIMIT 5000", w.args, func(r *sql.Rows) error {
			it, e := scanItem(r)
			list = append(list, it)
			return e
		}); e != nil {
			return e
		}
		return attachItemLinks(ctx, tx, list, false)
	})
	return list, e
}

func (s *Store) Item(ctx context.Context, id string) (domain.PipelineItem, error) {
	var it domain.PipelineItem
	e := s.read(ctx, func(tx *sql.Tx) error {
		var e error
		if it, e = scanItem(tx.QueryRowContext(ctx, "SELECT "+itemColumns+itemFrom+"WHERE i.id=?", id)); e != nil {
			return e
		}
		list := []domain.PipelineItem{it}
		if e = attachItemLinks(ctx, tx, list, true); e != nil {
			return e
		}
		it = list[0]
		return each(ctx, tx, "SELECT from_stage,to_stage,at FROM pipeline_stage_history WHERE item_id=? ORDER BY id", []any{id}, func(r *sql.Rows) error {
			var c domain.StageChange
			e := r.Scan(&c.From, &c.To, &c.At)
			it.History = append(it.History, c)
			return e
		})
	})
	return it, e
}

// SaveItem creates or edits a pipeline item. Entering a stage, on creation or by an
// edit, appends to the stage history with stage names copied so history survives renames.
func (s *Store) SaveItem(ctx context.Context, it domain.PipelineItem) (domain.PipelineItem, error) {
	e := s.write(ctx, func(tx *sql.Tx) error {
		it.Title = strings.TrimSpace(it.Title)
		if it.Title == "" {
			return fmt.Errorf("a pipeline item needs a title")
		}
		if it.ID == "" {
			it.ID = uuid.NewString()
		}
		now := stamp(s.now())
		if it.StageID == "" {
			// Default to the pipeline's first stage, or resolve a stage by name.
			e := tx.QueryRowContext(ctx, "SELECT id FROM pipeline_stages WHERE pipeline_id=? AND (?='' OR name=? COLLATE NOCASE) ORDER BY position LIMIT 1", it.PipelineID, it.Stage, it.Stage).Scan(&it.StageID)
			if e != nil {
				return fmt.Errorf("choose an existing stage of this pipeline")
			}
		}
		var stage, pipeline string
		if e := tx.QueryRowContext(ctx, "SELECT name,pipeline_id FROM pipeline_stages WHERE id=?", it.StageID).Scan(&stage, &pipeline); e != nil {
			return fmt.Errorf("choose an existing stage of this pipeline")
		}
		it.PipelineID = pipeline
		var revision int
		var created, oldStage, entered string
		e := tx.QueryRowContext(ctx, "SELECT i.revision,i.created_at,g.name,i.stage_entered_at FROM pipeline_items i JOIN pipeline_stages g ON g.id=i.stage_id WHERE i.id=?", it.ID).Scan(&revision, &created, &oldStage, &entered)
		if e != nil && e != sql.ErrNoRows {
			return e
		}
		if e == nil && it.Revision != revision {
			return fmt.Errorf("item changed since it was opened; reload before saving")
		}
		if e == sql.ErrNoRows {
			created = now
		}
		insertHistory := e == sql.ErrNoRows || oldStage != stage
		if insertHistory {
			entered = now
		}
		it.CreatedAt, it.UpdatedAt, it.Revision, it.StageEnteredAt = created, now, revision+1, entered
		if _, e = tx.ExecContext(ctx, `INSERT INTO pipeline_items(id,pipeline_id,stage_id,title,notes,next_action,stage_entered_at,archived,deleted,created_at,updated_at,revision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
 ON CONFLICT(id) DO UPDATE SET pipeline_id=excluded.pipeline_id,stage_id=excluded.stage_id,title=excluded.title,notes=excluded.notes,next_action=excluded.next_action,
 stage_entered_at=excluded.stage_entered_at,archived=excluded.archived,deleted=excluded.deleted,updated_at=excluded.updated_at,revision=excluded.revision`,
			it.ID, it.PipelineID, it.StageID, it.Title, it.Notes, it.NextAction, it.StageEnteredAt, it.Archived, it.Deleted, it.CreatedAt, it.UpdatedAt, it.Revision); e != nil {
			return e
		}
		if insertHistory {
			if _, e = tx.ExecContext(ctx, "INSERT INTO pipeline_stage_history(item_id,from_stage,to_stage,at) VALUES(?,?,?,?)", it.ID, oldStage, stage, now); e != nil {
				return e
			}
		}
		for target, refs := range map[string][]domain.Ref{"entity": it.Entities, "project": it.Projects, "task": it.Tasks, "activity": it.Activities} {
			if e = setLinks(ctx, tx, "item", it.ID, target, refs); e != nil {
				return e
			}
		}
		if e = setTags(ctx, tx, "item", it.ID, it.Tags); e != nil {
			return e
		}
		return reindexRecord(ctx, tx, "item", it.ID)
	})
	if e != nil {
		return it, e
	}
	return s.Item(ctx, it.ID)
}

// MoveItem places an item in another stage of its pipeline and records the transition.
// Moving to the stage it is already in changes nothing.
func (s *Store) MoveItem(ctx context.Context, id, stageID string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var from, to, pipeline, target string
		if e := tx.QueryRowContext(ctx, "SELECT g.name,i.pipeline_id FROM pipeline_items i JOIN pipeline_stages g ON g.id=i.stage_id WHERE i.id=?", id).Scan(&from, &pipeline); e != nil {
			return e
		}
		if e := tx.QueryRowContext(ctx, "SELECT name,pipeline_id FROM pipeline_stages WHERE id=?", stageID).Scan(&to, &target); e != nil || target != pipeline {
			return fmt.Errorf("that stage belongs to a different pipeline")
		}
		if from == to {
			return nil
		}
		now := stamp(s.now())
		if _, e := tx.ExecContext(ctx, "UPDATE pipeline_items SET stage_id=?,stage_entered_at=?,updated_at=?,revision=revision+1 WHERE id=?", stageID, now, now, id); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO pipeline_stage_history(item_id,from_stage,to_stage,at) VALUES(?,?,?,?)", id, from, to, now)
		return e
	})
}

// DeleteItem tombstones a pipeline item; its stage history is kept.
func (s *Store) DeleteItem(ctx context.Context, id string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(ctx, "UPDATE pipeline_items SET deleted=1,updated_at=?,revision=revision+1 WHERE id=?", stamp(s.now()), id); e != nil {
			return e
		}
		return reindexRecord(ctx, tx, "item", id)
	})
}
