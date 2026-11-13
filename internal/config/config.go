package config

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

type Config struct {
	Density          string              `toml:"density"`
	Theme            string              `toml:"theme"`
	DefaultWorkspace string              `toml:"default_workspace"`
	LogLevel         string              `toml:"log_level"`
	Mouse            bool                `toml:"mouse"`
	Layout           Layout              `toml:"layout"`
	Keybindings      map[string][]string `toml:"keybindings"`
}
type Layout struct {
	Narrow int     `toml:"narrow"`
	Wide   int     `toml:"wide"`
	Left   float64 `toml:"left"`
	Right  float64 `toml:"right"`
}

func Defaults() Config {
	return Config{Density: "compact", Theme: "dark", DefaultWorkspace: "people", LogLevel: "info", Layout: Layout{90, 140, .23, .25}, Keybindings: map[string][]string{}}
}
func (c Config) Validate() error {
	if c.Density != "compact" && c.Density != "comfortable" {
		return fmt.Errorf("density must be compact or comfortable")
	}
	if c.Theme != "dark" && c.Theme != "light" && c.Theme != "contrast" {
		return fmt.Errorf("theme must be dark, light or contrast")
	}
	if domain.WorkspaceIndex(c.DefaultWorkspace) < 0 {
		return fmt.Errorf("unknown default_workspace %q", c.DefaultWorkspace)
	}
	if c.LogLevel != "debug" && c.LogLevel != "info" && c.LogLevel != "warn" && c.LogLevel != "error" {
		return fmt.Errorf("log_level must be debug, info, warn or error")
	}
	if c.Layout.Narrow < 40 || c.Layout.Wide < c.Layout.Narrow+20 || c.Layout.Wide > 1000 {
		return fmt.Errorf("layout requires narrow >= 40 and wide between narrow+20 and 1000")
	}
	if !(c.Layout.Left >= .15 && c.Layout.Left <= .35 && c.Layout.Right >= .15 && c.Layout.Right <= .35) {
		return fmt.Errorf("pane ratios must be between 0.15 and 0.35")
	}
	_, err := c.Registry()
	return err
}
func canonical(k string) string {
	switch k {
	case "ctrl+i":
		return "tab"
	case "ctrl+m":
		return "enter"
	case "ctrl+[":
		return "esc"
	case "ctrl+h":
		return "backspace"
	}
	return k
}
func validKey(k string) bool {
	if utf8.RuneCountInString(k) == 1 && !strings.ContainsAny(k, "\n\r\t\x1b") {
		return true
	}
	for _, s := range []string{"up", "down", "left", "right", "tab", "shift+tab", "enter", "esc", "backspace", "home", "end", "pgup", "pgdown", "space", "shift+left", "shift+right", "shift+up", "shift+down"} {
		if k == s {
			return true
		}
	}
	return len(k) == 6 && strings.HasPrefix(k, "ctrl+") && k[5] >= 'a' && k[5] <= 'z'
}
func (c Config) Registry() (app.Registry, error) {
	r := app.Registry{}
	known := map[string]bool{}
	used := map[string]string{"ctrl+c": "safe exit", "ctrl+z": "suspend"}
	// Keys the user bound explicitly win over the soft defaults of newer commands.
	claimed := map[string]bool{}
	for _, keys := range c.Keybindings {
		for _, key := range keys {
			claimed[canonical(key)] = true
		}
	}
	for _, cmd := range app.Defaults().Commands() {
		known[cmd.ID] = true
		if keys, ok := c.Keybindings[cmd.ID]; ok {
			cmd.Keys = keys
		} else if cmd.Soft {
			free := []string{}
			for _, key := range cmd.Keys {
				if !claimed[key] {
					free = append(free, key)
				}
			}
			cmd.Keys = free
		}
		normalized := []string{}
		for _, key := range cmd.Keys {
			key = canonical(key)
			if !validKey(key) {
				return r, fmt.Errorf("invalid key %q for %s", key, cmd.ID)
			}
			if old, ok := used[key]; ok {
				return r, fmt.Errorf("key %q conflicts: %s and %s", key, old, cmd.ID)
			}
			used[key] = cmd.ID
			normalized = append(normalized, key)
		}
		cmd.Keys = normalized
		if err := r.Register(cmd); err != nil {
			return r, err
		}
	}
	for id := range c.Keybindings {
		if !known[id] {
			return r, fmt.Errorf("unknown command %q", id)
		}
	}
	return r, nil
}
func Load(path string) (Config, error) {
	c := Defaults()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	meta, err := toml.Decode(string(data), &c)
	if err == nil && len(meta.Undecoded()) > 0 {
		err = fmt.Errorf("unknown setting %s", meta.Undecoded()[0])
	}
	if err == nil {
		err = c.Validate()
	}
	if err != nil {
		return c, fmt.Errorf("%s: %w (edit this file to correct the setting)", path, err)
	}
	return c, nil
}

type Paths struct{ Config, Database, Log string }

func Resolve() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	conf, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, err
	}
	data, state := filepath.Join(home, ".local", "share"), filepath.Join(home, ".local", "state")
	if runtime.GOOS == "darwin" {
		data = filepath.Join(home, "Library", "Application Support")
		state = filepath.Join(home, "Library", "Logs")
	}
	if runtime.GOOS == "windows" {
		data = conf
		state = conf
	}
	for key, dst := range map[string]*string{"XDG_CONFIG_HOME": &conf, "XDG_DATA_HOME": &data, "XDG_STATE_HOME": &state} {
		if v := os.Getenv(key); v != "" {
			if !filepath.IsAbs(v) {
				return Paths{}, fmt.Errorf("%s must be absolute", key)
			}
			*dst = v
		}
	}
	return Paths{filepath.Join(conf, "tidecontacts", "config.toml"), filepath.Join(data, "tidecontacts", "tidecontacts.db"), filepath.Join(state, "tidecontacts", "tidecontacts.log")}, nil
}
