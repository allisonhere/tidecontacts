package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"net/mail"
	"net/url"
	"strings"
)

// OpenReadOnly never creates a database or applies migrations. Completion must
// not initialize or upgrade the user's address book as a side effect of Tab.
func OpenReadOnly(ctx context.Context, path string) (*Store, error) {
	u := url.URL{Scheme: "file", Path: path}
	q := url.Values{"mode": {"ro"}, "_pragma": {"query_only(1)", "busy_timeout(5000)"}}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	var version int
	if err = db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0) FROM schema_migrations").Scan(&version); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("address book is unavailable; open TideContacts or run --check first: %w", err)
	}
	if version != len(migrations) {
		_ = db.Close()
		return nil, fmt.Errorf("address book schema %d needs TideContacts schema %d; open the matching app first", version, len(migrations))
	}
	return &Store{db: db}, nil
}

// EmailRecipients loads only the columns needed by recipient completion, in one
// query. kind="" includes both people and organizations. This is intentionally
// separate from full-text search, so typing cannot match private contact notes.
func (s *Store) EmailRecipients(ctx context.Context, kind string) ([]domain.Recipient, error) {
	out := []domain.Recipient{}
	err := each(ctx, s.db, `SELECT e.id,e.display_name,m.value,m.preferred,
 COALESCE((SELECT group_concat(o.display_name,', ') FROM memberships j JOIN entities o ON o.id=j.organization_id
 WHERE j.person_id=e.id AND o.deleted=0 AND o.archived=0),'')
 FROM methods m JOIN entities e ON e.id=m.entity_id
 WHERE m.kind='email' AND e.deleted=0 AND e.archived=0 AND (?='' OR e.kind=?)
 ORDER BY CASE e.kind WHEN 'person' THEN 0 ELSE 1 END,e.display_name COLLATE NOCASE,e.id,m.preferred DESC,m.id`, []any{kind, kind}, func(rows *sql.Rows) error {
		var r domain.Recipient
		err := rows.Scan(&r.ContactID, &r.Name, &r.Email, &r.Preferred, &r.Organization)
		out = append(out, r)
		return err
	})
	return out, err
}

// CaptureSender checks existing email methods and saves a new local contact atomically.
func (s *Store) CaptureSender(ctx context.Context, from *mail.Address) (string, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback() }()
	// Acquire SQLite's writer lock before the lookup so concurrent captures see each other.
	if _, err = tx.ExecContext(ctx, "UPDATE entities SET revision=revision WHERE 0"); err != nil {
		return "", false, err
	}
	var existing string
	err = each(ctx, tx, `SELECT e.id,m.value FROM methods m JOIN entities e ON e.id=m.entity_id
 WHERE m.kind='email' AND e.deleted=0 ORDER BY e.archived,e.id`, nil, func(rows *sql.Rows) error {
		var id, value string
		if err := rows.Scan(&id, &value); err != nil {
			return err
		}
		if address, err := mail.ParseAddress(value); existing == "" && err == nil && strings.EqualFold(address.Address, from.Address) {
			existing = id
		}
		return nil
	})
	if err != nil {
		return "", false, err
	}
	if existing != "" {
		return existing, false, nil
	}
	c := domain.NewContact("person")
	c.DisplayName = from.Name
	if c.DisplayName == "" {
		c.DisplayName = from.Address
	}
	value := from.Address
	if _, err := mail.ParseAddress(value); err != nil {
		value = (&mail.Address{Address: value}).String()
	}
	c.Methods = []domain.Method{{Kind: "email", Value: value, Preferred: true}}
	c.Normalize()
	if err = c.Validate(); err != nil {
		return "", false, err
	}
	if err = saveContact(ctx, tx, &c, "capture sender"); err != nil {
		return "", false, err
	}
	if err = reindex(ctx, tx, c.ID); err != nil {
		return "", false, err
	}
	return c.ID, true, tx.Commit()
}
