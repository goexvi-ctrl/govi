package engine

import (
	"strconv"
	"strings"
	"testing"
)

// manyLines builds "l1\nl2\n...\ln\n".
func manyLines(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("l")
		b.WriteString(strconv.Itoa(i))
		b.WriteString("\n")
	}
	return b.String()
}

// GUI scrolling moves the viewport but not the cursor; the next key brings the
// cursor back into view via the Input post-dispatch clamp.
func TestScrollLinesLeavesCursorThenSnapsBack(t *testing.T) {
	e, _, _ := newTestEngine(t, manyLines(50)) // 10 text rows
	if e.scr.cursor != (Pos{1, 0}) {
		t.Fatalf("cursor = %+v, want {1,0}", e.scr.cursor)
	}
	e.ScrollLines(30)
	if e.scr.top != 31 {
		t.Fatalf("top after ScrollLines(30) = %d, want 31", e.scr.top)
	}
	if e.scr.cursor != (Pos{1, 0}) {
		t.Fatalf("cursor moved with the viewport: %+v", e.scr.cursor)
	}
	// Cursor is above the viewport; any key must scroll it back into view.
	drive(e, "j")
	if e.scr.cursor != (Pos{2, 0}) {
		t.Fatalf("cursor after j = %+v, want {2,0}", e.scr.cursor)
	}
	if e.scr.top > e.scr.cursor.Line {
		t.Fatalf("viewport did not snap back: top=%d cursor=%d", e.scr.top, e.scr.cursor.Line)
	}
}

func TestScrollLinesPaneAndSetPaneTop(t *testing.T) {
	e, _, _ := twoFileSplit(t) // pane 0 = aaa (6 lines), pane 1 = bbb (active)
	if e.cur != 1 {
		t.Fatalf("active pane = %d, want 1", e.cur)
	}
	e.ScrollLinesPane(0, 2) // scroll the inactive pane
	if got := e.screens[0].top; got != 3 {
		t.Fatalf("pane 0 top = %d, want 3", got)
	}
	if got := e.screens[1].top; got != 1 {
		t.Fatalf("pane 1 top = %d, want 1 (untouched)", got)
	}
	if e.screens[0].cursor != (Pos{1, 0}) {
		t.Fatalf("pane 0 cursor moved: %+v", e.screens[0].cursor)
	}
	// Clamps: past EOF and before line 1.
	e.SetPaneTop(0, 999)
	if got := e.screens[0].top; got != 6 {
		t.Fatalf("SetPaneTop(999) top = %d, want 6 (line count)", got)
	}
	e.SetPaneTop(0, -3)
	if got := e.screens[0].top; got != 1 {
		t.Fatalf("SetPaneTop(-3) top = %d, want 1", got)
	}
	// Out-of-range panes are ignored.
	e.ScrollLinesPane(7, 1)
	e.SetPaneTop(-1, 4)
}

func TestFocusPane(t *testing.T) {
	e, _, _ := twoFileSplit(t)
	if e.cur != 1 {
		t.Fatalf("active pane = %d, want 1", e.cur)
	}
	e.screens[0].msg, e.screens[1].msg = "", ""
	e.FocusPane(0)
	if e.cur != 0 || e.scr != e.screens[0] {
		t.Fatalf("FocusPane(0): cur=%d", e.cur)
	}
	// Like ^W, both the pane left and the pane entered get the transient status.
	if e.screens[0].msg == "" || e.screens[1].msg == "" {
		t.Fatalf("transient status not set: %q / %q", e.screens[0].msg, e.screens[1].msg)
	}
	// Focusing the active pane or a bogus index is a no-op.
	e.screens[0].msg = ""
	e.FocusPane(0)
	if e.screens[0].msg != "" {
		t.Fatalf("re-focusing the active pane set a status")
	}
	e.FocusPane(9)
	if e.cur != 0 {
		t.Fatalf("FocusPane(9) changed cur to %d", e.cur)
	}
}

func TestDragDividerRows(t *testing.T) {
	e, _, _ := twoFileSplit(t) // top: roff 0 rows 11; bottom: roff 12 rows 11
	top, bot := e.screens[0], e.screens[1]

	e.DragDividerRows(0, 3) // divider down: top grows
	if top.rows != 14 || bot.roff != 15 || bot.rows != 8 {
		t.Fatalf("after +3: top.rows=%d bot.roff=%d bot.rows=%d", top.rows, bot.roff, bot.rows)
	}
	if top.roff+top.rows+1 != bot.roff {
		t.Fatalf("screens not contiguous after drag")
	}
	if top.mapRows != top.rows || bot.mapRows != bot.rows {
		t.Fatalf("map rows not reset: %d/%d %d/%d", top.mapRows, top.rows, bot.mapRows, bot.rows)
	}

	// Clamped at the neighbor's minimum height.
	e.DragDividerRows(0, 100)
	if bot.rows != minScreenRows {
		t.Fatalf("bottom pane shrank past minimum: %d", bot.rows)
	}
	e.DragDividerRows(0, -100)
	if top.rows != minScreenRows {
		t.Fatalf("top pane shrank past minimum: %d", top.rows)
	}
	if top.rows+1+bot.rows+1 != 24 {
		t.Fatalf("display rows not conserved: %d + %d", top.rows, bot.rows)
	}

	// The bottom pane's status row is not a divider: no pane below.
	r0, r1 := top.rows, bot.rows
	e.DragDividerRows(1, 2)
	if top.rows != r0 || bot.rows != r1 {
		t.Fatalf("drag below the bottom pane resized something")
	}
}

