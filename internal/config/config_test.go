package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsAndInvalidConfig(t *testing.T) {
	if e := Defaults().Validate(); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "config.toml")
	c, e := Load(p)
	if e != nil || c.DefaultWorkspace != "people" {
		t.Fatal(c, e)
	}
	for _, s := range []string{`theme="unknown"`, `default_workspace="missing"`, `log_level="loud"`, `[layout]
left=0.9`, `[keybindings]
quit=["j"]`, `[keybindings]
quit=["ctrl+i"]`, `[keybindings]
quit=["ctrl+c"]`, `[keybindings]
missing=["x"]`, `typo=true`, `[keybindings]
quit=["banana"]`} {
		if e = os.WriteFile(p, []byte(s), 0600); e != nil {
			t.Fatal(e)
		}
		if _, e = Load(p); e == nil {
			t.Fatalf("accepted %s", s)
		}
		b, _ := os.ReadFile(p)
		if string(b) != s {
			t.Fatal("modified invalid file")
		}
	}
}
func TestXDG(t *testing.T) {
	base := t.TempDir()
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
		t.Setenv(key, base)
	}
	p, e := Resolve()
	if e != nil {
		t.Fatal(e)
	}
	if p.Database != filepath.Join(base, "tidecontacts", "tidecontacts.db") {
		t.Fatal(p)
	}
	t.Setenv("XDG_DATA_HOME", "relative")
	if _, e = Resolve(); e == nil {
		t.Fatal("relative XDG accepted")
	}
}
func TestKeyOverride(t *testing.T) {
	c := Defaults()
	c.Keybindings["quit"] = []string{"x"}
	r, e := c.Registry()
	if e != nil {
		t.Fatal(e)
	}
	if r.Key("x") != "quit" || r.Key("q") != "" {
		t.Fatal("override failed")
	}
}
