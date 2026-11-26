package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"

	"github.com/allisonhere/tidecontacts/internal/domain"
)

const pipelineItemSummaryColumns = "i.id,i.title,i.pipeline_id,i.stage_id,g.name,i.stage_entered_at,i.revision"

func scanPipelineItemSummary(row interface{ Scan(...any) error }) (domain.PipelineItem, error) {
	var item domain.PipelineItem
	err := row.Scan(&item.ID, &item.Title, &item.PipelineID, &item.StageID, &item.Stage, &item.StageEnteredAt, &item.Revision)
	return item, err
}

// PipelineItemSummary reads one live item without notes, next-action text or link history.
func (s *Store) PipelineItemSummary(ctx context.Context, id string) (domain.PipelineItem, error) {
	return scanPipelineItemSummary(s.db.QueryRowContext(ctx, "SELECT "+pipelineItemSummaryColumns+itemFrom+
		"WHERE i.id=? AND i.deleted=0 AND i.archived=0 AND p.archived=0", id))
}

// PipelineItemsPage is a stable ID page of live items in one live pipeline.
func (s *Store) PipelineItemsPage(ctx context.Context, pipelineID, after string, limit int) ([]domain.PipelineItem, error) {
	out := []domain.PipelineItem{}
	err := s.read(ctx, func(tx *sql.Tx) error {
		var id string
		if err := tx.QueryRowContext(ctx, "SELECT id FROM pipelines WHERE id=? AND archived=0", pipelineID).Scan(&id); err != nil {
			return err
		}
		return each(ctx, tx, "SELECT "+pipelineItemSummaryColumns+itemFrom+`WHERE i.pipeline_id=? AND i.id>? AND i.deleted=0 AND i.archived=0 ORDER BY i.id LIMIT ?`,
			[]any{pipelineID, after, min(max(limit, 1), 101)}, func(row *sql.Rows) error {
				item, err := scanPipelineItemSummary(row)
				if err == nil {
					out = append(out, item)
				}
				return err
			})
	})
	return out, err
}

// FollowUpRevision reads the active entity's follow-up schedule and the opaque revision
// of its persisted settings. Computed changes as time passes do not change the revision.
func (s *Store) FollowUpRevision(ctx context.Context, id string) (domain.FollowUp, string, error) {
	var f domain.FollowUp
	var revision string
	err := s.read(ctx, func(tx *sql.Tx) error {
		var err error
		f, _, revision, err = s.followUpRevision(ctx, tx, id)
		return err
	})
	return f, revision, err
}

// SetFollowUpRevision replaces the settings only if the observed revision still matches.
func (s *Store) SetFollowUpRevision(ctx context.Context, f domain.FollowUp, expected string) (string, error) {
	var revision string
	err := s.write(ctx, func(tx *sql.Tx) error {
		current, updated, rev, err := s.followUpRevision(ctx, tx, f.Entity.ID)
		if err != nil {
			return err
		}
		if current.Entity.Kind != f.Entity.Kind {
			return sql.ErrNoRows
		}
		if rev != expected {
			return ErrConflict
		}
		if err := s.setFollowUp(ctx, tx, f, updated); err != nil {
			return err
		}
		_, _, revision, err = s.followUpRevision(ctx, tx, f.Entity.ID)
		return err
	})
	return revision, err
}

func (s *Store) followUpRevision(ctx context.Context, tx *sql.Tx, id string) (domain.FollowUp, string, string, error) {
	var updated string
	row := tx.QueryRowContext(ctx, "SELECT "+followUpColumns+`,COALESCE(f.updated_at,'')
 FROM entities e LEFT JOIN followup_prefs f ON f.entity_id=e.id WHERE e.id=? AND e.archived=0 AND e.deleted=0`, stamp(s.now()), id)
	f, err := s.scanFollowUp(followUpRevisionRow{row, &updated})
	if err != nil {
		return f, "", "", err
	}
	// Interactions can clear an explicit date without calling SetFollowUp, so the
	// revision includes the settings themselves, not just their last write time.
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%q %q %q %d %q %t %q %q", id, f.Entity.Kind, updated, f.Interval.Every, f.Interval.Unit, f.Never, f.NextDate, f.SnoozedUntil)
	return f, updated, fmt.Sprintf("%x", h.Sum(nil)), nil
}

type followUpRevisionRow struct {
	row     *sql.Row
	updated *string
}

func (r followUpRevisionRow) Scan(values ...any) error {
	return r.row.Scan(append(values, r.updated)...)
}
