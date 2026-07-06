package grid

import "govi/engine"

// Pane-aware layout queries. A split display is composed of panes (one
// engine.ScreenView each) stacked in the grid; a lone screen is a single pane
// covering the whole grid. Every pane's band is Rows() text rows at
// (Roff, Coff), its status row at Roff+Rows, and -- for a vertically split
// pane -- a sacrificed divider column at Coff+Cols. The functions here let a
// GUI hit-test mouse coordinates against that layout and map cells to buffer
// carets within one pane, mirroring composeScreen's math the way Locate
// mirrors composeEditor's.

// PaneRegion classifies what part of the display a grid cell hits.
type PaneRegion int

const (
	PaneNone     PaneRegion = iota // outside every pane (ex transcript, gaps)
	PaneContent                    // a pane's text area
	PaneStatus                     // a pane's status/modeline row
	PaneVDivider                   // the divider column right of a pane
)

// PaneAt returns the index (in v.Screens() order) of the pane at grid cell
// (x, y) and the region hit. cols is the full grid width: a pane ending at the
// grid edge has no divider column. In ex (Q) mode the window is a transcript,
// not panes, so the active pane is returned with PaneNone.
func PaneAt(v engine.View, cols, x, y int) (int, PaneRegion) {
	svs := v.Screens()
	if v.Mode() == engine.ModeExText {
		return activeIndex(svs), PaneNone
	}
	for i, sv := range svs {
		x0, y0, w, h := sv.Coff(), sv.Roff(), sv.Cols(), sv.Rows()
		switch {
		case x >= x0 && x < x0+w && y >= y0 && y < y0+h:
			return i, PaneContent
		case x >= x0 && x < x0+w && y == y0+h:
			return i, PaneStatus
		case x == x0+w && x0+w < cols && y >= y0 && y <= y0+h:
			return i, PaneVDivider
		}
	}
	return activeIndex(svs), PaneNone
}

// PaneBelow returns the index of a pane whose top border is pane i's status
// row with overlapping column span -- i.e. dragging that divider will resize
// something (Engine.DragDividerRows moves the whole connected border, so
// overlap is enough; the pane below may itself be split). -1 when the status
// row is not a draggable divider (bottom pane).
func PaneBelow(v engine.View, i int) int {
	svs := v.Screens()
	if i < 0 || i >= len(svs) {
		return -1
	}
	s := svs[i]
	for j, t := range svs {
		if j != i && t.Roff() == s.Roff()+s.Rows()+1 &&
			t.Coff() < s.Coff()+s.Cols()+1 && t.Coff()+t.Cols()+1 > s.Coff() {
			return j
		}
	}
	return -1
}

// PaneRight returns the index of a pane starting just past pane i's divider
// column with overlapping display band (dragging resizes the whole connected
// border, so the pane to the right may itself be split), or -1 when that
// divider is not draggable.
func PaneRight(v engine.View, i int) int {
	svs := v.Screens()
	if i < 0 || i >= len(svs) {
		return -1
	}
	s := svs[i]
	for j, t := range svs {
		if j != i && t.Coff() == s.Coff()+s.Cols()+1 &&
			t.Roff() < s.Roff()+s.Rows()+1 && t.Roff()+t.Rows()+1 > s.Roff() {
			return j
		}
	}
	return -1
}

func activeIndex(svs []engine.ScreenView) int {
	for i, sv := range svs {
		if sv.Active() {
			return i
		}
	}
	return 0
}

// activePane returns the active pane's ScreenView.
func activePane(v engine.View) engine.ScreenView {
	svs := v.Screens()
	return svs[activeIndex(svs)]
}

// paneLocal translates grid cell coordinates into a pane's local frame and
// returns the rows/cols arguments the single-screen layout functions expect
// for that pane: its text rows plus its own status row, exactly the shape
// composeEditor lays out. For a lone (unsplit) screen this is the identity.
func paneLocal(sv engine.ScreenView, x, y int) (rows, cols, lx, ly int) {
	return sv.Rows() + 1, sv.Cols(), x - sv.Coff(), y - sv.Roff()
}

// LocateActive is Locate against the active pane: it maps grid cell (x, y) to
// the buffer caret it sits on, with coordinates clamped into the pane (a
// selection drag may leave it). Split-aware counterpart of Locate.
func LocateActive(v engine.View, x, y int) engine.Pos {
	sv := activePane(v)
	rows, cols, lx, ly := paneLocal(sv, x, y)
	return Locate(sv, rows, cols, lx, ly)
}

// ScreenToBufferActive is ScreenToBuffer against the active pane.
func ScreenToBufferActive(v engine.View, x, y int) (engine.Pos, bool) {
	sv := activePane(v)
	rows, cols, lx, ly := paneLocal(sv, x, y)
	return ScreenToBuffer(sv, rows, cols, lx, ly)
}

// SelectionEditRangeActive is SelectionEditRange with the selection's cells
// given in grid coordinates, resolved against the active pane.
func SelectionEditRangeActive(v engine.View, sel ScreenSelection) (a, b engine.Pos, ok bool) {
	sv := activePane(v)
	local := ScreenSelection{
		A: Cell{X: sel.A.X - sv.Coff(), Y: sel.A.Y - sv.Roff()},
		B: Cell{X: sel.B.X - sv.Coff(), Y: sel.B.Y - sv.Roff()},
	}
	return SelectionEditRange(sv, sv.Rows()+1, sv.Cols(), local)
}

// CellOfActive is CellOf against the active pane, returning grid coordinates.
func CellOfActive(v engine.View, p engine.Pos) (x, y int, visible bool) {
	sv := activePane(v)
	cx, cy, ok := CellOf(sv, sv.Rows()+1, sv.Cols(), p)
	return cx + sv.Coff(), cy + sv.Roff(), ok
}
