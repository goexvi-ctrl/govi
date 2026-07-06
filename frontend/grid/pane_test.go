package grid_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"govi/engine"
	"govi/frontend/grid"
)

// splitEngine opens a 30-line file on a 24x80 display and splits (":Edit b" or
// ":vsplit b") in a 5-line file, so the two panes show distinct buffers. The
// new pane is the active one (bottom, or right).
func splitEngine(t *testing.T, vertical bool) *engine.Engine {
	t.Helper()
	dir := t.TempDir()
	a := filepath.Join(dir, "aaa.txt")
	b := filepath.Join(dir, "bbb.txt")
	var lines []string
	for i := 1; i <= 30; i++ {
		lines = append(lines, "a"+strconv.Itoa(i))
	}
	if err := os.WriteFile(a, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b1\nb2\nb3\nb4\nb5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := engine.New(noopFrontend{}, engine.Options{})
	if err := e.Open(a); err != nil {
		t.Fatal(err)
	}
	e.Resize(23, 80) // 24 rows including the status row
	cmd := ":Edit "
	if vertical {
		cmd = ":vsplit "
	}
	feed(e, cmd+b+"\r")
	return e
}

// A viewport scrolled below the cursor line (GUI wheel/scroll bar) must hide
// the cursor, not draw it at the top row.
func TestCursorHiddenWhenViewportBelowCursor(t *testing.T) {
	e := engine.New(noopFrontend{}, engine.Options{})
	e.Resize(4, 20)
	feed(e, "il1\rl2\rl3\rl4\rl5\rl6\rl7\rl8\x1b1G")
	e.ScrollLines(3) // cursor stays on line 1, viewport starts at line 4
	g := compose(e, 5, 20)
	if got := row(&g, 0); got != "l4" {
		t.Fatalf("row 0 = %q, want %q", got, "l4")
	}
	if g.CursorVisible {
		t.Fatalf("cursor drawn at (%d,%d) though its line is above the viewport",
			g.CursorX, g.CursorY)
	}
}

// The same, in a split pane (placeCursorPane).
func TestPaneCursorHiddenWhenViewportBelowCursor(t *testing.T) {
	e := splitEngine(t, false) // active pane: bbb.txt, 5 lines, cursor line 1
	e.ScrollLines(3)
	g := compose(e, 24, 80)
	if g.CursorVisible {
		t.Fatalf("pane cursor drawn at (%d,%d) though its line is above the viewport",
			g.CursorX, g.CursorY)
	}
}

func TestPaneAtHorizontalSplit(t *testing.T) {
	e := splitEngine(t, false) // top: aaa rows 11 roff 0; bottom: bbb roff 12
	e.WithView(func(v engine.View) {
		checks := []struct {
			x, y   int
			pane   int
			region grid.PaneRegion
		}{
			{5, 3, 0, grid.PaneContent},
			{5, 10, 0, grid.PaneContent},
			{5, 11, 0, grid.PaneStatus}, // divider between the panes
			{5, 12, 1, grid.PaneContent},
			{79, 22, 1, grid.PaneContent},
			{5, 23, 1, grid.PaneStatus}, // bottom status row
		}
		for _, c := range checks {
			pane, region := grid.PaneAt(v, 80, c.x, c.y)
			if pane != c.pane || region != c.region {
				t.Errorf("PaneAt(%d,%d) = pane %d region %d, want %d/%d",
					c.x, c.y, pane, region, c.pane, c.region)
			}
		}
		if got := grid.PaneBelow(v, 0); got != 1 {
			t.Errorf("PaneBelow(0) = %d, want 1", got)
		}
		if got := grid.PaneBelow(v, 1); got != -1 {
			t.Errorf("PaneBelow(1) = %d, want -1", got)
		}
		if got := grid.PaneRight(v, 0); got != -1 {
			t.Errorf("PaneRight(0) = %d, want -1", got)
		}
	})
}

func TestPaneAtVerticalSplit(t *testing.T) {
	e := splitEngine(t, true) // left: aaa cols 40 coff 0; right: bbb coff 41
	e.WithView(func(v engine.View) {
		checks := []struct {
			x, y   int
			pane   int
			region grid.PaneRegion
		}{
			{5, 3, 0, grid.PaneContent},
			{39, 3, 0, grid.PaneContent},
			{40, 3, 0, grid.PaneVDivider}, // sacrificed '|' column
			{41, 3, 1, grid.PaneContent},
			{79, 3, 1, grid.PaneContent},
			{5, 23, 0, grid.PaneStatus},
			{45, 23, 1, grid.PaneStatus},
		}
		for _, c := range checks {
			pane, region := grid.PaneAt(v, 80, c.x, c.y)
			if pane != c.pane || region != c.region {
				t.Errorf("PaneAt(%d,%d) = pane %d region %d, want %d/%d",
					c.x, c.y, pane, region, c.pane, c.region)
			}
		}
		if got := grid.PaneRight(v, 0); got != 1 {
			t.Errorf("PaneRight(0) = %d, want 1", got)
		}
		if got := grid.PaneRight(v, 1); got != -1 {
			t.Errorf("PaneRight(1) = %d, want -1", got)
		}
		if got := grid.PaneBelow(v, 0); got != -1 {
			t.Errorf("PaneBelow(0) = %d, want -1", got)
		}
	})
}

// A divider bordering a split half is still draggable: overlap, not an exact
// border match, decides PaneBelow/PaneRight.
func TestPaneDividersAcrossMixedSplits(t *testing.T) {
	// hsplit, then vsplit the bottom: the top pane's status row still drags.
	e := splitEngine(t, false)
	feed(e, ":vsplit\r")
	e.WithView(func(v engine.View) {
		if len(v.Screens()) != 3 {
			t.Fatalf("screens = %d, want 3", len(v.Screens()))
		}
		if got := grid.PaneBelow(v, 0); got < 0 {
			t.Errorf("PaneBelow(top) = %d, want a bottom pane", got)
		}
	})

	// vsplit, then hsplit the right: the left pane's divider column still drags.
	e = splitEngine(t, true)
	feed(e, ":Edit\r")
	e.WithView(func(v engine.View) {
		if len(v.Screens()) != 3 {
			t.Fatalf("screens = %d, want 3", len(v.Screens()))
		}
		if got := grid.PaneRight(v, 0); got < 0 {
			t.Errorf("PaneRight(left) = %d, want a right pane", got)
		}
	})
}

// Clicks resolve within the active pane's own viewport/buffer, not the whole
// grid: cell (0, roff+2) in the bottom pane is that buffer's line 3.
func TestLocateActivePane(t *testing.T) {
	e := splitEngine(t, false) // active: bbb.txt at roff 12
	e.WithView(func(v engine.View) {
		p := grid.LocateActive(v, 0, 14)
		if p != (engine.Pos{Line: 3, Col: 0}) {
			t.Errorf("LocateActive(0,14) = %+v, want line 3 col 0", p)
		}
		// The pane's status row is not buffer text.
		if _, ok := grid.ScreenToBufferActive(v, 0, 23); ok {
			t.Errorf("ScreenToBufferActive on the status row reported buffer text")
		}
		// A drag above the pane clamps into it (its first row).
		p = grid.LocateActive(v, 0, 3)
		if p != (engine.Pos{Line: 1, Col: 0}) {
			t.Errorf("clamped LocateActive(0,3) = %+v, want line 1 col 0", p)
		}
	})

	// In the vertical split, x is pane-relative too.
	e = splitEngine(t, true) // active: bbb.txt at coff 41
	e.WithView(func(v engine.View) {
		p := grid.LocateActive(v, 41, 2)
		if p != (engine.Pos{Line: 3, Col: 0}) {
			t.Errorf("vsplit LocateActive(41,2) = %+v, want line 3 col 0", p)
		}
		p = grid.LocateActive(v, 42, 0)
		if p != (engine.Pos{Line: 1, Col: 1}) {
			t.Errorf("vsplit LocateActive(42,0) = %+v, want line 1 col 1", p)
		}
	})
}
