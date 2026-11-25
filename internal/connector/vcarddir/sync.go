// Package vcarddir connects local vdirsyncer-style address books without taking ownership of their files.
package vcarddir

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/connector/googlecontacts"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/secrets"
	"github.com/allisonhere/tidecontacts/internal/transfer"
)

// secretKey namespaces the keychain entry for a connected book's credential
// by its own path, so two Google accounts (or a Google account and an
// unrelated local folder) never collide.
func secretKey(path string) string { return "directory-password:" + path }

func revisionFor(path string, books []domain.ContactDirectory) int {
	for _, book := range books {
		if book.Path == path {
			return book.Revision
		}
	}
	return 0
}

// ConnectGoogle validates a Google account's contacts over CardDAV with an
// app password before anything is stored, then saves the password to the
// system keychain so later refreshes (Prepare/Sync) don't need it re-entered.
// A failed keychain write is reported rather than silently dropped: without
// it, the next refresh would have no password and fail.
func ConnectGoogle(ctx context.Context, email, appPassword string, repo app.Directories) (domain.DirectorySnapshot, int, error) {
	snapshot, err := googlecontacts.Fetch(ctx, email, appPassword)
	if err != nil {
		return domain.DirectorySnapshot{}, 0, err
	}
	books, err := repo.ContactDirectories(ctx)
	if err != nil {
		return domain.DirectorySnapshot{}, 0, err
	}
	if !secrets.Store(secretKey(snapshot.Path), appPassword) {
		_, reason := secrets.Usable()
		return snapshot, revisionFor(snapshot.Path, books), fmt.Errorf("connected, but could not save the app password to the system keychain (%s) — refresh will fail until reconnected", reason)
	}
	return snapshot, revisionFor(snapshot.Path, books), nil
}

func Prepare(ctx context.Context, path string, repo app.Directories) (domain.DirectorySnapshot, int, error) {
	if googlecontacts.IsAccount(path) {
		password := secrets.Lookup(secretKey(path))
		if password == "" {
			return domain.DirectorySnapshot{}, 0, fmt.Errorf("no saved app password for this account — disconnect and reconnect it")
		}
		snapshot, err := googlecontacts.Fetch(ctx, googlecontacts.Email(path), password)
		if err != nil {
			return domain.DirectorySnapshot{}, 0, err
		}
		books, err := repo.ContactDirectories(ctx)
		if err != nil {
			return domain.DirectorySnapshot{}, 0, err
		}
		return snapshot, revisionFor(path, books), nil
	}
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
	if err == nil && revision == 0 && len(snapshot.Cards) == 0 {
		return snapshot, revision, fmt.Errorf("folder is empty; choose the folder containing the vCard files: %s", absolute)
	}
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
	added, updated, removed, unchanged := 0, 0, 0, 0
	for _, r := range reports {
		added += r.Added
		updated += r.Updated
		removed += r.Removed
		unchanged += r.Unchanged
	}
	return fmt.Sprintf("%d folders refreshed · %d added · %d updated · %d removed · %d unchanged", len(reports), added, updated, removed, unchanged)
}
