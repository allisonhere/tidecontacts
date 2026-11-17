package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/mail"
	"sort"
	"strings"
	"unicode"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

func recipientQuery(ctx context.Context, s *sqlite.Store, o options) ([]domain.Recipient, error) {
	if len(o.query) > 1000 {
		return nil, invalid("recipient query exceeds 1000 bytes")
	}
	kind := ""
	if o.present["kind"] {
		kind = o.kind
	}
	rows, err := s.EmailRecipients(ctx, kind)
	if err != nil {
		return nil, err
	}
	tokens := strings.Fields(strings.ToLower(o.query))
	selected := []domain.Recipient{}
	seen := map[string]bool{}
	for _, r := range rows {
		if strings.IndexFunc(r.Email, func(ch rune) bool { return unicode.IsControl(ch) || unicode.Is(unicode.Cf, ch) }) >= 0 {
			continue
		}
		parsed, err := mail.ParseAddress(strings.TrimSpace(r.Email))
		if err != nil || parsed.Address == "" {
			continue
		}
		r.Email = parsed.Address
		if strings.IndexFunc(r.Email, func(ch rune) bool { return unicode.IsControl(ch) || unicode.Is(unicode.Cf, ch) }) >= 0 {
			continue
		}
		r.Name = singleLine(r.Name)
		r.Organization = singleLine(r.Organization)
		if r.Name == "" {
			r.Name = singleLine(parsed.Name)
		}
		haystack := strings.ToLower(r.Name + " " + r.Email + " " + r.Organization)
		match := true
		for _, token := range tokens {
			if !strings.Contains(haystack, token) {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		key := strings.ToLower(r.Email)
		if seen[key] {
			continue
		}
		seen[key] = true
		selected = append(selected, r)
	}
	sort.SliceStable(selected, func(i, j int) bool {
		a, b := selected[i], selected[j]
		if strings.EqualFold(a.Name, b.Name) {
			if a.ContactID != b.ContactID {
				return a.ContactID < b.ContactID
			}
			if a.Preferred != b.Preferred {
				return a.Preferred
			}
			return strings.ToLower(a.Email) < strings.ToLower(b.Email)
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return selected[:min(len(selected), o.limit)], nil
}
func singleLine(s string) string {
	s = strings.Map(func(ch rune) rune {
		if unicode.Is(unicode.Cf, ch) {
			return -1
		}
		if unicode.IsControl(ch) || unicode.IsSpace(ch) {
			return ' '
		}
		return ch
	}, s)
	return strings.Join(strings.Fields(s), " ")
}
func writeRecipients(out io.Writer, rows []domain.Recipient, format string) error {
	if format == "neomutt" && len(rows) == 0 {
		return fail(1, "no_matches", "No matching addresses")
	}
	var buffer bytes.Buffer
	if format == "neomutt" {
		fmt.Fprintf(&buffer, "TideContacts: %d matching address(es)\n", len(rows))
	}
	for _, r := range rows {
		email := r.Email
		// Quoted local parts need RFC 5322 quoting, not a raw space-containing addr.
		if parsed, err := mail.ParseAddress(email); err != nil || parsed.Address != email {
			email = (&mail.Address{Address: email}).String()
		}
		fmt.Fprintf(&buffer, "%s\t%s", email, r.Name)
		if format == "neomutt" {
			fmt.Fprintf(&buffer, "\t%s", r.Organization)
		}
		buffer.WriteByte('\n')
	}
	_, err := out.Write(buffer.Bytes())
	return err
}
