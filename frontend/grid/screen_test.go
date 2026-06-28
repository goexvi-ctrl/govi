package grid

import (
	"testing"

	"govi/engine"
)

func TestScreenRangeText(t *testing.T) {
	v := &fakeView{
		lines:  []string{"hello", "world"},
		cursor: engine.Pos{Line: 1, Col: 0},
		top:    1,
		msg:    "status here",
	}
	g := Compose(v, 5, 20)
	sel := ScreenSelection{A: Cell{0, 0}, B: Cell{2, 0}}
	if got := ScreenRangeText(g, sel); got != "hel" {
		t.Errorf("ScreenRangeText = %q, want %q", got, "hel")
	}
	// Status row.
	sel = ScreenSelection{A: Cell{0, 4}, B: Cell{10, 4}}
	if got := ScreenRangeText(g, sel); got != "status here" {
		t.Errorf("status ScreenRangeText = %q", got)
	}
}

func TestApplyScreenSel(t *testing.T) {
	v := &fakeView{lines: []string{"abc"}, top: 1, cursor: engine.Pos{Line: 1, Col: 0}}
	g := Compose(v, 4, 10)
	ApplyScreenSel(&g, &ScreenSelection{A: Cell{1, 0}, B: Cell{2, 0}})
	for _, x := range []int{1, 2} {
		if g.At(x, 0).Style&engine.StyleReverse == 0 {
			t.Errorf("cell (%d,0) should be reverse", x)
		}
	}
	if g.At(0, 0).Style&engine.StyleReverse != 0 {
		t.Error("cell (0,0) should not be reverse")
	}
}

func TestScreenToBuffer(t *testing.T) {
	v := &fakeView{
		lines:  []string{"hello", "world"},
		top:    1,
		msg:    "status",
		number: true,
	}
	rows, cols := 5, 20
	// Buffer text. Gutter is 8 wide, so screen col 10 is buffer col 2.
	if p, ok := ScreenToBuffer(v, rows, cols, 10, 1); !ok || p.Line != 2 || p.Col != 2 {
		t.Errorf("buffer cell = %+v,%v want {2,2},true", p, ok)
	}
	// Gutter.
	if _, ok := ScreenToBuffer(v, rows, cols, 2, 0); ok {
		t.Error("gutter should not map")
	}
	// Status row (row 4 when rows=5).
	if _, ok := ScreenToBuffer(v, rows, cols, 8, 4); ok {
		t.Error("status row should not map")
	}
	// Tilde row (row 2: one buffer line + tilde on row 2).
	if _, ok := ScreenToBuffer(v, rows, cols, 8, 2); ok {
		t.Error("tilde row should not map")
	}
}

func TestSelectionEditRange(t *testing.T) {
	v := &fakeView{lines: []string{"hello", "world"}, top: 1, msg: "status"}
	rows, cols := 5, 20

	// Single-line: cells (0,0)-(2,0) cover "hel"; [a,b) = (1,0)-(1,3).
	a, b, ok := SelectionEditRange(v, rows, cols, ScreenSelection{A: Cell{0, 0}, B: Cell{2, 0}})
	if !ok || a.Line != 1 || a.Col != 0 || b.Line != 1 || b.Col != 3 {
		t.Fatalf("single-line range = %+v-%+v ok=%v, want (1,0)-(1,3)", a, b, ok)
	}

	// Reversed corners normalize the same way.
	a, b, ok = SelectionEditRange(v, rows, cols, ScreenSelection{A: Cell{2, 0}, B: Cell{0, 0}})
	if !ok || a.Col != 0 || b.Col != 3 {
		t.Fatalf("reversed range = %+v-%+v ok=%v", a, b, ok)
	}

	// Multi-line: (0,0)-(4,1) ends on 'd' (col 4); b advances to (2,5).
	a, b, ok = SelectionEditRange(v, rows, cols, ScreenSelection{A: Cell{0, 0}, B: Cell{4, 1}})
	if !ok || a.Line != 1 || a.Col != 0 || b.Line != 2 || b.Col != 5 {
		t.Fatalf("multi-line range = %+v-%+v ok=%v, want (1,0)-(2,5)", a, b, ok)
	}

	// Spanning into the status row fails.
	if _, _, ok := SelectionEditRange(v, rows, cols, ScreenSelection{A: Cell{0, 0}, B: Cell{5, 4}}); ok {
		t.Error("selection into status row should fail")
	}

	// A tilde row fails (one buffer line, tilde from row 1).
	vOne := &fakeView{lines: []string{"hello"}, top: 1}
	if _, _, ok := SelectionEditRange(vOne, rows, cols, ScreenSelection{A: Cell{0, 0}, B: Cell{0, 1}}); ok {
		t.Error("selection into tilde row should fail")
	}
}

