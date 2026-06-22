package grid_test

import (
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