func TestDragDividerCols(t *testing.T) {
	e, _, _ := twoFileVsplit(t) // left: coff 0 cols 40; right: coff 41 cols 39
	left, right := e.screens[0], e.screens[1]
	if left.cols != 40 || right.coff != 41 || right.cols != 39 {
		t.Fatalf("vsplit geometry: left.cols=%d right.coff=%d right.cols=%d",
			left.cols, right.coff, right.cols)
	}

	e.DragDividerCols(0, 5) // divider right: left grows
	if left.cols != 45 || right.coff != 46 || right.cols != 34 {
		t.Fatalf("after +5: left.cols=%d right.coff=%d right.cols=%d",
			left.cols, right.coff, right.cols)
	}
	if left.coff+left.cols+1 != right.coff {
		t.Fatalf("panes not contiguous after drag")
	}

	// Clamped at the panes' minimum widths.
	e.DragDividerCols(0, 100)
	if right.cols != minScreenCols {
		t.Fatalf("right pane shrank past minimum: %d", right.cols)
	}
	e.DragDividerCols(0, -100)
	if left.cols != minScreenCols {
		t.Fatalf("left pane shrank past minimum: %d", left.cols)
	}
	if left.cols+1+right.cols != 80 {
		t.Fatalf("columns not conserved: %d + %d", left.cols, right.cols)
	}

	// The right pane has no right neighbor: no-op.
	c0, c1 := left.cols, right.cols
	e.DragDividerCols(1, 2)
	if left.cols != c0 || right.cols != c1 {
		t.Fatalf("drag right of the rightmost pane resized something")
	}
}

// Dragging the horizontal divider between a whole pane and a vertically split
// half moves the entire border: all three panes resize.
func TestDragDividerRowsAcrossVsplit(t *testing.T) {
	e, a, _ := twoFileSplit(t) // top @0 rows 11; bottom @12 rows 11 (active)
	if err := e.vsplitNewScreen(a); err != nil {
		t.Fatal(err)
	}
	top, bl, br := e.screens[0], e.screens[1], e.screens[2]
	if bl.roff != 12 || br.roff != 12 || bl.coff != 0 || br.coff != 41 {
		t.Fatalf("layout: bl @%d,%d br @%d,%d", bl.roff, bl.coff, br.roff, br.coff)
	}

	e.DragDividerRows(0, 3)
	if top.rows != 14 {
		t.Fatalf("top.rows = %d, want 14", top.rows)
	}
	for _, p := range []*screen{bl, br} {
		if p.roff != 15 || p.rows != 8 {
			t.Fatalf("bottom pane not moved with the border: roff=%d rows=%d", p.roff, p.rows)
		}
	}

	// Clamped on the shrinking side (both bottom panes).
	e.DragDividerRows(0, 100)
	if bl.rows != minScreenRows || br.rows != minScreenRows {
		t.Fatalf("bottom panes shrank past minimum: %d/%d", bl.rows, br.rows)
	}
	if top.roff+top.rows+1 != bl.roff {
		t.Fatalf("panes not contiguous after clamped drag")
	}
}

// Dragging the vertical divider between a full-height pane and a horizontally
// split half moves the entire border likewise.
func TestDragDividerColsAcrossHsplit(t *testing.T) {
	e, _, b := twoFileVsplit(t) // left @0 cols 40; right @41 cols 39 (active)
	if err := e.editNewScreen(b); err != nil {
		t.Fatal(err)
	}
	left, rt, rb := e.screens[0], e.screens[1], e.screens[2]
	if rt.coff != 41 || rb.coff != 41 || rt.roff != 0 || rb.roff != 12 {
		t.Fatalf("layout: rt @%d,%d rb @%d,%d", rt.roff, rt.coff, rb.roff, rb.coff)
	}

	e.DragDividerCols(0, 5)
	if left.cols != 45 {
		t.Fatalf("left.cols = %d, want 45", left.cols)
	}
	for _, p := range []*screen{rt, rb} {
		if p.coff != 46 || p.cols != 34 {
			t.Fatalf("right pane not moved with the border: coff=%d cols=%d", p.coff, p.cols)
		}
	}

	// Clamped on the shrinking side (both right panes).
	e.DragDividerCols(0, 100)
	if rt.cols != minScreenCols || rb.cols != minScreenCols {
		t.Fatalf("right panes shrank past minimum: %d/%d", rt.cols, rb.cols)
	}
	if left.coff+left.cols+1 != rt.coff {
		t.Fatalf("panes not contiguous after clamped drag")
	}
}