func TestSelectionEditRangeGutter(t *testing.T) {
	// With line numbers on, an interior/continuation row's gutter must NOT block
	// editing, but an endpoint that lands in the gutter must.
	v := &fakeView{lines: []string{"hello", "world"}, top: 1, number: true}
	rows, cols := 5, 20

	// Endpoints at x=8 (past the gutter) on both buffer rows: editable.
	if _, _, ok := SelectionEditRange(v, rows, cols, ScreenSelection{A: Cell{8, 0}, B: Cell{8, 1}}); !ok {
		t.Error("multi-line buffer selection should be editable with :set number")
	}

	// An endpoint in the gutter (x=0) is not buffer text: not editable.
	if _, _, ok := SelectionEditRange(v, rows, cols, ScreenSelection{A: Cell{0, 0}, B: Cell{8, 0}}); ok {
		t.Error("selection starting in the gutter should not be editable")
	}
}

func TestApplyScreenLinearSelViewSkipsGutter(t *testing.T) {
	v := &fakeView{lines: []string{"hello", "world"}, top: 1, number: true,
		cursor: engine.Pos{Line: 1, Col: 0}}
	rows, cols := 5, 20
	g := Compose(v, rows, cols)
	sel := &ScreenSelection{A: Cell{8, 0}, B: Cell{10, 1}}
	ApplyScreenLinearSelView(&g, v, rows, cols, sel)
	gutter := engine.GutterWidth(v.LineCount(), v.Number())
	if gutter < 1 {
		t.Fatalf("expected a gutter with :set number, got %d", gutter)
	}
	if g.At(0, 1).Style&engine.StyleReverse != 0 {
		t.Error("gutter cell (0,1) should not be highlighted")
	}
	if g.At(gutter, 1).Style&engine.StyleReverse == 0 {
		t.Errorf("text cell (%d,1) should be highlighted", gutter)
	}
	if g.At(8, 0).Style&engine.StyleReverse == 0 {
		t.Error("first-row text cell (8,0) should be highlighted")
	}
}

func TestScreenToBufferOverlay(t *testing.T) {
	v := &fakeView{
		lines: []string{"x"},
		top:   1,
	}
	vPending := &fakeViewOverlay{
		fakeView: fakeView{lines: []string{"x"}, top: 1},
		out:      []string{"line one", "line two"},
		prompt:   "Press any key",
	}
	if _, ok := ScreenToBuffer(vPending, 5, 20, 0, 0); ok {
		t.Error("overlay cell should not map")
	}
	_ = v
}

func TestScreenLinearRangeText(t *testing.T) {
	v := &fakeView{
		lines:  []string{"abcdef", "ghijkl"},
		cursor: engine.Pos{Line: 1, Col: 0},
		top:    1,
	}
	g := Compose(v, 5, 12)
	// Reading order from (4,0) through (1,1) — not the full 2x2 rectangle.
	sel := ScreenSelection{A: Cell{4, 0}, B: Cell{1, 1}}
	if got := ScreenLinearRangeText(g, sel); got != "ef\ngh" {
		t.Errorf("linear text = %q, want %q", got, "ef\ngh")
	}
	ApplyScreenLinearSel(&g, &sel)
	if g.At(1, 0).Style&engine.StyleReverse != 0 {
		t.Error("linear sel should not highlight (1,0)")
	}
	if g.At(4, 0).Style&engine.StyleReverse == 0 {
		t.Error("(4,0) should be highlighted")
	}
}

func TestComposeOverlayScreenSelect(t *testing.T) {
	v := &fakeViewOverlay{
		fakeView: fakeView{lines: []string{"x"}, top: 1, cursor: engine.Pos{Line: 1}},
		out:      []string{"output"},
		prompt:   "continue",
	}
	// The overlay is anchored at the bottom: in a 4-row grid with one output line
	// and the divider, "output" lands on row 2 (divider row 1, prompt row 3).
	g := Compose(v, 4, 20)
	const outRow = 2
	ApplyScreenSel(&g, &ScreenSelection{A: Cell{0, outRow}, B: Cell{5, outRow}})
	if g.At(0, outRow).Style&engine.StyleReverse == 0 {
		t.Error("overlay text should highlight")
	}
	sel := ScreenSelection{A: Cell{0, outRow}, B: Cell{5, outRow}}
	if got := ScreenRangeText(g, sel); got != "output" {
		t.Errorf("overlay text = %q", got)
	}
}

type fakeViewOverlay struct {
	fakeView
	out    []string
	prompt string
}

func (f *fakeViewOverlay) PendingOutput() []string     { return f.out }
func (f *fakeViewOverlay) PendingOutputPrompt() string { return f.prompt }

func TestComposeExModeScreenText(t *testing.T) {
	v := &fakeView{
		mode:       engine.ModeExText,
		transcript: []string{":1p", "hello"},
		msg:        ":",
	}
	g := Compose(v, 4, 20)
	sel := ScreenSelection{A: Cell{0, 0}, B: Cell{4, 1}}
	if got := ScreenRangeText(g, sel); got != ":1p\nhello" {
		t.Errorf("ex transcript = %q", got)
	}
	if _, ok := ScreenToBuffer(v, 4, 20, 0, 0); ok {
		t.Error("ex transcript should not map to buffer")
	}
}
