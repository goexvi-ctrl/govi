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
