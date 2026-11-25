// Package viz draws small charts with terminal character cells. Every function
// returns exactly the number of cells asked for, never emits colour or control
// sequences (the caller applies theme styles), distinguishes missing data from
// zero, and has an ASCII-only form for terminals without block glyphs.
//
// Values are integers (money in minor units, counts, basis points). They are
// converted to floating point only to decide which cells to fill; the numbers
// shown to the user are formatted by whoever owns the data.
package viz

import (
	"math"
	"strings"
)

// Value is one sample. Missing means "no data", which is not zero.
type Value struct {
	V       int64
	Missing bool
}

// Values wraps plain numbers.
func Values(v ...int64) []Value {
	out := make([]Value, len(v))
	for i, x := range v {
		out[i] = Value{V: x}
	}
	return out
}

// Style chooses the glyph set.
type Style struct{ ASCII bool }

// MissingGlyph marks absent data in every chart.
const MissingGlyph = "·"

func (s Style) missing() string {
	if s.ASCII {
		return "?"
	}
	return MissingGlyph
}

var (
	rampUnicode = []rune("▁▂▃▄▅▆▇█")
	rampASCII   = []rune("_.:-=+*#")
	eighths     = []rune(" ▏▎▍▌▋▊▉█")
)

func (s Style) ramp() []rune {
	if s.ASCII {
		return rampASCII
	}
	return rampUnicode
}

// bounds returns the scale of a series. The scale always includes zero, so bar
// heights are proportional to the values and small differences are not blown up.
func bounds(v []Value) (lo, hi float64, any bool) {
	for _, x := range v {
		if x.Missing {
			continue
		}
		any = true
		lo, hi = math.Min(lo, float64(x.V)), math.Max(hi, float64(x.V))
	}
	return
}

// level maps a value to 0..n-1 on a zero-based scale.
func level(v, lo, hi float64, n int) int {
	if hi <= lo {
		return 0
	}
	i := int(math.Round((v - lo) / (hi - lo) * float64(n-1)))
	return min(n-1, max(0, i))
}

// fit reduces or keeps a series so it has at most width samples, keeping the
// most recent ones.
func fit(v []Value, width int) []Value {
	if len(v) > width {
		return v[len(v)-width:]
	}
	return v
}

// Sparkline draws one cell per sample, newest on the right. When there are more
// samples than cells the oldest are dropped; with fewer, the line is padded
// with spaces.
func Sparkline(values []Value, width int, s Style) string {
	if width <= 0 {
		return ""
	}
	v := fit(values, width)
	lo, hi, _ := bounds(v)
	ramp := s.ramp()
	var b strings.Builder
	for _, x := range v {
		if x.Missing {
			b.WriteString(s.missing())
			continue
		}
		b.WriteRune(ramp[level(float64(x.V), lo, hi, len(ramp))])
	}
	return b.String() + strings.Repeat(" ", width-len(v))
}

// cells returns how many eighths of `width` cells the magnitude |v|/max fills.
// A non-zero value always shows at least one eighth.
func cells(v, maximum int64, width int) int {
	if v == 0 || maximum <= 0 || width <= 0 {
		return 0
	}
	f := math.Abs(float64(v)) / float64(maximum)
	n := int(math.Round(math.Min(1, f) * float64(width*8)))
	return max(1, n)
}

// Bar draws a horizontal bar of |v| on the scale 0..maximum, padded to width.
// A negative value is drawn with a different glyph; a missing one with dots.
func Bar(v Value, maximum int64, width int, s Style) string {
	if width <= 0 {
		return ""
	}
	if v.Missing {
		return strings.Repeat(s.missing(), 1) + strings.Repeat(" ", width-1)
	}
	n := cells(v.V, maximum, width)
	full, part := n/8, n%8
	var b string
	switch {
	case v.V < 0:
		c := max(full, min(1, n))
		glyph := "▒"
		if s.ASCII {
			glyph = "-"
		}
		return strings.Repeat(glyph, c) + strings.Repeat(" ", width-c)
	case s.ASCII:
		c := (n + 4) / 8
		if n > 0 && c == 0 {
			c = 1
		}
		return strings.Repeat("#", c) + strings.Repeat(" ", width-c)
	default:
		b = strings.Repeat("█", full)
		if part > 0 {
			b += string(eighths[part])
			full++
		}
		return b + strings.Repeat(" ", width-full)
	}
}

