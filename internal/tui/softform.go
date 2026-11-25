package tui

import (
	"strings"

	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// softUI renders form content for TideUI soft panels in the Tide-family modal
// style: a focus rail, a quiet label column, filled input fields, ‹› pickers, one
// status line and lowercase hints. Colours derive from the modal surface, so the
// same code is correct on every theme; without colour the rail and chevrons still
// mark focus.
type softUI struct {
	r                                                 tideui.Renderer
	base, surface, field, accent, text, muted, danger lipgloss.Color
}

func newSoftUI(r tideui.Renderer) softUI {
	t := r.Styles.Theme
	base := t.Overlay
	if base == "" {
		step := 0.06
		if !tideui.IsDark(t.Bg) {
			step = -step
		}
		base = tideui.AdjustLightness(t.Bg, step)
	}
	sign := 1.0
	if !tideui.IsDark(base) {
		sign = -1
	}
	accent := t.BorderFocus
	if accent == "" {
		accent = t.Border
	}
	text := tideui.ReadableText(t.Fg, base, 4.5)
	return softUI{r: r, base: base, surface: tideui.AdjustLightness(base, sign*0.04), field: tideui.AdjustLightness(base, sign*0.09),
		accent: accent, text: text, muted: tideui.MutedText(text, base), danger: t.ErrorColor()}
}

func (u softUI) style(bg, fg lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Background(bg).Foreground(fg)
}

// cell clips s to width and fills the remainder with bg.
func (u softUI) cell(s string, width int, bg lipgloss.Color) string {
	width = max(0, width)
	s = tideui.ContinuousBackground(ansi.Truncate(s, width, ""), bg)
	if gap := width - lipgloss.Width(s); gap > 0 {
		s += lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", gap))
	}
	return s
}

func (u softUI) line(s string, width int) string { return u.cell(s, width, u.base) }

func (u softUI) blank(width int) string { return u.line("", width) }

// input renders a single-line text field as a filled cell.
func (u softUI) input(in textinput.Model, width int, focused bool) string {
	bg := u.surface
	if focused {
		bg = u.field
	}
	in.Width = max(1, width-3)
	in.PromptStyle = u.style(bg, u.accent)
	in.TextStyle = u.style(bg, u.text)
	in.PlaceholderStyle = u.style(bg, u.muted)
	in.Cursor.TextStyle = u.style(bg, u.text)
	if focused {
		in.Cursor.Style = u.style(u.accent, tideui.ReadableText(u.base, u.accent, 4.5))
	}
	return u.cell(u.style(bg, u.text).Render(" ")+in.View(), width, bg)
}

// choice renders a value cycled with Left/Right, chevrons pinned right.
func (u softUI) choice(value string, width int, focused bool) string {
	bg, fg, mark := u.surface, u.muted, u.muted
	if focused {
		bg, fg, mark = u.field, u.text, u.accent
	}
	chevrons := "‹›"
	if u.r.Styles.PlainUI {
		chevrons = "<>"
	}
	left := u.cell(u.style(bg, fg).Render(" "+value), max(1, width-4), bg)
	return left + u.style(bg, mark).Bold(focused).Render(" "+chevrons+" ")
}

// preview renders read-only text in a field cell (an unfocused multi-line value).
func (u softUI) preview(text string, width int) string {
	return u.cell(u.style(u.surface, u.muted).Render(" "+text), width, u.surface)
}

// row lays out rail, label and control on one line.
func (u softUI) row(label string, focused bool, control string, width, labelW int) string {
	fg := u.muted
	if focused {
		fg = u.text
	}
	rail := u.r.SoftRail(focused, u.base)
	labelW = min(labelW, max(1, width-6))
	cell := u.cell(u.style(u.base, fg).Bold(focused).Render(ansi.Truncate(label, max(1, labelW-1), "…")), labelW, u.base)
	return u.line(rail+cell+control, width)
}

// controlWidth is the space a control gets in a row of the given width.
func (u softUI) controlWidth(width, labelW int) int {
	return max(4, width-2-min(labelW, max(1, width-6))-2)
}

// note is a single quiet line (help), or an error line when isErr.
func (u softUI) note(text string, width int, isErr bool) string {
	fg := u.muted
	if isErr {
		fg, text = u.danger, "! "+text
	}
	return u.line(u.style(u.base, fg).Render("  "+ansi.Truncate(text, max(1, width-4), "…")), width)
}

// tabs renders a section strip with the current section accented and marked.
func (u softUI) tabs(names []string, current, width int, focused bool) []string {
	var lines []string
	line := ""
	for i, name := range names {
		label := strings.ToLower(name)
		style := u.style(u.base, u.muted)
		if i == current {
			style = u.style(u.base, u.accent).Bold(true).Underline(true)
			if focused {
				label = "[" + label + "]"
			}
		}
		tab := style.Render(label)
		if line != "" && lipgloss.Width(line)+2+lipgloss.Width(tab) > width {
			lines = append(lines, u.line(line, width))
			line = ""
		}
		if line != "" {
			line += "  "
		}
		line += tab
	}
	return append(lines, u.line(line, width))
}

// hints renders the footer. When it does not fit, pairs are dropped from the
// middle; the first (the main action) and the last (cancel) always stay.
func (u softUI) hints(width int, pairs ...string) string {
	hints := []tideui.SoftHint{}
	for i := 0; i+1 < len(pairs); i += 2 {
		hints = append(hints, tideui.SoftHint{Key: pairs[i], Label: pairs[i+1]})
	}
	for len(hints) > 2 {
		line := u.r.RenderSoftHints(1000, hints...)
		if lipgloss.Width(strings.TrimRight(ansi.Strip(line), " ")) <= width-1 {
			break
		}
		hints = append(hints[:len(hints)-2], hints[len(hints)-1])
	}
	return u.r.RenderSoftHints(width, hints...)
}

// rows keeps input fields single-spaced and windows around focus when needed.
func (u softUI) rows(rows []string, focused, width, height int) []string {
	start, end := tideui.VisibleRange(len(rows), focused, max(1, height))
	return rows[start:end]
}

// panel wraps body lines in the soft modal box with a lowercase title.
func (u softUI) panel(title string, width int, body []string) *tideui.Overlay {
	lines := append([]string{u.blank(width)}, body...)
	o := u.r.SoftPanelOverlay(tideui.SoftPanel{Prefix: "tidecontacts", Title: title, Width: width, Content: strings.Join(lines, "\n")})
	return &o
}

// formSize returns the modal's inner width and the lines available for its body.
// Forms are overlays: the screen behind them stays visible.
func (m Model) formSize(want int) (width, height int) {
	return max(1, min(want, m.width-6)), max(1, m.height-6)
}

// recordFormSize keeps creation and edit forms steady across record types and
// sections, while leaving room for the surrounding screen on short terminals.
func (m Model) recordFormSize(want int) (width, height int) {
	width, height = m.formSize(want)
	return width, min(24, height)
}

// fixedFormPanel pads beneath the fields so the footer stays at the bottom.
func (u softUI) fixedFormPanel(title string, width, height int, body, footer []string) *tideui.Overlay {
	height = max(1, height)
	if len(footer) > height {
		footer = footer[len(footer)-height:]
	}
	available := height - len(footer)
	lines := append([]string(nil), body[:min(len(body), available)]...)
	for len(lines) < available {
		lines = append(lines, u.blank(width))
	}
	lines = append(lines, footer...)
	return u.panel(title, width, lines)
}
