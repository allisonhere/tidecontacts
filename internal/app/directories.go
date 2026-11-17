package app

import (
	"context"
	"github.com/allisonhere/tidecontacts/internal/domain"
)

// Directories persists connected address books separately from local contacts.
type Directories interface {
	ContactDirectories(context.Context) ([]domain.ContactDirectory, error)
	ApplyContactDirectory(context.Context, domain.DirectorySnapshot, int) (domain.DirectoryReport, error)
	DisconnectContactDirectory(context.Context, string, int) error
}