// Meter splits a progress indicator into its filled and unfilled parts, which
// together are exactly width cells. Progress above the target fills the meter;
// the number shown beside it says by how much. Completion short of the target
// never rounds up to a full meter.
func Meter(v, target int64, width int, s Style) (filled, rest string) {
	if width <= 0 {
		return "", ""
	}
	on, off := "█", "░"
	if s.ASCII {
		on, off = "#", "-"
	}
	n := 0
	switch {
	case target <= 0 || v <= 0:
	case v >= target:
		n = width
	default:
		n = min(width-1, max(1, int(math.Round(float64(v)/float64(target)*float64(width)))))
	}
	return strings.Repeat(on, n), strings.Repeat(off, width-n)
}

// Bullet draws a measure against a target: a bar for the actual value, a mark
// where the target lies, on a scale ending at the larger of the two.
func Bullet(actual, target int64, width int, s Style) string {
	if width <= 0 {
		return ""
	}
	maximum := max(actual, target, 1)
	on, off, mark := '━', '─', '┃'
	if s.ASCII {
		on, off, mark = '=', '-', '|'
	}
	out := []rune(strings.Repeat(string(off), width))
	n := (cells(max(0, actual), maximum, width) + 4) / 8
	for i := 0; i < min(n, width); i++ {
		out[i] = on
	}
	if target > 0 {
		at := int(math.Round(float64(target) / float64(maximum) * float64(width-1)))
		out[min(width-1, max(0, at))] = mark
	}
	return string(out)
}

// Columns draws vertical bars, top row first. Each sample gets the same bar
// width with a one-cell gap where room allows; surplus samples drop from the
// left. Every row is exactly width cells.
func Columns(values []Value, width, height int, s Style) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	gap := 1
	if len(values)*2-1 > width {
		gap = 0
	}
	v := fit(values, width)
	if len(v) == 0 {
		out := make([]string, height)
		for i := range out {
			out[i] = strings.Repeat(" ", width)
		}
		return out
	}
	bw := max(1, (width-gap*(len(v)-1))/len(v))
	lo, hi, _ := bounds(v)
	lo = math.Min(lo, 0)
	rows := make([]strings.Builder, height)
	used := 0
	for i, x := range v {
		units := 0
		if !x.Missing && hi > lo && x.V != 0 {
			units = max(1, int(math.Round(math.Abs(float64(x.V))/math.Max(hi, -lo)*float64(height*8))))
		}
		for r := 0; r < height; r++ {
			fromBottom := height - 1 - r
			glyph := " "
			switch {
			case x.Missing:
				if fromBottom == 0 {
					glyph = s.missing()
				}
			case x.V == 0:
				if fromBottom == 0 {
					glyph = string(s.ramp()[0])
				}
			default:
				fill := units - fromBottom*8
				if fill > 0 {
					switch {
					case x.V < 0:
						glyph = "▒"
						if s.ASCII {
							glyph = "-"
						}
					case s.ASCII:
						if fill >= 4 || fromBottom == 0 {
							glyph = "#"
						}
					default:
						glyph = string(rampUnicode[min(8, fill)-1])
					}
				}
			}
			rows[r].WriteString(strings.Repeat(glyph, bw))
			if i < len(v)-1 {
				rows[r].WriteString(strings.Repeat(" ", gap))
			}
		}
		used += bw
		if i < len(v)-1 {
			used += gap
		}
	}
	out := make([]string, height)
	for r := range rows {
		out[r] = rows[r].String() + strings.Repeat(" ", max(0, width-used))
	}
	return out
}

