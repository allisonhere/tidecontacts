package ext

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// stateEntry is what the user decided about one extension.
type stateEntry struct {
	Enabled bool     `json:"enabled"`
	Granted []string `json:"granted_permissions,omitempty"`
	// ExecSHA256 is the executable's digest when the user approved it. A changed
	// binary is surfaced for re-approval instead of running silently.
	ExecSHA256 string         `json:"executable_sha256,omitempty"`
	Version    string         `json:"approved_version,omitempty"`
	Config     map[string]any `json:"config,omitempty"`
}

type stateFile struct {
	Version    int                   `json:"version"`
	Extensions map[string]stateEntry `json:"extensions"`
}

func readState(path string) (stateFile, error) {
	s := stateFile{Version: 1, Extensions: map[string]stateEntry{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if len(data) > 1<<20 {
		return stateFile{Version: 1, Extensions: map[string]stateEntry{}}, fmt.Errorf("extension state file is too large")
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return stateFile{Version: 1, Extensions: map[string]stateEntry{}}, fmt.Errorf("extension state file is unreadable: %w", err)
	}
	if s.Extensions == nil {
		s.Extensions = map[string]stateEntry{}
	}
	return s, nil
}

func writeState(path string, s stateFile) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".extensions-*")
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
	return os.Rename(name, path)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
