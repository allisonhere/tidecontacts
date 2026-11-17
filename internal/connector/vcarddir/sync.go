// Package vcarddir connects local vdirsyncer-style address books without taking ownership of their files.
package vcarddir

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/transfer"
)

func Prepare(ctx context.Context, path string, repo app.Directories) (domain.DirectorySnapshot, int, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return domain.DirectorySnapshot{}, 0, err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return domain.DirectorySnapshot{}, 0, err
	}
	books, err := repo.ContactDirectories(ctx)
	if err != nil {
		return domain.DirectorySnapshot{}, 0, err
	}
	revision := 0
	for _, book := range books {
		if book.Path == absolute {
			revision = book.Revision
		}
	}
	snapshot, err := transfer.ReadDirectory(ctx, absolute)
	return snapshot, revision, err
}
func Sync(ctx context.Context, path string, repo app.Directories) (domain.DirectoryReport, error) {
	snapshot, revision, err := Prepare(ctx, path, repo)
	if err != nil {
		return domain.DirectoryReport{Path: path}, err
	}
	return repo.ApplyContactDirectory(ctx, snapshot, revision)
}

// RefreshAll keeps the previous snapshot when one source fails; other books can still refresh.
func RefreshAll(ctx context.Context, repo app.Directories) ([]domain.DirectoryReport, string) {
	reports := []domain.DirectoryReport{}
	books, err := repo.ContactDirectories(ctx)
	if err != nil {
		return reports, err.Error()
	}
	failures := []string{}
	for _, book := range books {
		if book.LastRefreshed == "" {
			continue
		}
		report, err := Sync(ctx, book.Path, repo)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v (keeping last good contacts)", book.Path, err))
			continue
		}
		reports = append(reports, report)
	}
	return reports, strings.Join(failures, "; ")
}
func Summary(reports []domain.DirectoryReport) string {
	added, updated, removed := 0, 0, 0
	for _, r := range reports {
		added += r.Added
		updated += r.Updated
		removed += r.Removed
	}
	return fmt.Sprintf("%d folders refreshed · %d added · %d updated · %d removed", len(reports), added, updated, removed)
}
