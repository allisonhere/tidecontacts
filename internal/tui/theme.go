package tui

import (
	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/lipgloss"
)

func resolveTheme(name string) tideui.Theme {
	t, _ := tideui.ThemeByName(name)
	if name == "light" {
		t, _ = tideui.ThemeByName("catppuccin-latte")
	}
	if name == "contrast" {
		t = tideui.Theme{Name: "contrast", Bg: "#000000", Fg: "#ffffff", Border: "#aaaaaa", BorderFocus: "#ffff00", Selected: "#555555", Unread: "#ffffff", Dimmed: "#cccccc", StatusBar: "#000000", StatusFg: "#ffffff", Error: "#ff7777", Overlay: "#000000", OverlayBorder: "#ffff00", Warning: "#ffff00", Success: "#88ff88"}
	}
	return t
}

func (m Model) theme() tideui.Theme {
	name := m.cfg.Theme
	if name == "match-tidemail" {
		name = m.cfg.TideMailThemeName
	}
	return resolveTheme(name)
}

func contactBand(theme tideui.Theme, index int) lipgloss.Color {
	if index%2 == 0 {
		return theme.Bg
	}
	if theme.Name == "contrast" && theme.Overlay == theme.Bg {
		return "#202020"
	}
	return theme.Overlay
}
