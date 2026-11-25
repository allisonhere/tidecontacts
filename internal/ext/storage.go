package ext

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Storage limits. Extension data is a small private key-value store owned by
// exactly one extension id and kept outside the core SQLite database.
const (
	MaxStorageKB      = 1024
	DefaultStorageKB  = 256
	maxStorageKeys    = 256
	maxStorageValue   = 64 << 10
	storageFileName   = "store.json"
	storageSchemaName = 1
)

// ErrQuota is returned when a write would exceed the storage allowance.
var ErrQuota = errors.New("extension storage quota exceeded")

// Storage is one extension's private key-value store at <root>/<id>/store.json.
// The path is derived only from a validated extension id, so one extension
// cannot name another's data or escape the root.
type Storage struct {
	mu       sync.Mutex
	dir      string
	id       string
	quotaKiB int
}

// OpenStorage prepares (lazily) the store for id under root.
func OpenStorage(root, id string, quotaKiB int) (*Storage, error) {
	if !ValidID(id) {
		return nil, fmt.Errorf("invalid extension id %q", id)
	}
	if quotaKiB <= 0 {
		quotaKiB = DefaultStorageKB
	}
	return &Storage{dir: filepath.Join(root, id), id: id, quotaKiB: min(quotaKiB, MaxStorageKB)}, nil
}

type storeFile struct {
	Schema int               `json:"schema"`
	Owner  string            `json:"owner"`
	Values map[string]string `json:"values"`
}

func (s *Storage) load() (storeFile, error) {
	f := storeFile{Schema: storageSchemaName, Owner: s.id, Values: map[string]string{}}
	data, err := os.ReadFile(filepath.Join(s.dir, storageFileName))
	if os.IsNotExist(err) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return f, fmt.Errorf("extension storage is corrupt: %w", err)
	}
	if f.Owner != s.id {
		return f, fmt.Errorf("extension storage belongs to %q", f.Owner)
	}
	if f.Values == nil {
		f.Values = map[string]string{}
	}
	return f, nil
}

func (s *Storage) save(f storeFile) error {
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if len(data) > s.quotaKiB<<10 {
		return ErrQuota
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, ".store-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	if serr := tmp.Sync(); werr == nil {
		werr = serr
	}
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(name)
		return werr
	}
	return os.Rename(name, filepath.Join(s.dir, storageFileName))
}

func checkKey(k string) error {
	if !keyPattern.MatchString(k) {
		return fmt.Errorf("storage key %q must be 1-40 characters of a-z, 0-9 and _", k)
	}
	return nil
}

// Get returns a value and whether it exists.
func (s *Storage) Get(key string) (string, bool, error) {
	if err := checkKey(key); err != nil {
		return "", false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load()
	v, ok := f.Values[key]
	return v, ok, err
}

// Put stores a value.
func (s *Storage) Put(key, value string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	if len(value) > maxStorageValue {
		return fmt.Errorf("storage value exceeds %d bytes", maxStorageValue)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load()
	if err != nil {
		return err
	}
	if _, ok := f.Values[key]; !ok && len(f.Values) >= maxStorageKeys {
		return ErrQuota
	}
	f.Values[key] = value
	return s.save(f)
}

// Delete removes a key.
func (s *Storage) Delete(key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load()
	if err != nil {
		return err
	}
	delete(f.Values, key)
	return s.save(f)
}

// Keys lists stored keys.
func (s *Storage) Keys() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load()
	keys := make([]string, 0, len(f.Values))
	for k := range f.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, err
}

// Purge deletes the extension's data directory, including any SQLite database
// it keeps there. It is only called on an explicit user request; disabling or
// removing an extension never destroys data. It refuses a directory that holds
// a manifest, so a misconfigured data root can never delete a program.
func (s *Storage) Purge() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(filepath.Join(s.dir, ManifestFile)); err == nil {
		return fmt.Errorf("refusing to purge %s: it contains an extension program", s.dir)
	}
	return os.RemoveAll(s.dir)
}
