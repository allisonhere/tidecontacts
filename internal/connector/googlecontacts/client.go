// Package googlecontacts fetches a Google account's contacts over its
// legacy CardDAV endpoint, authenticating with an app password rather than
// OAuth. An app password requires 2-Step Verification to be enabled on the
// account (https://myaccount.google.com/apppasswords); without it Google
// has no basic-auth path and this client cannot connect. The fetch is
// read-only: nothing is ever written back to Google.
package googlecontacts

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/transfer"
)

const (
	// PathPrefix marks a connected ContactDirectory as a Google account
	// rather than a local vCard folder; the rest of the path is the email.
	PathPrefix = "google://"

	baseURL    = "https://www.google.com/carddav/v1/principals/"
	maxCards   = 10000
	maxTotal   = 50 << 20
	httpBudget = 30 * time.Second
)

// IsAccount reports whether a ContactDirectory.Path identifies a connected
// Google account rather than a local folder.
func IsAccount(path string) bool { return strings.HasPrefix(path, PathPrefix) }

// Email extracts the account email from a Google-scheme path.
func Email(path string) string { return strings.TrimPrefix(path, PathPrefix) }

// Path builds the stable identifier stored as ContactDirectory.Path for a
// Google account.
func Path(email string) string { return PathPrefix + email }

// Fetch validates email/appPassword against Google's CardDAV collection and
// returns every contact as a directory snapshot. Nothing is written locally
// or remotely by this call; the caller decides whether to persist anything.
func Fetch(ctx context.Context, email, appPassword string) (domain.DirectorySnapshot, error) {
	collection := baseURL + url.PathEscape(email) + "/lists/default/"
	return fetchFrom(ctx, collection, email, appPassword)
}

// fetchFrom does the actual work against collection, a CardDAV address-book
// URL. Split out from Fetch so tests can point it at a fake server instead
// of Google's fixed endpoint.
func fetchFrom(ctx context.Context, collection, email, appPassword string) (domain.DirectorySnapshot, error) {
	snapshot := domain.DirectorySnapshot{Path: Path(email)}
	client := &http.Client{Timeout: httpBudget}

	hrefs, err := listCardHrefs(ctx, client, collection, email, appPassword)
	if err != nil {
		return snapshot, err
	}
	if len(hrefs) > maxCards {
		return snapshot, fmt.Errorf("account exceeds %d cards", maxCards)
	}
	seen := map[string]bool{}
	total := 0
	for i, href := range hrefs {
		if err := ctx.Err(); err != nil {
			return snapshot, err
		}
		data, err := getCard(ctx, client, href, email, appPassword)
		if err != nil {
			return snapshot, fmt.Errorf("fetching contact %d of %d: %w", i+1, len(hrefs), err)
		}
		total += len(data)
		if total > maxTotal {
			return snapshot, fmt.Errorf("account exceeds %d MiB", maxTotal>>20)
		}
		cards, err := transfer.DecodeVCards(ctx, fmt.Sprintf("contact %d", i+1), data, seen)
		if err != nil {
			return snapshot, err
		}
		snapshot.Cards = append(snapshot.Cards, cards...)
	}
	return snapshot, nil
}

type resourceType struct {
	Collection *struct{} `xml:"collection"`
}
type prop struct {
	ResourceType resourceType `xml:"resourcetype"`
}
type propstat struct {
	Prop prop `xml:"prop"`
}
type multistatusResponse struct {
	Href     string     `xml:"href"`
	Propstat []propstat `xml:"propstat"`
}
type multistatus struct {
	XMLName   xml.Name              `xml:"multistatus"`
	Responses []multistatusResponse `xml:"response"`
}

func (r multistatusResponse) isCollection() bool {
	for _, ps := range r.Propstat {
		if ps.Prop.ResourceType.Collection != nil {
			return true
		}
	}
	return false
}

// listCardHrefs runs a PROPFIND (Depth: 1) on the address-book collection
// and returns the absolute URL of every member resource, excluding the
// collection itself and any sub-collections. Google's CardDAV member hrefs
// are opaque IDs with no ".vcf" suffix (unlike most CardDAV servers), so
// membership is decided by resourcetype, not by filename.
func listCardHrefs(ctx context.Context, client *http.Client, collection, email, appPassword string) ([]string, error) {
	const body = `<?xml version="1.0" encoding="utf-8" ?>` +
		`<D:propfind xmlns:D="DAV:"><D:prop><D:resourcetype/><D:getetag/></D:prop></D:propfind>`
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", collection, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("Depth", "1")
	req.SetBasicAuth(email, appPassword)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach Google: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("Google rejected the email or app password (status %d)", resp.StatusCode)
	}
	if resp.StatusCode != 207 && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response listing contacts (status %d)", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	var ms multistatus
	if err := xml.Unmarshal(data, &ms); err != nil {
		return nil, fmt.Errorf("could not parse Google's directory listing: %w", err)
	}
	base, err := url.Parse(collection)
	if err != nil {
		return nil, err
	}
	collectionNormalized := strings.TrimRight(base.String(), "/")
	hrefs := []string{}
	for _, r := range ms.Responses {
		href := strings.TrimSpace(r.Href)
		if href == "" || r.isCollection() {
			continue
		}
		resolved, err := base.Parse(href)
		if err != nil {
			continue
		}
		if strings.TrimRight(resolved.String(), "/") == collectionNormalized {
			continue // the collection's own entry, not a card
		}
		hrefs = append(hrefs, resolved.String())
	}
	return hrefs, nil
}

func getCard(ctx context.Context, client *http.Client, href, email, appPassword string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, href, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(email, appPassword)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach Google: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response (status %d)", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}
