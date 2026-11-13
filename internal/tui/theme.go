package tui

import "github.com/allisonhere/tideui"

func resolveTheme(name string) tideui.Theme {
	t, _ := tideui.ThemeByName("catppuccin-mocha")
	if name == "light" {
		t, _ = tideui.ThemeByName("catppuccin-latte")
	}
	if name == "contrast" {
		t = tideui.Theme{Name: "contrast", Bg: "#000000", Fg: "#ffffff", Border: "#aaaaaa", BorderFocus: "#ffff00", Selected: "#555555", Unread: "#ffffff", Dimmed: "#cccccc", StatusBar: "#000000", StatusFg: "#ffffff", Error: "#ff7777", Overlay: "#000000", OverlayBorder: "#ffff00", Warning: "#ffff00", Success: "#88ff88"}
	}
	return t
}
