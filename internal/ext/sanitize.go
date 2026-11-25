package ext

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Clean makes untrusted text safe to place in the terminal: it removes ANSI
// escape sequences, control characters, newlines (unless keepNewlines), and
// bidirectional overrides, then truncates to max runes.
func Clean(s string, max int, keepNewlines bool) string {
	s = ansi.Strip(strings.ToValidUTF8(s, "�"))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' && keepNewlines:
			b.WriteRune(r)
		case r == '\t' || r == '\n' || r == '\r':
			b.WriteRune(' ')
		case unicode.IsControl(r):
		case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0x200e, r == 0x200f, r == 0x061c, r == 0xfeff:
		default:
			b.WriteRune(r)
		}
	}
	out := b.String()
	if utf8.RuneCountInString(out) > max {
		out = string([]rune(out)[:max])
	}
	return out
}

// Line is one host-rendered row.
type Line struct {
	Label string `json:"label,omitempty"`
	Text  string `json:"text"`
	Level string `json:"level,omitempty"` // info (default), ok, warn
}

// Content is declarative output from an extension: the host chooses layout,
// color and focus. Extensions cannot emit styling or terminal sequences.
type Content struct {
	Title string `json:"title,omitempty"`
	Lines []Line `json:"lines"`
}

const (
	maxContentLines = 40
	maxLineRunes    = 160
)

// SanitizeContent bounds and cleans untrusted content.
func SanitizeContent(c Content) Content {
	out := Content{Title: Clean(c.Title, 60, false)}
	for i, l := range c.Lines {
		if i == maxContentLines {
			out.Lines = append(out.Lines, Line{Text: "… output truncated", Level: "warn"})
			break
		}
		level := l.Level
		if level != "ok" && level != "warn" {
			level = "info"
		}
		out.Lines = append(out.Lines, Line{Label: Clean(l.Label, 24, false), Text: Clean(l.Text, maxLineRunes, false), Level: level})
	}
	return out
}
