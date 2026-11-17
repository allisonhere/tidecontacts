package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/google/uuid"
)

const directoriesSchema = `
CREATE TABLE contact_directories (
 id TEXT PRIMARY KEY, path TEXT NOT NULL UNIQUE, last_refreshed TEXT NOT NULL,
 revision INTEGER NOT NULL
);
CREATE TABLE directory_contacts (
 directory_id TEXT NOT NULL REFERENCES contact_directories(id) ON DELETE CASCADE,
 card_key TEXT NOT NULL, entity_id TEXT NOT NULL UNIQUE REFERENCES entities(id),
 fingerprint TEXT NOT NULL,
 PRIMARY KEY(directory_id,card_key)
);`

func (s *Store) ContactDirectories(ctx context.Context) ([]domain.ContactDirectory, error) {
	result := []domain.ContactDirectory{}
	err := each(ctx, s.db, `SELECT d.id,d.path,d.last_refreshed,d.revision,
 (SELECT count(*) FROM directory_contacts m JOIN entities e ON e.id=m.entity_id WHERE m.directory_id=d.id AND e.deleted=0)
 FROM contact_directories d ORDER BY d.path`, nil, func(rows *sql.Rows) error {
		var book domain.ContactDirectory
		if err := rows.Scan(&book.ID, &book.Path, &book.LastRefreshed, &book.Revision, &book.Contacts); err != nil {
			return err
		}
		result = append(result, book)
		return nil
	})
	return result, err
}

func directoryID(path string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("tidecontacts:directory:"+path)).String()
}
func directoryEntityID(book, key string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(book+":"+key)).String()
}

