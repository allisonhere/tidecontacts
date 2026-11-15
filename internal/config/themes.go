package config

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/allisonhere/tideui"
	"os"
	"path/filepath"
)

func ValidTheme(name string) bool {
	if name == "dark" || name == "light" || name == "contrast" || name == "match-tidemail" {
		return true
	}
	_, ok := tideui.ThemeByName(name)
	return ok
}

func TideMailTheme() (string, error) {
	root := os.Getenv("XDG_CONFIG_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".config")
	}
	var c struct {
		Theme string `toml:"theme"`
	}
	if _, err := toml.DecodeFile(filepath.Join(root, "tidemail", "config.toml"), &c); err != nil {
		return "", err
	}
	if c.Theme == "" {
		c.Theme = "catppuccin-mocha"
	}
	if _, ok := tideui.ThemeByName(c.Theme); !ok {
		return "", fmt.Errorf("TideMail theme %q is not supported yet", c.Theme)
	}
	return c.Theme, nil
}
