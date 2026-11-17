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
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return snapshot, err
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".vcf" && ext != ".vcard" {
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
		decoder := vcard.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte{239, 187, 191})))
		count := 0
		for {
			if err := ctx.Err(); err != nil {
				return snapshot, err
			}
			card, err := decoder.Decode()
			if err == io.EOF {
				break
			}
			if err != nil {
				return snapshot, fmt.Errorf("%s: %w", entry.Name(), err)
			}
			count++
			if len(snapshot.Cards) >= 10000 {
				return snapshot, fmt.Errorf("directory exceeds 10,000 cards")
			}
			key := "uid:" + card.Value(vcard.FieldUID)
			if key == "uid:" {
				key = fmt.Sprintf("file:%s:%d", entry.Name(), count)
			}
			if seen[key] {
				return snapshot, fmt.Errorf("%s: duplicate vCard UID %q", entry.Name(), card.Value(vcard.FieldUID))
			}
			seen[key] = true
			raw, err := json.Marshal(card)
			if err != nil {
				return snapshot, err
			}
			fingerprint := sha256.Sum256(raw)
			// An exported TideContacts extension must not inject local identities or links.
			delete(card, "X-TIDECONTACTS")
			contact, err := fromCard(card)
			if err != nil {
				return snapshot, fmt.Errorf("%s: %w", entry.Name(), err)
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
			if err := contact.Validate(); err != nil {
				return snapshot, fmt.Errorf("%s, card %d: %w", entry.Name(), count, err)
			}
			snapshot.Cards = append(snapshot.Cards, domain.DirectoryCard{Key: key, Fingerprint: hex.EncodeToString(fingerprint[:]), Contact: contact})
		}
		if count == 0 {
			return snapshot, fmt.Errorf("%s contains no vCards", entry.Name())
		}
	}
	return snapshot, nil
}