// A terminal resize must preserve the split topology: a vertical split stays
// side by side, its divider scaled proportionally.
func TestResizeKeepsVsplit(t *testing.T) {
	e, _, _ := twoFileVsplit(t) // left: coff 0 cols 40; right: coff 41 cols 39
	left, right := e.screens[0], e.screens[1]

	e.Resize(23, 120) // grow width 80 -> 120: divider 40 -> 60
	if left.coff != 0 || left.cols != 60 {
		t.Fatalf("left after resize: coff=%d cols=%d, want 0/60", left.coff, left.cols)
	}
	if right.coff != 61 || right.cols != 59 {
		t.Fatalf("right after resize: coff=%d cols=%d, want 61/59", right.coff, right.cols)
	}
	if left.roff != 0 || right.roff != 0 || left.rows != 23 || right.rows != 23 {
		t.Fatalf("vsplit rows changed: left %d/%d right %d/%d",
			left.roff, left.rows, right.roff, right.rows)
	}

	e.Resize(11, 120) // shrink height 24 -> 12: both panes 11 text rows
	if left.rows != 11 || right.rows != 11 || left.coff != 0 || right.coff != 61 {
		t.Fatalf("after height shrink: left %d cols@%d right %d cols@%d",
			left.rows, left.coff, right.rows, right.coff)
	}
}

// A resize scales an hsplit's divider proportionally, keeping the panes
// contiguous (each pane's status row is its bottom border).
func TestResizeScalesHsplit(t *testing.T) {
	e, _, _ := twoFileSplit(t)   // top rows 11 @0, bottom rows 11 @12 (24 rows)
	e.DragDividerRows(0, 3)      // top 14 rows, bottom 8 rows @15
	e.Resize(47, 80)             // double the height: borders 15/24 -> 30/48
	top, bot := e.screens[0], e.screens[1]
	if top.roff != 0 || top.rows != 29 {
		t.Fatalf("top after resize: roff=%d rows=%d, want 0/29", top.roff, top.rows)
	}
	if bot.roff != 30 || bot.rows != 17 {
		t.Fatalf("bottom after resize: roff=%d rows=%d, want 30/17", bot.roff, bot.rows)
	}
	if top.roff+top.rows+1 != bot.roff {
		t.Fatalf("panes not contiguous after resize")
	}
}

// A mixed layout (vsplit, then hsplit in the right half) survives a resize
// with its structure intact.
func TestResizeKeepsMixedLayout(t *testing.T) {
	e, a, _ := twoFileVsplit(t) // active: right pane
	if err := e.editNewScreen(a); err != nil {
		t.Fatal(err)
	}
	if len(e.screens) != 3 {
		t.Fatalf("screens = %d, want 3", len(e.screens))
	}
	e.Resize(47, 120) // 24x80 -> 48x120
	var lefts, rights []*screen
	for _, s := range e.screens {
		if s.coff == 0 {
			lefts = append(lefts, s)
		} else {
			rights = append(rights, s)
		}
	}
	if len(lefts) != 1 || len(rights) != 2 {
		t.Fatalf("topology lost: %d full-height left, %d right", len(lefts), len(rights))
	}
	if lefts[0].rows != 47 || lefts[0].cols != 60 {
		t.Fatalf("left pane: rows=%d cols=%d, want 47/60", lefts[0].rows, lefts[0].cols)
	}
	if rights[0].coff != 61 || rights[1].coff != 61 {
		t.Fatalf("right panes not aligned at the divider: %d/%d",
			rights[0].coff, rights[1].coff)
	}
	if rights[0].roff+rights[0].rows+1 != rights[1].roff {
		t.Fatalf("right stack not contiguous")
	}
	if rights[1].roff+rights[1].rows+1 != 48 {
		t.Fatalf("right stack does not end at the terminal bottom")
	}
}

// When the new size cannot fit the tiling (divider columns collide), relayout
// falls back to stacking the screens full-width rather than corrupting the
// geometry.
func TestResizeTinyFallsBackToStack(t *testing.T) {
	e, _, _ := twoFileVsplit(t)
	e.Resize(23, 2) // 2 columns cannot hold two panes plus a divider
	a, b := e.screens[0], e.screens[1]
	if a.coff != 0 || b.coff != 0 || a.cols != 2 || b.cols != 2 {
		t.Fatalf("fallback stack: a coff=%d cols=%d, b coff=%d cols=%d",
			a.coff, a.cols, b.coff, b.cols)
	}
	if a.roff+a.rows+1 != b.roff || b.roff+b.rows+1 != 24 {
		t.Fatalf("fallback stack not contiguous: a %d+%d, b %d+%d",
			a.roff, a.rows, b.roff, b.rows)
	}
}
