package grid

import "govi/engine"

// Cell is a screen cell coordinate (column x, row y) in the composed grid.
type Cell struct {
	X, Y int
}

// ScreenSelection is a screen-rectangle selection stored by two corners (either
// order). ApplyScreenSel and ScreenRangeText normalize it to a half-open
// [minX, maxX] x [minY, maxY] inclusive rectangle of cells.
type ScreenSelection struct {
	A, B Cell
}

func (s ScreenSelection) norm() (x1, y1, x2, y2 int) {
	x1, y1 = s.A.X, s.A.Y
	x2, y2 = s.B.X, s.B.Y
	if x2 < x1 {
		x1, x2 = x2, x1
	}
	if y2 < y1 {
		y1, y2 = y2, y1
	}
	return x1, y1, x2, y2
}

// ordered returns the selection corners in reading order (top-to-bottom, left-to-right).
func (s ScreenSelection) ordered() (start, end Cell) {
	if s.A.Y < s.B.Y || (s.A.Y == s.B.Y && s.A.X <= s.B.X) {
		return s.A, s.B
	}
	return s.B, s.A
}

func forEachLinearCell(g Grid, sel ScreenSelection, fn func(x, y int)) {
	start, end := sel.ordered()
	for y := start.Y; y <= end.Y; y++ {
		if y < 0 || y >= g.Rows {
			continue
		}
		x0, x1 := 0, g.Cols-1
		if y == start.Y {
			x0 = start.X
		}
		if y == end.Y {
			x1 = end.X
		}
		for x := x0; x <= x1; x++ {
			if x >= 0 && x < g.Cols {
				fn(x, y)
			}
		}
	}
}

// ApplyScreenLinearSel paints StyleReverse on cells from sel.A through sel.B in
// reading order (terminal-style), not as an axis-aligned rectangle.
func ApplyScreenLinearSel(g *Grid, sel *ScreenSelection) {
	if sel == nil {
		return
	}
	forEachLinearCell(*g, *sel, func(x, y int) {
		i := y*g.Cols + x
		g.Cells[i].Style |= engine.StyleReverse
	})
}

// ScreenLinearRangeText returns text from sel.A through sel.B in reading order.
func ScreenLinearRangeText(g Grid, sel ScreenSelection) string {
	start, end := sel.ordered()
	var rows []string
	for y := start.Y; y <= end.Y; y++ {
		if y < 0 || y >= g.Rows {
			continue
		}
		x0, x1 := 0, g.Cols-1
		if y == start.Y {
			x0 = start.X
		}
		if y == end.Y {
			x1 = end.X
		}
		last := -1
		runes := make([]rune, x1-x0+1)
		for x := x0; x <= x1; x++ {
			r := ' '
			if x >= 0 && x < g.Cols {
				r = g.At(x, y).Rune
				if r == 0 {
					r = ' '
				} else {
					last = x - x0
				}
			}
			runes[x-x0] = r
		}
		if last < 0 {
			rows = append(rows, "")
		} else {
			rows = append(rows, string(runes[:last+1]))
		}
	}
	if len(rows) == 0 {
		return ""
	}
	out := rows[0]
	for i := 1; i < len(rows); i++ {
		out += "\n" + rows[i]
	}
	return out
}

// ApplyScreenSel paints StyleReverse on every cell in sel.
func ApplyScreenSel(g *Grid, sel *ScreenSelection) {
	if sel == nil {
		return
	}
	x1, y1, x2, y2 := sel.norm()
	for y := y1; y <= y2; y++ {
		for x := x1; x <= x2; x++ {
			if x < 0 || y < 0 || x >= g.Cols || y >= g.Rows {
				continue
			}
			i := y*g.Cols + x
			g.Cells[i].Style |= engine.StyleReverse
		}
	}
}

// ScreenRangeText returns the text in sel from a composed grid: one row per
// screen line, cells joined left-to-right, rows separated by '\n', trailing
// blanks on each row trimmed (matching GoviRowText).
func ScreenRangeText(g Grid, sel ScreenSelection) string {
	x1, y1, x2, y2 := sel.norm()
	var rows []string
	for y := y1; y <= y2; y++ {
		if y < 0 || y >= g.Rows {
			continue
		}
		last := -1
		runes := make([]rune, x2-x1+1)
		for x := x1; x <= x2; x++ {
			r := ' '
			if x >= 0 && x < g.Cols {
				r = g.At(x, y).Rune
				if r == 0 {
					r = ' '
				} else {
					last = x - x1
				}
			}
			runes[x-x1] = r
		}
		if last < 0 {
			rows = append(rows, "")
		} else {
			rows = append(rows, string(runes[:last+1]))
		}
	}
	if len(rows) == 0 {
		return ""
	}
	out := rows[0]
	for i := 1; i < len(rows); i++ {
		out += "\n" + rows[i]
	}
	return out
}

type editorRowKind int

const (
	rowBuffer editorRowKind = iota
	rowTilde
	rowBlank
	rowStatus
)

// ScreenToBuffer maps a screen cell to the buffer caret it sits on in editor
// (vi) mode. ok is false for gutter cells, ~ filler, blank filler, the status
// row, and all non-editor layouts (overlay, ex mode).
func ScreenToBuffer(v engine.View, rows, cols, x, y int) (engine.Pos, bool) {
	if v.PendingOutput() != nil || v.Mode() == engine.ModeExText {
		return engine.Pos{}, false
	}
	tr := textRows(rows)
	if y < 0 || y >= tr {
		return engine.Pos{}, false
	}
	gutter := engine.GutterWidth(v.LineCount(), v.Number())
	if x < gutter {
		return engine.Pos{}, false
	}
	switch editorRowKindAt(v, rows, cols, y) {
	case rowBuffer:
		return Locate(v, rows, cols, x, y), true
	default:
		return engine.Pos{}, false
	}
}

func editorRowKindAt(v engine.View, rows, cols, y int) editorRowKind {
	tr := textRows(rows)
	if y >= tr {
		return rowStatus
	}
	vp := v.Viewport()
	mapRows := vp.MapRows
	if mapRows <= 0 || mapRows > tr {
		mapRows = tr
	}
	gutter := engine.GutterWidth(v.LineCount(), v.Number())
	textW := cols - gutter
	if textW < 1 {
		textW = 1
	}

	row := 0
	lno := vp.Top
	for row < mapRows && lno <= v.LineCount() {
		segs := wrapRowsOf(v.Line(lno), textW)
		for seg := 0; seg < segs; seg++ {
			if row == y {
				return rowBuffer
			}
			row++
			if row >= tr {
				return rowStatus
			}
		}
		lno++
	}
	for ; row < mapRows; row++ {
		if row == y {
			return rowTilde
		}
	}
	for ; row < tr; row++ {
		if row == y {
			return rowBlank
		}
	}
	return rowStatus
}
