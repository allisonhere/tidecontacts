package sqlite

import (
	"context"
	"errors"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"testing"
)

func TestConditionalPipelineSaveRejectsConcurrentEdit(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	original, err := s.SavePipeline(ctx, domain.Pipeline{Name: "Original", Stages: []domain.Stage{{Name: "New"}}})
	if err != nil {
		t.Fatal(err)
	}
	changed := original
	changed.Name = "Other editor"
	if _, err := s.SavePipeline(ctx, changed); err != nil {
		t.Fatal(err)
	}
	stale := original
	stale.Name = "Stale editor"
	if _, err := s.SavePipelineIfUnchanged(ctx, stale, original); !errors.Is(err, ErrConflict) {
		t.Fatal("stale config accepted", err)
	}
	list, err := s.Pipelines(ctx, true)
	if err != nil || list[0].Name != "Other editor" {
		t.Fatal("stale write changed stored data", list, err)
	}
}
