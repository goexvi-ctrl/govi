package grid_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govi/engine"
	"govi/frontend/grid"
)

// noopFrontend is the minimal engine.Frontend a non-reactive host (one that
// pulls state via WithView) needs.
type noopFrontend struct{}

func (noopFrontend) Render(engine.View, engine.ChangeSet) {}
func (noopFrontend) Bell()                                {}
func (noopFrontend) SetTitle(string)                      {}

// feed drives a string of keystrokes into the engine. ESC (0x1b) and CR (\r)
// become the corresponding special keys; everything else is a typed rune.
func feed(e *engine.Engine, keys string) {
	for _, r := range keys {
		switch r {
		case 0x1b:
			e.Input(engine.KeyEvent{Key: engine.KeyEscape})
		case '\r', '\n':
			e.Input(engine.KeyEvent{Key: engine.KeyEnter})
		default:
			e.Input(engine.KeyEvent{Rune: r})
		}
	}
}

func compose(e *engine.Engine, rows, cols int) grid.Grid {
	var g grid.Grid
	e.WithView(func(v engine.View) { g = grid.Compose(v, rows, cols) })
	return g
}

// TestEngineThroughGrid exercises the exact path the GUI bridge uses: a real
// engine driven by input events, its View pulled via WithView and laid out by
// the grid composer. This is the embeddability proof in pure Go, independent of
// any terminal or GUI.
func TestEngineThroughGrid(t *testing.T) {
	e := engine.New(noopFrontend{}, engine.Options{})
	e.Resize(4, 20) // 4 text rows; grid is given 5 (incl. status)

	// Insert two lines of text, as a user would type them.
	feed(e, "ihello\rworld\x1b")

	g := compose(e, 5, 20)
	if got := row(&g, 0); got != "hello" {
		t.Errorf("row 0 = %q, want %q", got, "hello")
	}
	if got := row(&g, 1); got != "world" {
		t.Errorf("row 1 = %q, want %q", got, "world")
	}
	// After ESC the cursor sits on the last typed character of line 2.
	if !g.CursorVisible || g.CursorY != 1 {
		t.Errorf("cursor = (%d,%d) vis=%v, want row 1", g.CursorX, g.CursorY, g.CursorVisible)
	}

	// Delete the first line with dd; the grid should reflow.
	feed(e, "kdd")
	g = compose(e, 5, 20)
	if got := row(&g, 0); got != "world" {
		t.Errorf("after dd, row 0 = %q, want %q", got, "world")
	}
	if got := row(&g, 1); got != "~" {
		t.Errorf("after dd, row 1 = %q, want tilde filler", got)
	}
}

// rowReverse reports whether cell (x, y) is drawn in reverse video.
func rowReverse(g *grid.Grid, x, y int) bool {
	return g.At(x, y).Style&engine.StyleReverse != 0
}

// TestSplitThroughGrid drives a real engine into a horizontal split (the path
// GoVi.app uses) and checks the composed grid paints both panes, a reverse-video
// status divider between them, and the cursor in the active (lower) pane. Before
// multi-pane grid rendering, only the active screen was drawn (top half only).
func TestSplitThroughGrid(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "aaa.txt")
	b := filepath.Join(dir, "bbb.txt")
	if err := os.WriteFile(a, []byte("a1\na2\na3\na4\na5\na6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b1\nb2\nb3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := engine.New(noopFrontend{}, engine.Options{})
	if err := e.Open(a); err != nil {
		t.Fatal(err)
	}
	e.Resize(23, 80) // 24-row terminal: 23 text rows + status
	// :E (capital) opens bbb in a new horizontal split, focusing the lower pane.
	if err := e.RunEx("E " + b); err != nil {
		t.Fatal(err)
	}

	g := compose(e, 24, 80)

	// Display height 24 -> half 12: top pane rows 0..10 (status at 11), lower pane
	// rows 12..22 (status at 23).
	if got := row(&g, 0); got != "a1" {
		t.Errorf("top pane row 0 = %q, want %q", got, "a1")
	}
	if got := row(&g, 12); got != "b1" {
		t.Errorf("lower pane row 0 = %q, want %q", got, "b1")
	}
	if got := row(&g, 13); got != "b2" {
		t.Errorf("lower pane row 1 = %q, want %q", got, "b2")
	}
	// The top pane's status line (row 11) is the reverse-video divider naming aaa.
	if !rowReverse(&g, 0, 11) {
		t.Errorf("top status row 11 not reverse video")
	}
	if got := row(&g, 11); !strings.Contains(got, "aaa") {
		t.Errorf("top status row 11 = %q, want it to name aaa", got)
	}
	// The lower pane's status line (row 23) is reverse video and names bbb.
	if !rowReverse(&g, 0, 23) {
		t.Errorf("lower status row 23 not reverse video")
	}
	if got := row(&g, 23); !strings.Contains(got, "bbb") {
		t.Errorf("lower status row 23 = %q, want it to name bbb", got)
	}
	// The cursor is in the active (lower) pane: rows 12..22.
	if !g.CursorVisible || g.CursorY < 12 || g.CursorY > 22 {
		t.Errorf("cursor = (%d,%d) vis=%v, want it in the lower pane (rows 12..22)",
			g.CursorX, g.CursorY, g.CursorVisible)
	}
}

// TestVsplitThroughGrid drives a real engine into a vertical split and checks
// the composed grid paints both side-by-side panes with a '|' divider column
// between them.
func TestVsplitThroughGrid(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "aaa.txt")
	b := filepath.Join(dir, "bbb.txt")
	if err := os.WriteFile(a, []byte("a1\na2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b1\nb2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := engine.New(noopFrontend{}, engine.Options{})
	if err := e.Open(a); err != nil {
		t.Fatal(err)
	}
	e.Resize(23, 80)
	if err := e.RunEx("vsplit " + b); err != nil {
		t.Fatal(err)
	}

	g := compose(e, 24, 80)

	// Left pane occupies columns 0..39, a '|' divider at column 40, right pane to
	// its right. The new (focused) screen is on the right.
	if got := g.At(0, 0).Rune; got != 'a' && got != 'b' {
		t.Errorf("left pane top-left = %q, want a buffer glyph", got)
	}
	if got := g.At(40, 0).Rune; got != '|' {
		t.Errorf("divider at col 40 row 0 = %q, want '|'", got)
	}
	// The right pane has buffer text starting just past the divider.
	if got := g.At(41, 0).Rune; got == 0 || got == ' ' {
		t.Errorf("right pane top-left (col 41) = %q, want a buffer glyph", got)
	}
}

func row(g *grid.Grid, y int) string {
	out := make([]rune, 0, g.Cols)
	for x := 0; x < g.Cols; x++ {
		r := g.At(x, y).Rune
		if r == 0 {
			r = ' '
		}
		out = append(out, r)
	}
	// trim trailing spaces
	end := len(out)
	for end > 0 && out[end-1] == ' ' {
		end--
	}
	return string(out[:end])
}
