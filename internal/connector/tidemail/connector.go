// Package tidemail reads TideMail's local cache without opening a network connection.
package tidemail

import (
	"context"
	"database/sql"
	"fmt"
	"net/mail"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/allisonhere/tidecontacts/internal/domain"
)

type Sink interface {
	UpsertExternalActivity(context.Context, domain.Activity, []string) (domain.Activity, error)
	Put(context.Context, string, string) error
}

type Result struct {
	Imported, Skipped int
	LastSynced        string
}

// Sync reads a consistent snapshot. Repeating a partial or complete run is safe:
// Message-ID identifies activities across folders and source cache rebuilds.
// Bodies and attachments are deliberately not copied. No contacts are created.
func Sync(ctx context.Context, path string, sink Sink) (Result, error) {
	var result Result
	if !filepath.IsAbs(path) {
		return result, fmt.Errorf("TideMail database path must be absolute")
	}
	u := url.URL{Scheme: "file", Path: path}
	q := url.Values{"mode": {"ro"}, "_pragma": {"query_only(1)", "busy_timeout(5000)"}}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return result, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	rows, err := db.QueryContext(ctx, `SELECT message_id,subject,from_addr,to_addr,cc_addr,date FROM messages ORDER BY id`)
	if err != nil {
		return result, fmt.Errorf("read TideMail cache: %w", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var ref, subject, from, to, cc string
		var date int64
		if err = rows.Scan(&ref, &subject, &from, &to, &cc, &date); err != nil {
			return result, err
		}
		ref = strings.Trim(strings.TrimSpace(ref), "<>")
		if ref == "" || date <= 0 || seen[ref] {
			result.Skipped++
			continue
		}
		seen[ref] = true
		addresses := []string{}
		for _, header := range []string{from, to, cc} {
			list, e := mail.ParseAddressList(header)
			if e != nil {
				continue
			}
			for _, addr := range list {
				addresses = append(addresses, strings.ToLower(addr.Address))
			}
		}
		title := strings.TrimSpace(subject)
		if title == "" {
			title = "(no subject)"
		}
		a := domain.Activity{Type: "email", Title: title, OccurredAt: time.Unix(date, 0).UTC(), Provenance: domain.External, SourceApp: "tidemail", ExternalRef: ref}
		if _, err = sink.UpsertExternalActivity(ctx, a, addresses); err != nil {
			return result, fmt.Errorf("import TideMail message: %w", err)
		}
		result.Imported++
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	result.LastSynced = time.Now().UTC().Format(time.RFC3339)
	if err = sink.Put(ctx, "tidemail-last-synced", result.LastSynced); err != nil {
		return result, err
	}
	return result, nil
}
