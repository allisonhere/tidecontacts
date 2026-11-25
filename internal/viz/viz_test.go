package viz

import (
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestSparkline(t *testing.T) {
	if got := Sparkline(Values(0, 1, 2, 3, 4, 5, 6, 7), 8, Style{}); got != "▁▂▃▄▅▆▇█" {
		t.Fatalf("%q", got)
	}
	// Zero-based: 90..100 is nearly flat, not a cliff.
	if got := Sparkline(Values(90, 100), 2, Style{}); got != "▇█" {
		t.Fatalf("%q", got)
	}
	if got := Sparkline([]Value{{Missing: true}, {V: 0}, {V: 4}}, 5, Style{}); got != "·▁█  " {
		t.Fatalf("missing is not zero: %q", got)
	}
	if got := Sparkline(Values(1, 2, 3, 9), 2, Style{}); got != "▃█" {
		t.Fatalf("newest kept: %q", got)
	}
	if got := Sparkline(Values(0, 0), 2, Style{}); got != "▁▁" {
		t.Fatalf("all zero: %q", got)
	}
	if got := Sparkline(Values(0, 7), 2, Style{ASCII: true}); got != "_#" {
		t.Fatalf("%q", got)
	}
}

func TestBarAndMeter(t *testing.T) {
	for _, c := range []struct {
		v, max int64
		w      int
		want   string
	}{{10, 10, 4, "████"}, {5, 10, 4, "██  "}, {1, 1000000, 4, "▏   "}, {0, 10, 4, "    "}, {3, 16, 2, "▍ "}, {20, 10, 3, "███"}, {-5, 10, 4, "▒▒  "}} {
		if got := Bar(Value{V: c.v}, c.max, c.w, Style{}); got != c.want {
			t.Errorf("bar %d/%d: %q want %q", c.v, c.max, got, c.want)
		}
	}
	if got := Bar(Value{Missing: true}, 10, 3, Style{}); got != "·  " {
		t.Errorf("missing %q", got)
	}
	if got := Bar(Value{V: 1}, 100, 4, Style{ASCII: true}); got != "#   " {
		t.Errorf("ascii %q", got)
	}
	m := func(v, target int64, w int) string { a, b := Meter(v, target, w, Style{}); return a + "|" + b }
	for in, want := range map[[3]int64]string{{74, 100, 20}: "███████████████|░░░░░", {999, 1000, 10}: "█████████|░", {1000, 1000, 4}: "████|", {0, 10, 4}: "|░░░░", {1, 1000, 4}: "█|░░░", {5, 0, 3}: "|░░░"} {
		if got := m(in[0], in[1], int(in[2])); got != want {
			t.Errorf("meter %v: %q want %q", in, got, want)
		}
	}
	if got := Bullet(50, 100, 11, Style{}); got != "━━━━━━────┃" {
		t.Errorf("bullet %q", got)
	}
	if got := Bullet(100, 50, 5, Style{ASCII: true}); got != "==|==" {
		t.Errorf("bullet over %q", got)
	}
}

func TestColumnsStackHeatAndLine(t *testing.T) {
	got := Columns([]Value{{V: 8}, {V: 4}, {V: 0}, {Missing: true}}, 7, 2, Style{})
	if want := []string{"█      ", "█ █ ▁ ·"}; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%q", got)
	}
	if w := Stack([]int64{42, 21, 15, 10, 12}, 20); sum(w) != 20 || w[0] < w[1] {
		t.Fatal(w)
	}
	if w := Stack([]int64{1000000, 1, 1}, 10); sum(w) != 10 || w[1] != 1 || w[2] != 1 {
		t.Fatal(w)
	}
	if w := Stack([]int64{1, 1, 1, 1}, 2); sum(w) != 2 {
		t.Fatal(w)
	}
	if w := Stack([]int64{0, -4}, 10); sum(w) != 0 {
		t.Fatal(w)
	}
	if Heat(Value{V: 0}, 5, Style{}) != " " || Heat(Value{V: 1}, 100, Style{}) != "░" || Heat(Value{V: 100}, 100, Style{}) != "█" || Heat(Value{Missing: true}, 1, Style{}) != "·" {
		t.Fatal("heat")
	}
	l := Line(Values(0, 10), 4, 2, Style{})
	if len(l) != 2 || l[0] == "    " || l[1] == "    " {
		t.Fatalf("%q", l)
	}
	if a := Line(Values(0, 10), 2, 2, Style{ASCII: true}); a[0] != " *" || a[1] != "* " {
		t.Fatalf("%q", a)
	}
	gap := Line([]Value{{V: 5}, {Missing: true}, {V: 5}}, 3, 1, Style{ASCII: true})
	if gap[0] != "* *" {
		t.Fatalf("a gap must stay a gap: %q", gap)
	}
}

func sum(v []int) (n int) {
	for _, x := range v {
		n += x
	}
	return
}

// Every chart is exactly as wide and tall as asked, for any input.
func TestChartsNeverExceedTheirBox(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	extreme := []int64{math.MaxInt64, math.MinInt64 + 1, 0, 1, -1}
	for i := 0; i < 4000; i++ {
		w, h := r.Intn(60), r.Intn(8)
		v := make([]Value, r.Intn(90))
		for j := range v {
			v[j] = Value{V: r.Int63n(1_000_000) - 100_000, Missing: r.Intn(6) == 0}
			if r.Intn(20) == 0 {
				v[j].V = extreme[r.Intn(len(extreme))]
			}
		}
		s := Style{ASCII: r.Intn(2) == 0}
		check := func(name string, lines []string, rows int) {
			t.Helper()
			if w > 0 && h >= 0 && len(lines) != rows {
				t.Fatalf("%s: %d rows, want %d", name, len(lines), rows)
			}
			for _, l := range lines {
				if got := lipgloss.Width(l); got != w {
					t.Fatalf("%s %dx%d: width %d %q", name, w, h, got, l)
				}
				if s.ASCII {
					for _, c := range l {
						if c > 127 {
							t.Fatalf("%s: non-ASCII %q", name, l)
						}
					}
				}
			}
		}
		var one Value
		if len(v) > 0 {
			one = v[0]
		}
		if w > 0 {
			check("spark", []string{Sparkline(v, w, s)}, 1)
			check("bar", []string{Bar(one, one.V/2+1, w, s)}, 1)
			a, b := Meter(one.V, 1000, w, s)
			check("meter", []string{a + b}, 1)
			check("bullet", []string{Bullet(one.V, r.Int63n(1000), w, s)}, 1)
		}
		if w > 0 && h > 0 {
			check("columns", Columns(v, w, h, s), h)
			check("line", Line(v, w, h, s), h)
		}
	}
}