// ColumnWidth reports the bar width and gap Columns will use, for axis labels.
func ColumnWidth(n, width int) (bar, gap int) {
	if n <= 0 || width <= 0 {
		return 0, 0
	}
	gap = 1
	if n*2-1 > width {
		gap = 0
	}
	n = min(n, width)
	return max(1, (width-gap*(n-1))/n), gap
}

// Stack divides width cells among parts in proportion to their values (largest
// remainder), giving every positive part at least one cell where there is room.
func Stack(values []int64, width int) []int {
	out := make([]int, len(values))
	var total float64
	for _, v := range values {
		if v > 0 {
			total += float64(v)
		}
	}
	if total == 0 || width <= 0 {
		return out
	}
	used := 0
	rem := make([]float64, len(values))
	for i, v := range values {
		if v <= 0 {
			continue
		}
		exact := float64(v) / total * float64(width)
		out[i], rem[i] = int(exact), exact-math.Floor(exact)
		if out[i] == 0 {
			out[i], rem[i] = 1, -1
		}
		used += out[i]
	}
	for used < width {
		best := -1
		for i := range values {
			if values[i] > 0 && (best < 0 || rem[i] > rem[best]) {
				best = i
			}
		}
		out[best]++
		rem[best] -= 1
		used++
	}
	for used > width {
		big := 0
		for i := range out {
			if out[i] > out[big] {
				big = i
			}
		}
		out[big]--
		used--
	}
	return out
}

// Heat returns the shade cell for a value on 0..maximum.
func Heat(v Value, maximum int64, s Style) string {
	if v.Missing {
		return s.missing()
	}
	ramp := []rune(" ░▒▓█")
	if s.ASCII {
		ramp = []rune(" .:*#")
	}
	if v.V <= 0 || maximum <= 0 {
		return string(ramp[0])
	}
	i := int(math.Ceil(float64(v.V) / float64(maximum) * float64(len(ramp)-1)))
	return string(ramp[min(len(ramp)-1, max(1, i))])
}

// Line draws a line chart, top row first. With Unicode it uses braille cells
// (2×4 dots each); in ASCII it plots one mark per column. Gaps in the data are
// gaps in the line. The scale includes zero.
func Line(values []Value, width, height int, s Style) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	px, py := width*2, height*4
	if s.ASCII {
		px, py = width, height
	}
	dots := make([][]bool, py)
	for i := range dots {
		dots[i] = make([]bool, px)
	}
	v := fit(values, px)
	lo, hi, any := bounds(v)
	if any && len(v) > 0 {
		x := func(i int) int {
			if len(v) == 1 {
				return 0
			}
			return i * (px - 1) / (len(v) - 1)
		}
		y := func(val int64) int { return py - 1 - level(float64(val), lo, hi, py) }
		for i, a := range v {
			if a.Missing {
				continue
			}
			x0, y0 := x(i), y(a.V)
			dots[y0][x0] = true
			if i+1 < len(v) && !v[i+1].Missing {
				x1, y1 := x(i+1), y(v[i+1].V)
				steps := max(abs(x1-x0), abs(y1-y0))
				for k := 1; k < steps; k++ {
					dots[y0+int(math.Round(float64((y1-y0)*k)/float64(steps)))][x0+int(math.Round(float64((x1-x0)*k)/float64(steps)))] = true
				}
			}
		}
	}
	out := make([]string, height)
	for r := 0; r < height; r++ {
		var b strings.Builder
		for c := 0; c < width; c++ {
			if s.ASCII {
				if dots[r][c] {
					b.WriteByte('*')
				} else {
					b.WriteByte(' ')
				}
				continue
			}
			bits := rune(0)
			for dy, row := range [4][2]rune{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}} {
				for dx := 0; dx < 2; dx++ {
					if dots[r*4+dy][c*2+dx] {
						bits |= row[dx]
					}
				}
			}
			if bits == 0 {
				b.WriteByte(' ')
			} else {
				b.WriteRune(0x2800 + bits)
			}
		}
		out[r] = b.String()
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
