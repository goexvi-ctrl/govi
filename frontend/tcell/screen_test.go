package tcell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tc "github.com/gdamore/tcell/v2"

	"govi/engine"
	"govi/frontend/grid"
)

// rowsOf reads the simulation screen back into trimmed row strings.
func rowsOf(t *testing.T, sim tc.SimulationScreen) []string {
	t.Helper()
	cells, w, h := sim.GetContents()
	out := make([]string, h)
	for y := 0; y < h; y++ {
		var b strings.Builder
		for x := 0; x < w; x++ {
			rs := cells[y*w+x].Runes
			if len(rs) == 0 || rs[0] == 0 {
				b.WriteRune(' ')
			} else {
				b.WriteRune(rs[0])
			}
		}
		out[y] = strings.TrimRight(b.String(), " ")
	}
	return out
}

func setup(t *testing.T, content string, w, h int) (*engine.Engine, tc.SimulationScreen) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	sim := tc.NewSimulationScreen("")
	fe, err := NewWithScreen(sim)
	if err != nil {
		t.Fatal(err)
	}
	sim.SetSize(w, h)
	eng := engine.New(fe, engine.Options{})
	fe.Attach(eng)
	if err := eng.Open(path); err != nil {
		t.Fatal(err)
	}
	eng.Resize(textRows(h), w)
	return eng, sim
}

// TestFrontendRendersBuffer proves the full seam end to end: the engine drives
// the real tcell frontend (via a SimulationScreen), and the rendered grid shows
// the buffer with tildes past EOF -- with no real terminal involved.
func TestFrontendRendersBuffer(t *testing.T) {
	_, sim := setup(t, "alpha\nbeta\n", 20, 5)
	rows := rowsOf(t, sim)

	if rows[0] != "alpha" {
		t.Errorf("row0 = %q, want alpha", rows[0])
	}
	if rows[1] != "beta" {
		t.Errorf("row1 = %q, want beta", rows[1])
	}
	// rows[2],[3] are text rows past EOF -> "~"; row 4 is the status line.
	if rows[2] != "~" || rows[3] != "~" {
		t.Errorf("EOF rows = %q,%q, want ~,~", rows[2], rows[3])
	}
}

func TestFrontendTabExpansion(t *testing.T) {
	_, sim := setup(t, "a\tb\n", 20, 3)
	rows := rowsOf(t, sim)
	// Tab from column 1 expands to the next multiple of 8.
	if rows[0] != "a       b" {
		t.Errorf("tab row = %q, want %q", rows[0], "a       b")
	}
}

func TestFrontendCursorTracksMotion(t *testing.T) {
	eng, sim := setup(t, "hello\nworld\n", 20, 4)

	eng.Input(engine.KeyEvent{Rune: 'j'})
	eng.Input(engine.KeyEvent{Rune: 'l'})
	eng.Input(engine.KeyEvent{Rune: 'l'})

	x, y, vis := sim.GetCursor()
	if !vis {
		t.Fatal("cursor should be visible")
	}
	if x != 2 || y != 1 {
		t.Fatalf("cursor at (%d,%d), want (2,1)", x, y)
	}
}

func TestFrontendNumberGutter(t *testing.T) {
	eng, sim := setup(t, "alpha\nbeta\n", 20, 4)
	eng.Input(engine.KeyEvent{Rune: ':'})
	for _, r := range "set number" {
		eng.Input(engine.KeyEvent{Rune: r})
	}
	eng.Input(engine.KeyEvent{Key: engine.KeyEnter})

	rows := rowsOf(t, sim)
	// Gutter is right-aligned in a 7-wide field + space (nvi O_NUMBER_LENGTH 8).
	if rows[0] != "      1 alpha" {
		t.Errorf("row0 = %q, want %q", rows[0], "      1 alpha")
	}
	if rows[1] != "      2 beta" {
		t.Errorf("row1 = %q, want %q", rows[1], "      2 beta")
	}
	// Cursor sits after the gutter at column 8.
	x, y, _ := sim.GetCursor()
	if x != 8 || y != 0 {
		t.Fatalf("cursor (%d,%d), want (8,0)", x, y)
	}
}

func TestFrontendWrapsLongLines(t *testing.T) {
	// Width 10; a 25-column line wraps onto three rows.
	_, sim := setup(t, "abcdefghijklmnopqrstuvwxy\nnext\n", 10, 6)
	rows := rowsOf(t, sim)
	if rows[0] != "abcdefghij" {
		t.Errorf("row0 = %q", rows[0])
	}
	if rows[1] != "klmnopqrst" {
		t.Errorf("row1 = %q", rows[1])
	}
	if rows[2] != "uvwxy" {
		t.Errorf("row2 = %q", rows[2])
	}
	if rows[3] != "next" {
		t.Errorf("row3 (next logical line) = %q, want next", rows[3])
	}
	if rows[4] != "~" {
		t.Errorf("row4 = %q, want ~", rows[4])
	}
}

