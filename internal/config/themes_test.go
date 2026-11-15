package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTideMailTheme(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	if _, err := TideMailTheme(); err == nil {
		t.Fatal("missing installation accepted")
	}
	dir := filepath.Join(root, "tidemail")
	os.MkdirAll(dir, 0700)
	for _, name := range []string{"oxocarbon-dark", "catppuccin-latte"} {
		os.WriteFile(filepath.Join(dir, "config.toml"), []byte("theme = \""+name+"\"\n"), 0600)
		got, err := TideMailTheme()
		if err != nil || got != name {
			t.Fatal(got, err)
		}
	}
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte("theme = 'unsupported'"), 0600)
	if _, err := TideMailTheme(); err == nil {
		t.Fatal("unsupported theme accepted")
	}
}
