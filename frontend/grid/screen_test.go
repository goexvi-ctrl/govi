package grid

import (
	"strings"
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
	// Buffer text.
	if p, ok := ScreenToBuffer(v, rows, cols, 8, 1); !ok || p.Line != 2 || p.Col != 2 {
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

func TestSelectionBufferRange(t *testing.T) {
	v := &fakeView{
		lines: []string{"hello", "world"},
		top:   1,
		msg:   "status",
	}
	rows, cols := 5, 20
	// Pure buffer selection.
	sel := ScreenSelection{A: Cell{0, 0}, B: Cell{2, 0}}
	a, b, ok := SelectionBufferRange(v, rows, cols, sel)
	if !ok || a.Line != 1 || a.Col != 0 || b.Col != 2 {
		t.Errorf("buffer range = %+v-%+v,%v", a, b, ok)
	}
	// Spanning into status fails.
	sel = ScreenSelection{A: Cell{0, 0}, B: Cell{5, 4}}
	if _, _, ok := SelectionBufferRange(v, rows, cols, sel); ok {
		t.Error("buffer+status should fail")
	}
	// Gutter cell fails (line numbers on).
	vNum := &fakeView{
		lines:  []string{"hello"},
		top:    1,
		number: true,
	}
	if _, _, ok := SelectionBufferRange(vNum, 4, 20, ScreenSelection{A: Cell{2, 0}, B: Cell{8, 0}}); ok {
		t.Error("selection including gutter should fail")
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
		fakeView: fakeView{lines: []string{"x"}, top: 1},
		out:      []string{"output"},
		prompt:   "continue",
	}
	g := Compose(v, 4, 20)
	ApplyScreenSel(&g, &ScreenSelection{A: Cell{0, 0}, B: Cell{5, 0}})
	if g.At(0, 0).Style&engine.StyleReverse == 0 {
		t.Error("overlay text should highlight")
	}
	sel := ScreenSelection{A: Cell{0, 0}, B: Cell{5, 0}}
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

func TestSelectionBufferRangeMultiline(t *testing.T) {
	v := &fakeView{
		lines: []string{"hello", "world"},
		top:   1,
	}
	rows, cols := 5, 20
	sel := ScreenSelection{A: Cell{2, 0}, B: Cell{2, 1}}
	a, b, ok := SelectionBufferRange(v, rows, cols, sel)
	if !ok {
		t.Fatal("multiline buffer selection should succeed")
	}
	txt := engineRangeText(v, a, b)
	if txt != "llo\nwo" {
		t.Errorf("range text = %q, want %q", txt, "llo\nwo")
	}
}

// engineRangeText is a test helper mirroring engine.RangeText for fakeView.
func engineRangeText(v *fakeView, a, b engine.Pos) string {
	if a.Line > b.Line || (a.Line == b.Line && a.Col > b.Col) {
		a, b = b, a
	}
	var parts []string
	for ln := a.Line; ln <= b.Line; ln++ {
		line := string(v.lines[ln-1])
		start, end := 0, len(line)
		if ln == a.Line {
			start = a.Col
		}
		if ln == b.Line {
			end = b.Col
		}
		parts = append(parts, line[start:end])
	}
	return strings.Join(parts, "\n")
}