func TestFrontendCursorOnWrappedLine(t *testing.T) {
	eng, sim := setup(t, "abcdefghijklmnopqrstuvwxy\n", 10, 6)
	eng.Input(engine.KeyEvent{Rune: '$'}) // last char, col 24
	x, y, _ := sim.GetCursor()
	// col 24 -> display col 24 -> row 24/10 = 2, x = 24%10 = 4
	if x != 4 || y != 2 {
		t.Fatalf("cursor at (%d,%d), want (4,2)", x, y)
	}
}

func TestFrontendShowmatchFlash(t *testing.T) {
	eng, sim := setup(t, "\n", 20, 4)
	eng.RunEx("set showmatch")
	for _, r := range "i(abc" {
		eng.Input(engine.KeyEvent{Rune: r})
	}
	eng.Input(engine.KeyEvent{Rune: ')'})
	// The cursor should flash at the matching '(' (column 0), not at the
	// insertion point (column 5).
	x, y, vis := sim.GetCursor()
	if !vis || x != 0 || y != 0 {
		t.Fatalf("showmatch cursor at (%d,%d) vis=%v, want (0,0)", x, y, vis)
	}
	// A timeout returns the cursor to the insertion point.
	eng.Input(engine.TimeoutEvent{})
	x, _, _ = sim.GetCursor()
	if x != 5 {
		t.Fatalf("after timeout cursor x = %d, want 5", x)
	}
}

// gridRowsOf lays out the same view through the grid composer (the GoVi.app
// path) and reads it back into trimmed row strings, like rowsOf does for tcell.
func gridRowsOf(g grid.Grid) []string {
	out := make([]string, g.Rows)
	for y := 0; y < g.Rows; y++ {
		var b strings.Builder
		for x := 0; x < g.Cols; x++ {
			r := g.At(x, y).Rune
			if r == 0 {
				r = ' '
			}
			b.WriteRune(r)
		}
		out[y] = strings.TrimRight(b.String(), " ")
	}
	return out
}

// TestSplitGridMatchesTcell is the parity proof for GoVi.app's split rendering:
// it drives the engine into a horizontal then a vertical split and asserts the
// grid composer (the GUI path) lays out exactly the same rows -- and places the
// cursor in the same cell -- as the terminal frontend, for identical engine
// state. Before multi-pane grid rendering the grid drew only the active screen.
func TestSplitGridMatchesTcell(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "aaa.txt")
	b := filepath.Join(dir, "bbb.txt")
	if err := os.WriteFile(a, []byte("a1\na2\na3\na4\na5\na6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b1\nb2\nb3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name string
		cmd  string
	}{
		{"horizontal", "E " + b},
		{"vertical", "vsplit " + b},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w, h := 80, 24
			sim := tc.NewSimulationScreen("")
			fe, err := NewWithScreen(sim)
			if err != nil {
				t.Fatal(err)
			}
			sim.SetSize(w, h)
			eng := engine.New(fe, engine.Options{})
			fe.Attach(eng)
			if err := eng.Open(a); err != nil {
				t.Fatal(err)
			}
			eng.Resize(textRows(h), w)
			if err := eng.RunEx(tt.cmd); err != nil {
				t.Fatal(err)
			}

			// Render both frontends from the *same* View instant, so a one-shot
			// transient status (e.g. a lock message that reverts to the modeline on
			// the next refresh) cannot differ between them.
			var g grid.Grid
			eng.WithView(func(v engine.View) {
				g = grid.Compose(v, h, w)
				fe.paintNow(v)
			})
			tcRows := rowsOf(t, sim)
			tcx, tcy, _ := sim.GetCursor()
			gRows := gridRowsOf(g)

			for y := 0; y < h; y++ {
				if tcRows[y] != gRows[y] {
					t.Errorf("row %d differs:\n tcell=%q\n grid =%q", y, tcRows[y], gRows[y])
				}
			}
			if g.CursorX != tcx || g.CursorY != tcy {
				t.Errorf("cursor differs: tcell=(%d,%d) grid=(%d,%d)", tcx, tcy, g.CursorX, g.CursorY)
			}
		})
	}
}

func TestFrontendColonLine(t *testing.T) {
	eng, sim := setup(t, "x\n", 20, 3)
	eng.Input(engine.KeyEvent{Rune: ':'})
	eng.Input(engine.KeyEvent{Rune: 'w'})

	rows := rowsOf(t, sim)
	status := rows[len(rows)-1]
	if status != ":w" {
		t.Fatalf("status line = %q, want :w", status)
	}
	// Cursor sits on the status row at the end of the colon line.
	x, y, _ := sim.GetCursor()
	if y != textRows(3) || x != 2 {
		t.Fatalf("colon cursor at (%d,%d), want (2,%d)", x, y, textRows(3))
	}
}
