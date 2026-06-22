package grid

import (
	"strings"
	"testing"

	"govi/engine"
)

// fakeView is a minimal engine.View for exercising the composer without a full
// engine.
type fakeView struct {
	lines  []string
	cursor engine.Pos
	mode   engine.Mode
	top    int64
	msg    string
	number bool
}

func (f *fakeView) LineCount() int64 { return int64(len(f.lines)) }
func (f *fakeView) Line(lno int64) engine.DisplayLine {
	runes := []rune(f.lines[lno-1])
	w := make([]int8, len(runes))
	for i := range w {
		w[i] = 1
	}
	return engine.DisplayLine{Text: runes, Widths: w}
}
func (f *fakeView) Cursor() engine.Pos        { return f.cursor }
func (f *fakeView) Mode() engine.Mode         { return f.mode }
func (f *fakeView) Viewport() engine.Viewport { return engine.Viewport{Top: f.top} }
func (f *fakeView) Message() (string, engine.MessageKind) {
	return f.msg, engine.MsgInfo
}
func (f *fakeView) Name() string            { return "" }
func (f *fakeView) Modified() bool          { return false }
func (f *fakeView) Number() bool            { return f.number }
func (f *fakeView) ExTranscript() []string  { return nil }
func (f *fakeView) PendingOutput() []string { return nil }
func (f *fakeView) MatchHighlight() (engine.Pos, bool) {
	return engine.Pos{}, false
}

func gridRow(g *Grid, y int) string {
	var b strings.Builder
	for x := 0; x < g.Cols; x++ {
		r := g.At(x, y).Rune
		if r == 0 {
			r = ' '
		}
		b.WriteRune(r)
	}
	return strings.TrimRight(b.String(), " ")
}

func TestComposeBasic(t *testing.T) {
	v := &fakeView{
		lines:  []string{"hello", "world"},
		cursor: engine.Pos{Line: 1, Col: 0},
		top:    1,
		msg:    "\"f\" 2 lines",
	}
	g := Compose(v, 5, 20) // 4 text rows + status

	if got := gridRow(&g, 0); got != "hello" {
		t.Errorf("row 0 = %q, want %q", got, "hello")
	}
	if got := gridRow(&g, 1); got != "world" {
		t.Errorf("row 1 = %q, want %q", got, "world")
	}
	if got := gridRow(&g, 2); got != "~" {
		t.Errorf("row 2 (tilde) = %q", got)
	}
	if got := gridRow(&g, 4); got != "\"f\" 2 lines" {
		t.Errorf("status = %q", got)
	}
	if !g.CursorVisible || g.CursorX != 0 || g.CursorY != 0 {
		t.Errorf("cursor = (%d,%d) vis=%v", g.CursorX, g.CursorY, g.CursorVisible)
	}
}

func TestComposeWrap(t *testing.T) {
	// A 24-column line in a 10-wide text area wraps across 3 rows; the cursor at
	// rune 15 lands on the second wrapped row.
	long := strings.Repeat("abcdefghij", 3)[:24]
	v := &fakeView{
		lines:  []string{long},
		cursor: engine.Pos{Line: 1, Col: 15},
		top:    1,
	}
	g := Compose(v, 6, 10)
	if got := gridRow(&g, 0); got != "abcdefghij" {
		t.Errorf("row 0 = %q", got)
	}
	if g.CursorY != 1 || g.CursorX != 5 {
		t.Errorf("wrapped cursor = (%d,%d), want (5,1)", g.CursorX, g.CursorY)
	}
}

func TestLocate(t *testing.T) {
	v := &fakeView{
		lines: []string{"hello", "world"},
		top:   1,
	}
	// Click on the 'r' of "world" (row 1, col 2).
	p := Locate(v, 5, 20, 2, 1)
	if p.Line != 2 || p.Col != 2 {
		t.Errorf("Locate(2,1) = %+v, want line 2 col 2", p)
	}
	// Click far right of line 0 lands at end-of-line caret (col 5).
	p = Locate(v, 5, 20, 19, 0)
	if p.Line != 1 || p.Col != 5 {
		t.Errorf("Locate past EOL = %+v, want line 1 col 5", p)
	}
	// Click below the last line clamps to the end of the last line.
	p = Locate(v, 5, 20, 0, 4)
	if p.Line != 2 || p.Col != 5 {
		t.Errorf("Locate below buffer = %+v, want line 2 col 5", p)
	}
}

func TestLocateWithGutter(t *testing.T) {
	v := &fakeView{lines: []string{"abc"}, top: 1, number: true}
	// Gutter is 6 wide; clicking screen column 6 is buffer col 0.
	if p := Locate(v, 4, 20, 6, 0); p.Col != 0 {
		t.Errorf("Locate col 6 with gutter = %+v, want col 0", p)
	}
	if p := Locate(v, 4, 20, 8, 0); p.Col != 2 {
		t.Errorf("Locate col 8 with gutter = %+v, want col 2", p)
	}
}

func TestComposeSelection(t *testing.T) {
	v := &fakeView{lines: []string{"hello", "world"}, top: 1, cursor: engine.Pos{Line: 1, Col: 0}}
	// Select "llo\nwo" : from line 1 col 2 to line 2 col 2.
	sel := &Selection{A: engine.Pos{Line: 1, Col: 2}, B: engine.Pos{Line: 2, Col: 2}}
	g := ComposeSel(v, 5, 20, sel)

	rev := func(x, y int) bool { return g.At(x, y).Style&engine.StyleReverse != 0 }
	// Line 1: cols 0,1 normal; 2,3,4 selected.
	if rev(0, 0) || rev(1, 0) {
		t.Error("line 1 cols 0-1 should not be selected")
	}
	if !rev(2, 0) || !rev(4, 0) {
		t.Error("line 1 cols 2-4 should be selected")
	}
	// Line 2: cols 0,1 selected; 2.. normal.
	if !rev(0, 1) || !rev(1, 1) {
		t.Error("line 2 cols 0-1 should be selected")
	}
	if rev(2, 1) {
		t.Error("line 2 col 2 should not be selected")
	}
}

func TestComposeGutter(t *testing.T) {
	v := &fakeView{
		lines:  []string{"a", "b"},
		cursor: engine.Pos{Line: 2, Col: 0},
		top:    1,
		number: true,
	}
	g := Compose(v, 4, 20)
	// Gutter is right-aligned line numbers in a 6-wide column ("    1 ").
	if got := gridRow(&g, 0); got != "    1 a" {
		t.Errorf("gutter row 0 = %q", got)
	}
	if g.CursorY != 1 || g.CursorX != 6 {
		t.Errorf("cursor with gutter = (%d,%d), want (6,1)", g.CursorX, g.CursorY)
	}
}
