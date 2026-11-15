package tidemail

import (
	"os"
	"path/filepath"
)

// DefaultPath follows TideMail's own dataDir convention on every platform.
// Resolving the path does not create or open the cache.
func DefaultPath() (string, error) {
	root := os.Getenv("XDG_DATA_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".local", "share")
	}
	return filepath.Abs(filepath.Join(root, "tidemail", "mail.db"))
}
