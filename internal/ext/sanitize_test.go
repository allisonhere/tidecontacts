package ext

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCleanRemovesTerminalControls(t *testing.T) {
	cases := map[string]string{
		"\x1b[31mred\x1b[0m":                          "red",
		"\x1b]0;window title\x07shown":                "shown",
		"\x1b]8;;http://evil\x1b\\link\x1b]8;;\x1b\\": "link",
		"a\x00b\x07c\x7fd":                            "abcd",
		"line1\nline2\r\ttab":                         "line1 line2  tab",
		"safe‮txt.exe":                                "safetxt.exe",
		"\u009b31mC1 CSI":                             "31mC1 CSI",
		"\x9b31mraw byte":                             "�31mraw byte",
		"bad\xffbytes":                                "bad�bytes",
	}
	for in, want := range cases {
		got := Clean(in, 100, false)
		if strings.ContainsAny(got, "\x1b\x07\x00\x7f\r\n\t‮") {
			t.Errorf("%q: controls survived: %q", in, got)
		}
		if got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
	if got := Clean(strings.Repeat("é", 500), 10, false); utf8.RuneCountInString(got) != 10 {
		t.Fatalf("not truncated by runes: %d", utf8.RuneCountInString(got))
	}
	if got := Clean("a\nb", 10, true); got != "a\nb" {
		t.Fatalf("newline option: %q", got)
	}
}

func TestSanitizeContentBounds(t *testing.T) {
	c := Content{Title: strings.Repeat("t", 500)}
	for i := 0; i < 500; i++ {
		c.Lines = append(c.Lines, Line{Label: strings.Repeat("l", 99), Text: strings.Repeat("x", 999), Level: "<script>"})
	}
	out := SanitizeContent(c)
	if len(out.Lines) != maxContentLines+1 || utf8.RuneCountInString(out.Title) != 60 || utf8.RuneCountInString(out.Lines[0].Text) != maxLineRunes || out.Lines[0].Level != "info" {
		t.Fatalf("unbounded content: %d lines, title %d", len(out.Lines), utf8.RuneCountInString(out.Title))
	}
}

func FuzzParseManifest(f *testing.F) {
	f.Add([]byte("id = \"demo-one\"\nname = \"Demo\"\nversion = \"1.0.0\"\napi = 1\ncommand = \"run\"\n"))
	f.Add([]byte("id = ["))
	f.Add([]byte("\x00\xff"))
	f.Add([]byte("id=\"a\"\n[[provides]]\nid=\"core.x\"\nversion=1"))
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := ParseManifest(data)
		if err == nil {
			// Anything accepted must satisfy the invariants the host relies on.
			if !ValidID(m.ID) || ValidateCommandPath(m.Command) != nil {
				t.Fatalf("accepted an unsafe manifest: %+v", m)
			}
			for _, p := range m.Provides {
				if strings.HasPrefix(p.ID, "core.") {
					t.Fatal("accepted core namespace")
				}
			}
		}
	})
}
