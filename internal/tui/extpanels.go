package tui

import (
	"fmt"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/ext"
	"github.com/allisonhere/tidecontacts/internal/viz"
	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Panel views: the host arranges an extension's panels in one, two or three
// columns, owns focus and selection, and draws every chart in character cells.

// Width thresholds (modal content cells) for the number of panel columns.
const (
	twoColumnsFrom   = 96
	threeColumnsFrom = 144
	minPanelRows     = 6
)

func panelColumns(width int) int {
	switch {
	case width >= threeColumnsFrom:
		return 3
	case width >= twoColumnsFrom:
		return 2
	}
	return 1
}

// panelItems lists the selectable items of a panel, in drawing order.
func panelItems(p ext.Panel) []extItem {
	var out []extItem
	add := func(id string, t *ext.Target) {
		if id == "" && t != nil {
			id = t.ID
		}
		if id != "" {
			out = append(out, extItem{id: id, target: t})
		}
	}
	for _, b := range p.Blocks {
		switch b.Kind {
		case "lines":
			for _, l := range b.Lines {
				add(l.ID, l.Target)
			}
		case "bars", "columns", "stack", "spark", "trend", "progress", "bullet":
			for _, d := range b.Data {
				add(d.ID, d.Target)
			}
		}
	}
	return out
}

// focusedPanel is the index of the panel with focus.
func (v *extView) focusedPanel() int {
	for i, p := range v.doc.Panels {
		if p.ID == v.cur.panel {
			return i
		}
	}
	return 0
}

// focusPanel moves focus by d, remembering each panel's selection.
func (v *extView) focusPanel(d int) {
	n := len(v.doc.Panels)
	if n == 0 {
		return
	}
	i := v.focusedPanel()
	v.cur.remember()
	i = (i + d + n) % n
	v.cur.panel = v.doc.Panels[i].ID
	v.cur.row, v.cur.selID = v.cur.psel[v.cur.panel], ""
	v.settlePanels()
}

func (f *extFrame) remember() {
	if f.panel == "" {
		return
	}
	next := map[string]int{}
	for k, val := range f.psel {
		next[k] = val
	}
	next[f.panel] = f.row
	f.psel = next
}

// settlePanels keeps focus on an existing panel and the selection on the same
// record when a refreshed document still holds it.
func (v *extView) settlePanels() {
	if v.cur.panel == "" || v.doc.Panels[v.focusedPanel()].ID != v.cur.panel {
		v.cur.panel = v.doc.Panels[0].ID
		for _, p := range v.doc.Panels {
			if len(panelItems(p)) > 0 {
				v.cur.panel = p.ID
				break
			}
		}
		v.cur.row = v.cur.psel[v.cur.panel]
	}
	items := panelItems(v.doc.Panels[v.focusedPanel()])
	if v.cur.selID != "" && !(v.cur.row >= 0 && v.cur.row < len(items) && items[v.cur.row].id == v.cur.selID) {
		for i, it := range items {
			if it.id == v.cur.selID {
				v.cur.row = i
				break
			}
		}
	}
	v.cur.row = min(max(0, v.cur.row), max(0, len(items)-1))
}

// pline is a drawn panel row; item is the selectable item it shows, or -1.
type pline struct {
	s    string
	item int
}

type panelDraw struct {
	u     softUI
	w     int
	style viz.Style
	sel   int // selected item, or -1 when the panel is not focused
	next  int
	out   []pline
}

func (d *panelDraw) paint(tone, text string) string {
	return d.u.style(d.u.base, d.u.tone(tone)).Render(text)
}
func (d *panelDraw) muted(text string) string { return d.paint("muted", text) }
func (d *panelDraw) add(s string, item int) {
	d.out = append(d.out, pline{d.u.cell(s, d.w, d.u.base), item})
}
func (d *panelDraw) plain(s string) { d.add(s, -1) }
func (d *panelDraw) claim(selectable bool) int {
	if !selectable {
		return -1
	}
	d.next++
	return d.next - 1
}

// row draws a selectable row: a marker column plus content, highlighted when selected.
func (d *panelDraw) row(content string, item int) {
	if item >= 0 && item == d.sel {
		mark := "›"
		if d.style.ASCII {
			mark = ">"
		}
		bg := tideui.MixColors(d.u.base, d.u.accent, .22)
		s := d.u.style(bg, d.u.accent).Bold(true).Render(mark) + tideui.ContinuousBackground(content, bg)
		d.out = append(d.out, pline{d.u.cell(s, d.w, bg), item})
		return
	}
	d.add(" "+content, item)
}

func pad(s string, w int) string {
	s = ansi.Truncate(s, max(0, w), "…")
	return s + strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
}

func lpad(s string, w int) string {
	s = ansi.Truncate(s, max(0, w), "…")
	return strings.Repeat(" ", max(0, w-lipgloss.Width(s))) + s
}

func selectable(id string, t *ext.Target) bool { return id != "" || t != nil }

func widest(n int, f func(int) string) (w int) {
	for i := 0; i < n; i++ {
		w = max(w, lipgloss.Width(f(i)))
	}
	return
}

func (d *panelDraw) lines(b ext.Block) {
	n := len(b.Lines)
	room := d.w - 1
	right := min(widest(n, func(i int) string { return b.Lines[i].Right }), room/3)
	label := min(widest(n, func(i int) string { return b.Lines[i].Label }), max(8, room*2/5))
	mark := min(widest(n, func(i int) string { return b.Lines[i].Mark }), 2)
	for _, l := range b.Lines {
		item := d.claim(selectable(l.ID, l.Target))
		s, used := "", 0
		if mark > 0 {
			s, used = d.paint(l.Tone, pad(l.Mark, mark))+" ", mark+1
		}
		rest := room - used
		if right > 0 {
			rest -= right + 1
		}
		if l.Label == "" {
			s += d.paint(l.Tone, pad(l.Text, rest))
		} else {
			lw := min(label, rest)
			s += d.u.style(d.u.base, d.u.text).Render(pad(l.Label, lw))
			if tw := rest - lw - 2; tw >= 6 {
				tone := l.Tone
				if tone == "" {
					tone = "muted"
				}
				s += "  " + d.paint(tone, pad(l.Text, tw))
			} else {
				s += strings.Repeat(" ", max(0, rest-lw))
			}
		}
		if right > 0 {
			s += " " + d.u.style(d.u.base, d.u.text).Render(lpad(l.Right, right))
		}
		d.row(s, item)
	}
}

func scaleOf(data []ext.Datum) (m int64) {
	for _, x := range data {
		v := x.Value
		if v < 0 {
			v = -v
		}
		if !x.Missing {
			m = max(m, v)
		}
	}
	return
}

func vizValues(data []ext.Datum) []viz.Value {
	out := make([]viz.Value, len(data))
	for i, x := range data {
		out[i] = viz.Value{V: x.Value, Missing: x.Missing}
	}
	return out
}

func datumText(x ext.Datum) string {
	if x.Text == "" && x.Missing {
		return "no data"
	}
	return x.Text
}

func (d *panelDraw) bars(b ext.Block) {
	n := len(b.Data)
	room := d.w - 1
	text := min(widest(n, func(i int) string { return datumText(b.Data[i]) }), room/2)
	label := min(widest(n, func(i int) string { return b.Data[i].Label }), max(6, room/3))
	bar := room - label - text - 2
	if bar < 4 { // too narrow for a bar: keep the label and the figure
		bar, label = 0, max(1, room-text-1)
	}
	scale := scaleOf(b.Data)
	for _, x := range b.Data {
		item := d.claim(selectable(x.ID, x.Target))
		s := d.u.style(d.u.base, d.u.text).Render(pad(x.Label, label)) + " "
		if bar > 0 {
			tone := x.Tone
			if tone == "" {
				tone = "accent"
			}
			if x.Missing {
				tone = "muted"
			}
			s += d.paint(tone, viz.Bar(viz.Value{V: x.Value, Missing: x.Missing}, scale, bar, d.style)) + " "
		}
		fg := d.u.text
		if x.Missing {
			fg = d.u.muted
		}
		s += d.u.style(d.u.base, fg).Render(lpad(datumText(x), text))
		d.row(s, item)
	}
}

// detail names the selected sample of a chart whose cells carry no figures.
func (d *panelDraw) detail(b ext.Block, first int) {
	i := len(b.Data) - 1
	picked := -1
	if d.sel >= first && d.sel < d.next {
		n := first
		for j, x := range b.Data {
			if selectable(x.ID, x.Target) {
				if n == d.sel {
					i, picked = j, d.sel
				}
				n++
			}
		}
	}
	if i < 0 {
		return
	}
	x := b.Data[i]
	d.row(d.muted(x.Label+"  ")+d.u.style(d.u.base, d.u.text).Bold(true).Render(datumText(x)), picked)
}

func (d *panelDraw) claimAll(data []ext.Datum) (first int) {
	first = d.next
	for _, x := range data {
		d.claim(selectable(x.ID, x.Target))
	}
	return
}

func (d *panelDraw) columns(b ext.Block, height int) {
	if len(b.Data) == 0 {
		return
	}
	first := d.claimAll(b.Data)
	room := d.w - 2
	bw, gap := viz.ColumnWidth(len(b.Data), room)
	// Widen narrow series only so far: very wide columns read as blocks.
	if bw > 7 {
		room = len(b.Data)*7 + (len(b.Data)-1)*gap
		bw = 7
	}
	selected := -1
	if d.sel >= first && d.sel < d.next {
		selected = d.sel - first
	}
	shown := b.Data
	if len(shown) > room {
		shown = shown[len(shown)-room:]
	}
	tone := b.Tone
	if tone == "" {
		tone = "accent"
	}
	for _, r := range viz.Columns(vizValues(shown), room, height, d.style) {
		d.add(" "+d.paint(tone, r), -1)
	}
	axis := ""
	if bw >= 3 {
		for i, x := range shown {
			l := pad(ansi.Truncate(x.Label, bw, ""), bw)
			if i == selected-(len(b.Data)-len(shown)) {
				axis += d.u.style(d.u.base, d.u.accent).Bold(true).Underline(true).Render(l)
			} else {
				axis += d.muted(l)
			}
			if i < len(shown)-1 {
				axis += strings.Repeat(" ", gap)
			}
		}
	} else {
		a, z := shown[0].Label, shown[len(shown)-1].Label
		axis = d.muted(a + strings.Repeat(" ", max(1, room-lipgloss.Width(a)-lipgloss.Width(z))) + z)
	}
	d.add(" "+axis, -1)
	d.detail(b, first)
}

func (d *panelDraw) series(b ext.Block, height int) {
	if len(b.Data) == 0 {
		return
	}
	first := d.claimAll(b.Data)
	tone := b.Tone
	if tone == "" {
		tone = "info"
	}
	if b.Kind == "spark" {
		label := b.Text
		w := min(len(b.Data), max(1, d.w-2-lipgloss.Width(label)))
		s := ""
		if label != "" {
			s = d.muted(label + " ")
		}
		d.add(" "+s+d.paint(tone, viz.Sparkline(vizValues(b.Data), w, d.style)), -1)
	} else {
		room := min(d.w-2, max(24, len(b.Data)*6))
		for _, r := range viz.Line(vizValues(b.Data), room, height, d.style) {
			d.add(" "+d.paint(tone, r), -1)
		}
		a, z := b.Data[0].Label, b.Data[len(b.Data)-1].Label
		d.add(" "+d.muted(a+strings.Repeat(" ", max(1, room-lipgloss.Width(a)-lipgloss.Width(z)))+z), -1)
	}
	d.detail(b, first)
}

func (d *panelDraw) progress(b ext.Block) {
	if len(b.Data) == 0 {
		return
	}
	x := b.Data[0]
	item := d.claim(selectable(x.ID, x.Target))
	if b.Text != "" {
		d.row(d.u.style(d.u.base, d.u.text).Bold(true).Render(b.Text), item)
		item = -1
	}
	fig := datumText(x)
	w := max(1, min(40, d.w-3-lipgloss.Width(fig)))
	tone := x.Tone
	if tone == "" {
		tone = "accent"
	}
	var bar string
	switch {
	case x.Missing:
		bar = d.muted(strings.Repeat(viz.MissingGlyph, w))
	case b.Kind == "bullet":
		bar = d.paint(tone, viz.Bullet(x.Value, b.Goal, w, d.style))
	default:
		on, off := viz.Meter(x.Value, b.Goal, w, d.style)
		bar = d.paint(tone, on) + d.muted(off)
	}
	d.row(bar+" "+d.u.style(d.u.base, d.u.text).Render(fig), item)
}

var stackGlyphs = []string{"█", "▓", "▒", "░", "▚"}
var stackASCII = []string{"#", "=", "+", ":", "."}
var stackTones = []string{"accent", "info", "ok", "warn", "muted"}

func (d *panelDraw) stack(b ext.Block) {
	values := make([]int64, len(b.Data))
	for i, x := range b.Data {
		if !x.Missing {
			values[i] = x.Value
		}
	}
	glyphs := stackGlyphs
	if d.style.ASCII {
		glyphs = stackASCII
	}
	w := max(1, d.w-2)
	bar := ""
	for i, n := range viz.Stack(values, w) {
		bar += d.paint(stackTones[i%len(stackTones)], strings.Repeat(glyphs[i%len(glyphs)], n))
	}
	d.add(" "+bar, -1)
	room := d.w - 1
	text := min(widest(len(b.Data), func(i int) string { return datumText(b.Data[i]) }), room*2/3)
	for i, x := range b.Data {
		item := d.claim(selectable(x.ID, x.Target))
		key := d.paint(stackTones[i%len(stackTones)], glyphs[i%len(glyphs)])
		d.row(key+" "+d.u.style(d.u.base, d.u.text).Render(pad(x.Label, max(1, room-text-3)))+" "+d.u.style(d.u.base, d.u.text).Render(lpad(datumText(x), text)), item)
	}
}

func (d *panelDraw) heat(b ext.Block) {
	var scale int64
	cols := len(b.Columns)
	for _, r := range b.Rows {
		cols = max(cols, len(r.Values))
		for _, v := range r.Values {
			if v != nil {
				scale = max(scale, *v)
			}
		}
	}
	if cols == 0 {
		return
	}
	label := min(widest(len(b.Rows), func(i int) string { return b.Rows[i].Label }), d.w/3)
	text := min(widest(len(b.Rows), func(i int) string { return b.Rows[i].Text }), d.w/4)
	cell := max(1, min(4, (d.w-2-label-text-2)/cols))
	head := strings.Repeat(" ", label+1)
	for i := 0; i < cols; i++ {
		h := ""
		if i < len(b.Columns) {
			h = b.Columns[i]
		}
		head += pad(ansi.Truncate(h, cell, ""), cell)
	}
	d.add(" "+d.muted(head), -1)
	for _, r := range b.Rows {
		s := d.u.style(d.u.base, d.u.text).Render(pad(r.Label, label)) + " "
		for i := 0; i < cols; i++ {
			v := viz.Value{Missing: true}
			if i < len(r.Values) && r.Values[i] != nil {
				v = viz.Value{V: *r.Values[i]}
			}
			s += d.paint("accent", strings.Repeat(viz.Heat(v, scale, d.style), max(1, cell-1))) + strings.Repeat(" ", min(1, cell-1))
		}
		d.add(" "+s+" "+d.muted(r.Text), -1)
	}
}

// drawPanel renders a whole panel body (without its title) at width w.
func (m Model) drawPanel(u softUI, p ext.Panel, w, sel int, compact bool) []pline {
	d := &panelDraw{u: u, w: w, sel: sel, style: viz.Style{ASCII: u.r.Styles.PlainUI}}
	for bi, b := range p.Blocks {
		if bi > 0 {
			d.plain("")
		}
		if b.Title != "" {
			d.add(" "+d.muted(b.Title), -1)
		}
		height := b.Height
		if height < 2 {
			height = 5
		}
		if compact {
			height = min(height, 4)
		}
		switch b.Kind {
		case "lines":
			d.lines(b)
		case "bars":
			d.bars(b)
		case "columns":
			d.columns(b, height)
		case "spark", "trend":
			d.series(b, height)
		case "progress", "bullet":
			d.progress(b)
		case "stack":
			d.stack(b)
		case "heat":
			d.heat(b)
		case "text":
			tone := b.Tone
			if tone == "" {
				tone = "muted"
			}
			for _, line := range strings.Split(ansi.Wrap(b.Text, max(1, w-2), ""), "\n") {
				d.add(" "+d.paint(tone, line), -1)
			}
		}
	}
	if len(d.out) == 0 {
		d.add(" "+d.muted(firstNonEmptyStr(p.Empty, "Nothing to show")), -1)
	}
	return d.out
}

// panelBox is a titled panel clipped to height rows, scrolled to the selection.
func (m Model) panelBox(u softUI, v *extView, i, w, height int, compact bool) []string {
	p := v.doc.Panels[i]
	focused := i == v.focusedPanel()
	sel := -1
	if focused && len(panelItems(p)) > 0 {
		sel = v.cur.row
	}
	body := m.drawPanel(u, p, w, sel, compact)
	mark, style := " ", u.style(u.base, u.text).Bold(true)
	if focused {
		mark, style = "▌", u.style(u.base, u.accent).Bold(true).Underline(true)
		if u.r.Styles.PlainUI {
			mark = "*"
		}
	}
	title := u.style(u.base, u.accent).Render(mark) + style.Render(ansi.Truncate(strings.ToUpper(p.Title), max(1, w-2-lipgloss.Width(p.Count)), "…"))
	if p.Count != "" {
		gap := max(1, w-lipgloss.Width(title)-lipgloss.Width(p.Count)-1)
		title += strings.Repeat(" ", gap) + u.style(u.base, u.muted).Render(p.Count)
	}
	out := []string{u.cell(title, w, u.base)}
	room := max(0, height-1)
	start := 0
	if len(body) > room {
		at := 0
		for n, l := range body {
			if sel >= 0 && l.item == sel {
				at = n
				break
			}
		}
		visible := max(1, room-1)
		start = min(max(0, at-visible/2), len(body)-visible)
		if at < visible {
			start = 0
		}
		for _, l := range body[start : start+visible] {
			out = append(out, l.s)
		}
		below := len(body) - start - visible
		note := fmt.Sprintf(" ↑ %d  ↓ %d more", start, below)
		if start == 0 {
			note = fmt.Sprintf(" ↓ %d more", below)
		}
		out = append(out, u.cell(u.style(u.base, u.muted).Render(note), w, u.base))
	} else {
		for _, l := range body {
			out = append(out, l.s)
		}
	}
	return out[:min(len(out), max(1, height))]
}

// panelRows groups panels into grid rows.
func panelRows(panels []ext.Panel, cols int) [][]int {
	var rows [][]int
	var row []int
	for i, p := range panels {
		if p.Full {
			if len(row) > 0 {
				rows, row = append(rows, row), nil
			}
			rows = append(rows, []int{i})
			continue
		}
		row = append(row, i)
		if len(row) == cols {
			rows, row = append(rows, row), nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

// share divides avail rows among rows wanting natural[i]: small rows get what
// they need, the others split the rest evenly.
func share(natural []int, avail int) []int {
	out := make([]int, len(natural))
	left, open := avail, len(natural)
	done := make([]bool, len(natural))
	for open > 0 {
		each := left / open
		progressed := false
		for i, n := range natural {
			if !done[i] && n <= each {
				out[i], done[i], progressed = n, true, true
				left -= n
				open--
			}
		}
		if !progressed {
			for i := range natural {
				if !done[i] {
					out[i] = left / open
					left -= out[i]
					open--
					done[i] = true
				}
			}
		}
	}
	return out
}

// extPanels draws the panel area into exactly the available height.
func (m Model) extPanels(u softUI, v *extView, width, avail int) []string {
	panels := v.doc.Panels
	inner := max(1, width-4)
	cols := panelColumns(width)
	focus := v.focusedPanel()
	var out []string
	emit := func(s string) { out = append(out, u.line("  "+s, width)) }
	if cols == 1 { // one panel at a time, with a strip naming the others
		if len(panels) > 1 {
			names := make([]string, len(panels))
			for i, p := range panels {
				names[i] = firstNonEmptyStr(p.Short, p.Title)
			}
			emit(u.strip(names, focus, inner))
		}
		for _, l := range m.panelBox(u, v, focus, inner, max(2, avail-len(out)), true) {
			emit(l)
		}
		return out
	}
	rows := panelRows(panels, cols)
	cellWidth := func(row []int) int { return (inner - 3*(len(row)-1)) / len(row) }
	natural := make([]int, len(rows))
	focusRow := 0
	for r, row := range rows {
		for _, i := range row {
			if i == focus {
				focusRow = r
			}
			natural[r] = max(natural[r], 1+len(m.drawPanel(u, panels[i], cellWidth(row), -1, false)))
		}
	}
	// Show as many rows as fit at their minimum height, including the focused one.
	first, last := focusRow, focusRow+1
	need := func(a, b int) (n int) {
		for r := a; r < b; r++ {
			n += min(natural[r], minPanelRows)
		}
		return n + (b - a - 1)
	}
	for last < len(rows) && need(first, last+1) <= avail {
		last++
	}
	for first > 0 && need(first-1, last) <= avail {
		first--
	}
	room := avail - (last - first - 1)
	if first > 0 || last < len(rows) {
		room--
	}
	heights := share(natural[first:last], max(1, room))
	sep := u.style(u.base, u.muted)
	bar, rule := " │ ", "─"
	if u.r.Styles.PlainUI {
		bar, rule = " | ", "-"
	}
	for r := first; r < last; r++ {
		if r > first {
			emit(sep.Render(strings.Repeat(rule, inner)))
		}
		row := rows[r]
		w := cellWidth(row)
		boxes := make([][]string, len(row))
		tall := 0
		for c, i := range row {
			boxes[c] = m.panelBox(u, v, i, w, heights[r-first], false)
			tall = max(tall, len(boxes[c]))
		}
		for line := 0; line < tall; line++ {
			parts := make([]string, len(row))
			for c := range row {
				if line < len(boxes[c]) {
					parts[c] = boxes[c][line]
				} else {
					parts[c] = u.blank(w)
				}
			}
			emit(strings.Join(parts, sep.Render(bar)))
		}
	}
	if first > 0 || last < len(rows) {
		hidden := 0
		for r, row := range rows {
			if r < first || r >= last {
				hidden += len(row)
			}
		}
		emit(sep.Render(fmt.Sprintf("%d more panel(s) · tab to reach them", hidden)))
	}
	if len(out) > avail {
		out = out[:avail]
	}
	return out
}

// strip is a one-line selector that scrolls to keep the current name visible.
func (u softUI) strip(names []string, current, width int) string {
	render := func(i int) string {
		label := strings.ToLower(names[i])
		if i == current {
			return u.style(u.base, u.accent).Bold(true).Underline(true).Render("[" + label + "]")
		}
		return u.style(u.base, u.muted).Render(label)
	}
	a, b := current, current+1
	size := lipgloss.Width(render(current))
	for {
		grew := false
		if b < len(names) && size+2+lipgloss.Width(render(b)) <= width-4 {
			size += 2 + lipgloss.Width(render(b))
			b++
			grew = true
		}
		if a > 0 && size+2+lipgloss.Width(render(a-1)) <= width-4 {
			size += 2 + lipgloss.Width(render(a-1))
			a--
			grew = true
		}
		if !grew {
			break
		}
	}
	var parts []string
	for i := a; i < b; i++ {
		parts = append(parts, render(i))
	}
	s := strings.Join(parts, "  ")
	if a > 0 {
		s = u.style(u.base, u.muted).Render("‹ ") + s
	}
	if b < len(names) {
		s += u.style(u.base, u.muted).Render(" ›")
	}
	return ansi.Truncate(s, max(1, width), "…")
}

// compactTiles packs headline figures into at most two lines.
func (m Model) compactTiles(u softUI, tiles []ext.Tile, width int) []string {
	var out []string
	line := ""
	for _, t := range tiles {
		s := u.style(u.base, u.muted).Render(t.Label+" ") + u.style(u.base, u.tone(t.Tone)).Bold(true).Render(t.Value)
		if line != "" && lipgloss.Width(line)+3+lipgloss.Width(s) > width-4 {
			out = append(out, u.line("  "+line, width))
			line = ""
			if len(out) == 2 {
				return out
			}
		}
		if line != "" {
			line += u.style(u.base, u.muted).Render(" · ")
		}
		line += s
	}
	if line != "" {
		out = append(out, u.line("  "+line, width))
	}
	return out
}

// extHelp lists every key of the current view.
func (m Model) extHelp(u softUI, v *extView, width int) []string {
	var out []string
	row := func(key, what string) {
		out = append(out, u.line("  "+u.style(u.base, u.accent).Bold(true).Render(pad(key, 14))+u.style(u.base, u.text).Render(what), width))
	}
	if len(v.doc.Panels) > 0 {
		row("tab  h/l", "move between panels")
	}
	row("j/k  ↑/↓", "select within the panel or list")
	row("enter", "open the selected record")
	if len(v.doc.Tabs) > 1 {
		row("[ ]", "previous or next section")
	}
	row(m.hint("palette"), "open the command palette")
	for _, a := range v.doc.Actions {
		what := a.Label
		if a.Scope == "item" {
			what += " (selected item)"
		}
		row(a.Key, what)
	}
	if v.doc.Searchable {
		row("/", "search this view")
	}
	row("esc", "back, then close")
	return out
}
