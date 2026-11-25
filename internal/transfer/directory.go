package transfer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/emersion/go-vcard"
)

// ReadDirectory produces a complete snapshot before any database change. A bad
// file fails the entire refresh so unreadable cards can never look like deletions.
func ReadDirectory(ctx context.Context, path string) (domain.DirectorySnapshot, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return domain.DirectorySnapshot{}, err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return domain.DirectorySnapshot{}, err
	}
	snapshot := domain.DirectorySnapshot{Path: absolute}
	folder, err := os.Open(absolute)
	if err != nil {
		return snapshot, err
	}
	defer func() { _ = folder.Close() }()
	entries, err := folder.ReadDir(10001)
	if err != nil && err != io.EOF {
		return snapshot, err
	}
	if len(entries) > 10000 {
		return snapshot, fmt.Errorf("directory exceeds 10,000 entries")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	seen := map[string]bool{}
	total := 0
	skippedFiles, subdirs := 0, 0
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return snapshot, err
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".vcf" && ext != ".vcard" {
			if entry.IsDir() {
				subdirs++
			} else {
				skippedFiles++
			}
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return snapshot, fmt.Errorf("%s: symlinked vCards are unsupported", entry.Name())
		}
		if entry.IsDir() {
			continue
		}
		data, err := readLocalFile(filepath.Join(absolute, entry.Name()))
		if err != nil {
			return snapshot, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		total += len(data)
		if total > 50<<20 {
			return snapshot, fmt.Errorf("directory exceeds 50 MiB")
		}
		cards, err := DecodeVCards(ctx, entry.Name(), data, seen)
		if err != nil {
			return snapshot, err
		}
		if len(snapshot.Cards)+len(cards) > 10000 {
			return snapshot, fmt.Errorf("directory exceeds 10,000 cards")
		}
		snapshot.Cards = append(snapshot.Cards, cards...)
	}
	if len(snapshot.Cards) == 0 && len(entries) > 0 {
		return snapshot, fmt.Errorf("no .vcf or .vcard files found directly in %s (%d subfolders and %d other files skipped; subfolders are not scanned, so choose the folder that holds the cards)", absolute, subdirs, skippedFiles)
	}
	return snapshot, nil
}

// DecodeVCards parses one vCard-format blob — a local file's bytes, or one
// fetched over the network — into directory cards. name identifies the
// source for error messages. seen deduplicates by UID (or by name+position,
// for a card with none) across every blob decoded into the same snapshot, so
// callers share one seen map across files or fetched contacts.
// dropInvalidWebsites removes website methods that still aren't a valid
// http(s) URL after Normalize's scheme-adding pass (for example an ftp://
// link, or a value Google exported with stray characters). A connected
// directory can't be fixed from inside TideContacts — the source account
// owns the data — so one unusable website drops silently rather than
// rejecting the whole card and blocking every other contact's sync.
func dropInvalidWebsites(c *domain.Contact) {
	if c.Website != "" && !domain.URLValid(c.Website) {
		c.Website = ""
	}
	kept := c.Methods[:0]
	for _, m := range c.Methods {
		if m.Kind == "website" && !domain.URLValid(m.Value) {
			continue
		}
		kept = append(kept, m)
	}
	c.Methods = kept
}

func DecodeVCards(ctx context.Context, name string, data []byte, seen map[string]bool) ([]domain.DirectoryCard, error) {
	var cards []domain.DirectoryCard
	decoder := vcard.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte{239, 187, 191})))
	count := 0
	for {
		if err := ctx.Err(); err != nil {
			return cards, err
		}
		card, err := decoder.Decode()
		if err == io.EOF {
			break
		}
		if err != nil {
			return cards, fmt.Errorf("%s: %w", name, err)
		}
		count++
		key := "uid:" + card.Value(vcard.FieldUID)
		if key == "uid:" {
			key = fmt.Sprintf("file:%s:%d", name, count)
		}
		if seen[key] {
			return cards, fmt.Errorf("%s: duplicate vCard UID %q", name, card.Value(vcard.FieldUID))
		}
		seen[key] = true
		raw, err := json.Marshal(card)
		if err != nil {
			return cards, err
		}
		fingerprint := sha256.Sum256(raw)
		// An exported TideContacts extension must not inject local identities or links.
		delete(card, "X-TIDECONTACTS")
		contact, err := fromCard(card)
		if err != nil {
			return cards, fmt.Errorf("%s: %w", name, err)
		}
		contact.ID = ""
		contact.Relationships = nil
		for i := range contact.Methods {
			contact.Methods[i].ID = ""
		}
		for i := range contact.Memberships {
			contact.Memberships[i].ID = ""
			contact.Memberships[i].OrganizationID = ""
		}
		contact.Normalize()
		dropInvalidWebsites(&contact)
		if err := contact.Validate(); err != nil {
			return cards, fmt.Errorf("%s, card %d: %w", name, count, err)
		}
		cards = append(cards, domain.DirectoryCard{Key: key, Fingerprint: hex.EncodeToString(fingerprint[:]), Contact: contact})
	}
	if count == 0 {
		return cards, fmt.Errorf("%s contains no vCards", name)
	}
	return cards, nil
}
