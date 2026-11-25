package ext

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizePanelsBoundsAndCleans(t *testing.T) {
	hostile := "\x1b[31mred\x1b[0m\x07\n" + strings.Repeat("x", 1000)
	var panels []Panel
	for i := 0; i < 40; i++ {
		p := Panel{ID: fmt.Sprintf("p%d", i), Title: hostile, Short: hostile, Count: hostile, Open: &Target{Kind: "shell", ID: "x"}}
		for j := 0; j < 40; j++ {
			b := Block{Kind: "bars", Title: hostile, Text: hostile, Tone: "#ff0000", Height: 10_000}
			for k := 0; k < 300; k++ {
				b.Data = append(b.Data, Datum{ID: "bad id\n", Label: hostile, Text: hostile, Value: -1, Target: &Target{Kind: "ext", ID: "ok"}})
				b.Lines = append(b.Lines, SLine{Text: hostile, Mark: hostile, Right: hostile})
				b.Rows = append(b.Rows, HeatRow{Label: hostile, Values: make([]*int64, 500)})
				b.Columns = append(b.Columns, hostile)
			}
			p.Blocks = append(p.Blocks, b)
		}
		panels = append(panels, p)
	}
	panels = append([]Panel{{ID: "evil", Blocks: []Block{{Kind: "html", Text: "<b>"}}}, {ID: "evil"}, {ID: "no spaces allowed"}}, panels...)
	out := SanitizeView(View{Panels: panels}).Panels
	if len(out) != maxPanels || len(out[0].Blocks) != 0 || out[1].ID != "p0" {
		t.Fatalf("%d panels, first has %d blocks", len(out), len(out[0].Blocks))
	}
	p := out[1]
	b := p.Blocks[0]
	if len(p.Blocks) != maxBlocks || len(b.Data) != maxData || len(b.Lines) != maxBlockLines || len(b.Rows) != maxHeatRows || len(b.Rows[0].Values) != maxHeatCells || len(b.Columns) != maxHeatCells {
		t.Fatal("collections are not bounded")
	}
	if b.Height != 10 || b.Tone != "" || p.Open != nil || b.Data[0].ID != "" || b.Data[0].Target == nil {
		t.Fatalf("%+v", b.Data[0])
	}
	for _, s := range []string{p.Title, p.Short, p.Count, b.Title, b.Text, b.Data[0].Label, b.Data[0].Text, b.Lines[0].Text, b.Lines[0].Mark, b.Lines[0].Right, b.Rows[0].Label, b.Columns[0]} {
		if strings.ContainsAny(s, "\x1b\x07\n") || utf8.RuneCountInString(s) > 600 {
			t.Fatalf("unsafe string %q", s)
		}
	}
	if utf8.RuneCountInString(b.Lines[0].Mark) > 2 || utf8.RuneCountInString(p.Short) > 14 {
		t.Fatal("short fields not truncated")
	}
}
