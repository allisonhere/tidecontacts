package ext

// Panels let a view arrange several pieces of information side by side. The
// extension says what each panel holds and in which order; the host decides how
// many columns fit, which panel has focus, and how charts are drawn. Chart
// values are integers used only for geometry: every number a user reads is a
// string the extension formatted.

// Panel is one titled area of a view.
type Panel struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Short names the panel where space is tight (compact panel strip).
	Short string `json:"short,omitempty"`
	// Count is a small figure shown beside the title.
	Count string `json:"count,omitempty"`
	// Full makes the panel span every column.
	Full   bool    `json:"full,omitempty"`
	Blocks []Block `json:"blocks"`
	// Open is followed by Enter when the panel has no selectable item.
	Open  *Target `json:"open,omitempty"`
	Empty string  `json:"empty,omitempty"`
}

// Block kinds:
//
//	lines    Lines, aligned as label · text · right
//	bars     one horizontal bar per datum
//	columns  vertical bars, one per datum
//	spark    a one-row sparkline of the data
//	trend    a multi-row line chart of the data
//	progress the first datum against Goal
//	bullet   the first datum against Goal on a shared scale
//	stack    one bar divided among the data, with a legend
//	heat     rows of shaded cells under Columns
//	text     Text, wrapped
type Block struct {
	Kind    string    `json:"kind"`
	Title   string    `json:"title,omitempty"`
	Text    string    `json:"text,omitempty"`
	Tone    string    `json:"tone,omitempty"`
	Lines   []SLine   `json:"lines,omitempty"`
	Data    []Datum   `json:"data,omitempty"`
	Goal    int64     `json:"goal,omitempty"`
	Height  int       `json:"height,omitempty"` // columns and trend rows (2–10)
	Columns []string  `json:"columns,omitempty"`
	Rows    []HeatRow `json:"rows,omitempty"`
}

// Datum is one value of a chart.
type Datum struct {
	ID      string  `json:"id,omitempty"` // passed to item actions
	Label   string  `json:"label,omitempty"`
	Value   int64   `json:"value"`
	Text    string  `json:"text,omitempty"` // the formatted figure shown to the user
	Tone    string  `json:"tone,omitempty"`
	Missing bool    `json:"missing,omitempty"`
	Target  *Target `json:"target,omitempty"`
}

// HeatRow is one row of a heat block. A nil cell is missing data.
type HeatRow struct {
	Label  string   `json:"label"`
	Values []*int64 `json:"values"`
	Text   string   `json:"text,omitempty"`
}

var blockKinds = map[string]bool{"lines": true, "bars": true, "columns": true, "spark": true, "trend": true, "progress": true, "bullet": true, "stack": true, "heat": true, "text": true}

const maxPanels, maxBlocks, maxData, maxBlockLines, maxHeatRows, maxHeatCells = 16, 12, 120, 200, 31, 60

func sanitizeLine(l SLine) SLine {
	return SLine{ID: tokenOr(l.ID), Label: Clean(l.Label, 40, false), Text: Clean(l.Text, 200, false), Right: Clean(l.Right, 30, false), Mark: Clean(l.Mark, 2, false), Tone: tone(l.Tone), Target: sanitizeTarget(l.Target)}
}

func sanitizePanels(in []Panel) []Panel {
	var out []Panel
	seen := map[string]bool{}
	for _, p := range in {
		if len(out) >= maxPanels || tokenOr(p.ID) == "" || seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		n := Panel{ID: p.ID, Title: Clean(p.Title, 48, false), Short: Clean(p.Short, 14, false), Count: Clean(p.Count, 24, false), Full: p.Full, Open: sanitizeTarget(p.Open), Empty: Clean(p.Empty, 160, false)}
		for _, b := range p.Blocks {
			if len(n.Blocks) >= maxBlocks || !blockKinds[b.Kind] {
				continue
			}
			c := Block{Kind: b.Kind, Title: Clean(b.Title, 48, false), Text: Clean(b.Text, 600, false), Tone: tone(b.Tone), Goal: b.Goal, Height: min(10, max(0, b.Height))}
			for i, l := range b.Lines {
				if i < maxBlockLines {
					c.Lines = append(c.Lines, sanitizeLine(l))
				}
			}
			for i, d := range b.Data {
				if i < maxData {
					c.Data = append(c.Data, Datum{ID: tokenOr(d.ID), Label: Clean(d.Label, 40, false), Value: d.Value, Text: Clean(d.Text, 40, false), Tone: tone(d.Tone), Missing: d.Missing, Target: sanitizeTarget(d.Target)})
				}
			}
			for i, h := range b.Columns {
				if i < maxHeatCells {
					c.Columns = append(c.Columns, Clean(h, 8, false))
				}
			}
			for i, r := range b.Rows {
				if i < maxHeatRows {
					c.Rows = append(c.Rows, HeatRow{Label: Clean(r.Label, 24, false), Values: r.Values[:min(len(r.Values), maxHeatCells)], Text: Clean(r.Text, 30, false)})
				}
			}
			n.Blocks = append(n.Blocks, c)
		}
		out = append(out, n)
	}
	return out
}
