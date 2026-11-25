// Package observe wraps the SQLite store so successful writes publish domain
// events. Events are emitted only after the underlying call returns without
// error, i.e. after its transaction committed. A failed or rolled-back write
// publishes nothing.
//
// The wrapper embeds the concrete store, so every other method (directories,
// TideMail sink, key-value settings) is promoted unchanged. Directory refresh
// and TideMail activity sync are not wrapped in Phase 3A: a folder refresh can
// rewrite thousands of records and would flood subscribers with events about
// records the user did not touch.
package observe

import (
	"context"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/ext"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

// Store publishes events for writes made through it.
type Store struct {
	*sqlite.Store
	bus *ext.Bus
	src string
}

// Wrap returns a store that publishes to bus. src names the caller ("ui", "cli").
// A nil bus makes the wrapper a transparent pass-through.
func Wrap(s *sqlite.Store, bus *ext.Bus, src string) *Store {
	return &Store{Store: s, bus: bus, src: src}
}

func (s *Store) emit(typ, id string, revision int, source, corr string) {
	s.emitRel(typ, id, "", revision, source, corr)
}

func (s *Store) emitRel(typ, id, related string, revision int, source, corr string) {
	if s.bus != nil && id != "" {
		e := ext.NewEvent(typ, id, revision, source, corr)
		e.RelatedID = related
		s.bus.Publish(e)
	}
}

func (s *Store) sourceFor(action string) string {
	switch {
	case strings.HasPrefix(action, ext.ExtensionSource):
		return action
	case action == "import":
		return "import"
	case strings.HasPrefix(action, "CLI"):
		return "cli"
	}
	return s.src
}

func (s *Store) contactEvents(in, saved []domain.Contact, action string) {
	if action == "demo" || len(saved) == 0 {
		return
	}
	corr := ""
	if len(saved) > 1 {
		corr = ext.NewID()
	}
	source := s.sourceFor(action)
	for i, c := range saved {
		kind, created := "contact", i < len(in) && in[i].Revision == 0
		if c.Kind == "organization" {
			kind = "organization"
		}
		typ := kind + ".updated"
		switch {
		case c.Deleted:
			typ = kind + ".deleted"
		case created:
			typ = kind + ".created"
		case kind == "contact" && c.Archived:
			typ = "contact.archived"
		}
		s.emit(typ, c.ID, c.Revision, source, corr)
	}
}

// SaveContacts saves and then announces created, updated and archived contacts.
func (s *Store) SaveContacts(ctx context.Context, in []domain.Contact, action string) ([]domain.Contact, error) {
	return s.SaveContactsWithProgress(ctx, in, action, nil)
}

// SaveContactsWithProgress is the progress-reporting variant used by imports.
func (s *Store) SaveContactsWithProgress(ctx context.Context, in []domain.Contact, action string, progress func(int, int)) ([]domain.Contact, error) {
	saved, err := s.Store.SaveContactsWithProgress(ctx, in, action, progress)
	if err == nil {
		s.contactEvents(in, saved, action)
	}
	return saved, err
}

func (s *Store) MergeContacts(ctx context.Context, keepID, mergeID string) (domain.Contact, error) {
	c, err := s.Store.MergeContacts(ctx, keepID, mergeID)
	if err == nil {
		s.emitRel("contact.merged", c.ID, mergeID, c.Revision, s.src, "")
	}
	return c, err
}

func (s *Store) SaveActivity(ctx context.Context, a domain.Activity) (domain.Activity, error) {
	out, err := s.Store.SaveActivity(ctx, a)
	if err == nil {
		typ := "activity.updated"
		if a.Revision == 0 {
			typ = "activity.created"
		}
		source := s.src
		if strings.HasPrefix(a.SourceApp, ext.ExtensionSource) {
			source = a.SourceApp
		}
		s.emit(typ, out.ID, out.Revision, source, "")
	}
	return out, err
}

func (s *Store) DeleteActivity(ctx context.Context, id string) error {
	err := s.Store.DeleteActivity(ctx, id)
	if err == nil {
		s.emit("activity.deleted", id, 0, s.src, "")
	}
	return err
}

func (s *Store) SaveTask(ctx context.Context, t domain.Task) (domain.Task, error) {
	out, err := s.Store.SaveTask(ctx, t)
	if err == nil {
		typ := "task.updated"
		if t.Revision == 0 {
			typ = "task.created"
		}
		s.emit(typ, out.ID, out.Revision, s.src, "")
	}
	return out, err
}

func (s *Store) CompleteTask(ctx context.Context, id string) (domain.Task, error) {
	out, err := s.Store.CompleteTask(ctx, id)
	if err == nil {
		s.emit("task.completed", out.ID, out.Revision, s.src, "")
	}
	return out, err
}

func (s *Store) ReopenTask(ctx context.Context, id string) (domain.Task, error) {
	out, err := s.Store.ReopenTask(ctx, id)
	if err == nil {
		s.emit("task.reopened", out.ID, out.Revision, s.src, "")
	}
	return out, err
}

func (s *Store) SaveProject(ctx context.Context, p domain.Project) (domain.Project, error) {
	out, err := s.Store.SaveProject(ctx, p)
	if err == nil {
		typ := "project.updated"
		if p.Revision == 0 {
			typ = "project.created"
		}
		s.emit(typ, out.ID, out.Revision, s.src, "")
	}
	return out, err
}

func (s *Store) SavePipeline(ctx context.Context, p domain.Pipeline) (domain.Pipeline, error) {
	out, err := s.Store.SavePipeline(ctx, p)
	if err == nil {
		typ := "pipeline.updated"
		if p.ID == "" {
			typ = "pipeline.created"
		}
		s.emit(typ, out.ID, 0, s.src, "")
	}
	return out, err
}

func (s *Store) SaveItem(ctx context.Context, it domain.PipelineItem) (domain.PipelineItem, error) {
	out, err := s.Store.SaveItem(ctx, it)
	if err == nil && it.ID == "" {
		s.emit("pipeline.item_created", out.ID, 0, s.src, "")
	}
	return out, err
}

func (s *Store) MoveItem(ctx context.Context, id, stageID string) error {
	err := s.Store.MoveItem(ctx, id, stageID)
	if err == nil {
		s.emit("pipeline.item_moved", id, 0, s.src, "")
	}
	return err
}