func (s *Store) ApplyContactDirectory(ctx context.Context, snapshot domain.DirectorySnapshot, expected int) (domain.DirectoryReport, error) {
	report := domain.DirectoryReport{Path: snapshot.Path}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return report, err
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return report, err
	}
	if snapshot.Path == "" {
		return report, fmt.Errorf("directory path is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return report, err
	}
	defer func() { _ = tx.Rollback() }()
	id := directoryID(snapshot.Path)
	var revision int
	err = tx.QueryRowContext(ctx, "SELECT revision FROM contact_directories WHERE id=?", id).Scan(&revision)
	if err != nil && err != sql.ErrNoRows {
		return report, err
	}
	if revision != expected {
		return report, fmt.Errorf("%w: directory refreshed elsewhere; retry", ErrConflict)
	}
	// Claim the write before loading records; racing refreshes cannot overwrite a newer snapshot.
	_, err = tx.ExecContext(ctx, `INSERT INTO contact_directories VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET last_refreshed=excluded.last_refreshed,revision=excluded.revision`, id, snapshot.Path, time.Now().UTC().Format(time.RFC3339Nano), revision+1)
	if err != nil {
		return report, err
	}
	type previous struct{ id, hash string }
	old := map[string]previous{}
	rows, err := tx.QueryContext(ctx, "SELECT card_key,entity_id,fingerprint FROM directory_contacts WHERE directory_id=?", id)
	if err != nil {
		return report, err
	}
	for rows.Next() {
		var key string
		var p previous
		if err = rows.Scan(&key, &p.id, &p.hash); err != nil {
			_ = rows.Close()
			return report, err
		}
		old[key] = p
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return report, err
	}
	// Directory-owned organizations preserve membership details without reusing or modifying local organizations.
	cards := append([]domain.DirectoryCard(nil), snapshot.Cards...)
	orgs := map[string]string{}
	for i := range cards {
		c := &cards[i].Contact
		for j := range c.Memberships {
			member := &c.Memberships[j]
			name := strings.TrimSpace(member.Organization)
			if name == "" {
				return report, fmt.Errorf("directory membership has no organization name")
			}
			key := "organization:" + strings.ToLower(name)
			member.OrganizationID = directoryEntityID(id, key)
			orgs[key] = name
		}
	}
	organizations := []domain.DirectoryCard{}
	for key, name := range orgs {
		c := domain.NewContact("organization")
		c.DisplayName = name
		organizations = append(organizations, domain.DirectoryCard{Key: key, Fingerprint: name, Contact: c})
	}
	cards = append(organizations, cards...)
	seen := map[string]bool{}
	for _, card := range cards {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if card.Key == "" || card.Fingerprint == "" || seen[card.Key] {
			return report, fmt.Errorf("duplicate or empty directory card identity")
		}
		seen[card.Key] = true
		p, exists := old[card.Key]
		if exists && p.hash == card.Fingerprint {
			report.Unchanged++
			continue
		}
		c := card.Contact
		c.ID = directoryEntityID(id, card.Key)
		c.Source = domain.DirectorySourcePrefix + id
		c.Deleted, c.Archived, c.Favorite = false, false, false
		c.Relationships = nil
		c.Revision = 0
		if exists {
			prior, e := readContact(ctx, tx, p.id)
			if e != nil {
				return report, e
			}
			if prior.Source != c.Source || prior.ID != c.ID {
				return report, fmt.Errorf("directory ownership mismatch")
			}
			c.Revision = prior.Revision
			report.Updated++
		} else {
			report.Added++
		}
		for i := range c.Methods {
			c.Methods[i].ID = directoryEntityID(c.ID, fmt.Sprintf("method:%d", i))
		}
		for i := range c.Memberships {
			c.Memberships[i].ID = directoryEntityID(c.ID, fmt.Sprintf("membership:%d", i))
		}
		c.Normalize()
		if err := c.Validate(); err != nil {
			return report, fmt.Errorf("%s: %w", card.Key, err)
		}
		if err := saveDirectoryContact(ctx, tx, &c, "directory refresh", true); err != nil {
			return report, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO directory_contacts VALUES(?,?,?,?) ON CONFLICT(directory_id,card_key) DO UPDATE SET fingerprint=excluded.fingerprint`, id, card.Key, c.ID, card.Fingerprint)
		if err != nil {
			return report, err
		}
	}
	for key, p := range old {
		if seen[key] || p.hash == "" {
			continue
		}
		c, e := readContact(ctx, tx, p.id)
		if e != nil {
			return report, e
		}
		if c.Source != domain.DirectorySourcePrefix+id {
			return report, fmt.Errorf("directory ownership mismatch")
		}
		c.Deleted = true
		if err := saveDirectoryContact(ctx, tx, &c, "removed from directory", true); err != nil {
			return report, err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE directory_contacts SET fingerprint='' WHERE directory_id=? AND card_key=?", id, key); err != nil {
			return report, err
		}
		report.Removed++
	}
	return report, tx.Commit()
}

// Disconnect removes the connection and hides its cached contacts, preserving history and CRM links.
func (s *Store) DisconnectContactDirectory(ctx context.Context, id string, expected int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var revision int
	if err = tx.QueryRowContext(ctx, "SELECT revision FROM contact_directories WHERE id=?", id).Scan(&revision); err != nil {
		return err
	}
	if revision != expected {
		return fmt.Errorf("%w: directory changed; reopen connections", ErrConflict)
	}
	rows, err := tx.QueryContext(ctx, "SELECT entity_id FROM directory_contacts WHERE directory_id=?", id)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var entity string
		if err = rows.Scan(&entity); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, entity)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, entity := range ids {
		c, e := readContact(ctx, tx, entity)
		if e != nil {
			return e
		}
		if c.Deleted {
			continue
		}
		c.Deleted = true
		if err := saveDirectoryContact(ctx, tx, &c, "directory disconnected", true); err != nil {
			return err
		}
	}
	// Keep mappings and revision so reconnecting revives the same identities.
	_, err = tx.ExecContext(ctx, "UPDATE contact_directories SET last_refreshed='',revision=revision+1 WHERE id=?", id)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE directory_contacts SET fingerprint='' WHERE directory_id=?", id)
	if err != nil {
		return err
	}
	return tx.Commit()
}
