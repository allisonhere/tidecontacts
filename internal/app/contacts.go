package app

import (
	"context"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
)

// Contacts is the application-owned persistence boundary shared by the UI and transfers.
type Contacts interface {
	List(context.Context, string, search.Query, string) ([]domain.Summary, error)
	Contact(context.Context, string) (domain.Contact, error)
	SaveContacts(context.Context, []domain.Contact, string) ([]domain.Contact, error)
	History(context.Context, string) ([]domain.History, error)
}
