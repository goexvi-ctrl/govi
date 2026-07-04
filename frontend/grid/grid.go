// Package grid composes the engine's semantic View into a flat character grid:
// a rectangle of styled glyph cells plus a cursor position. It is the layout
// step shared by non-cell-addressable frontends (a GUI canvas, a golden
// renderer) that, unlike a terminal, must be handed an already-laid-out screen.
//
// All vi presentation logic — line wrapping, the line-number gutter, tilde
// filler, the status line, ex-mode transcript, pending-output overlays, and
// cursor placement (including the showmatch flash) — lives here, reusing the
// engine's display helpers so a GUI renders identically to the tcell terminal
// frontend. The grid has no terminal or GUI dependency.
package grid

import (
	"strconv"

	"govi/engine"
)

// Glyph is one cell of the composed screen: the rune to paint and its style. A
// zero Rune is an empty cell (painted as background).
type Glyph struct {
	Rune  rune
	Style engine.Style
}

// Grid is a Rows x Cols rectangle of glyphs in row-major order, with the cursor
// the frontend should draw. Rows includes the bottom status row.
type Grid struct {
	Rows, Cols    int
	Cells         []Glyph
	CursorX       int
	CursorY       int
	CursorVisible bool
}

// At returns the glyph at (x, y); out-of-range coordinates return an empty cell.
func (g *Grid) At(x, y int) Glyph {
	if x < 0 || y < 0 || x >= g.Cols || y >= g.Rows {
		return Glyph{}
	}
	return g.Cells[y*g.Cols+x]
}

func (g *Grid) set(x, y int, r rune, st engine.Style) {
	if x < 0 || y < 0 || x >= g.Cols || y >= g.Rows {
		return
	}
	g.Cells[y*g.Cols+x] = Glyph{Rune: r, Style: st}
}

// drawText paints a plain string left-aligned on row y in the default style.
func (g *Grid) drawText(s string, y int) {
	x := 0
	for _, r := range s {
		if x >= g.Cols {
			break
		}
		g.set(x, y, r, engine.StyleNormal)
		x++
	}
}

// textRows is the number of buffer rows, reserving the bottom row for the
// status/message line (matching the terminal frontend's layout).
func textRows(rows int) int {
	if rows <= 1 {
		return 1
	}
	return rows - 1
}

// Selection is a half-open caret range [A, B) in buffer coordinates that the
// host wants drawn highlighted (reverse video). A and B may be given in either
// order.
type Selection struct {
	A, B engine.Pos
}

// Compose lays out view v into a rows x cols grid. rows is the full height
// including the status row.
func Compose(v engine.View, rows, cols int) Grid {
	return ComposeSel(v, rows, cols, nil)
}

// ComposeSel is Compose with an optional selection highlighted over the editor
// text.
func ComposeSel(v engine.View, rows, cols int, sel *Selection) Grid {
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	g := Grid{Rows: rows, Cols: cols, Cells: make([]Glyph, rows*cols)}

	if v.Mode() == engine.ModeExText {
		g.composeExMode(v)
		return g
	}
	if v.Split() {
		// Multiple screens: draw each pane in its own row/column band, every pane
		// carrying its own status-line divider, with the cursor in the active pane.
		// This mirrors the terminal frontend's paintScreen so a GUI split renders
		// identically. The buffer selection is in the active screen's coordinates,
		// so it is applied only to the active pane.
		for _, sv := range v.Screens() {
			var paneSel *Selection
			if sv.Active() {
				paneSel = sel
			}
			g.composeScreen(sv, cols, paneSel)
		}
	} else {
		g.composeEditor(v, sel)
	}
	// An ex-output overlay is drawn over the bottom of the buffer: a "+=+="
	// divider, the output lines, and a continue prompt on the last row, with the
	// buffer still visible above (nvi vs_msg).
	if out := v.PendingOutput(); out != nil {
		g.composeOverlay(out, v.PendingOutputPrompt(), v.PendingOutputFirst())
	}
	return g
}

// composeScreen draws one split pane (engine.ScreenView) into its band of the
// grid: the text rows at [roff, roff+rows), the line-number gutter, tilde and
// blank filler, the per-screen status/modeline divider at roff+rows (reverse
// video, like nvi's vs_modeline), the vertical-split '|' divider in the
// sacrificed column to its right, and -- for the active pane -- the cursor. It
// mirrors the terminal frontend's paintScreen so a GUI split renders the same.
// sel, if non-nil, highlights a buffer selection on this pane.
func (g *Grid) composeScreen(sv engine.ScreenView, termW int, sel *Selection) {
	roff := sv.Roff()
	coff := sv.Coff()
	rows := sv.Rows()
	cols := sv.Cols()
	statusRow := roff + rows

	gutter := engine.GutterWidth(sv.LineCount(), sv.Number())
	textW := cols - gutter
	if textW < 1 {
		textW = 1
	}
	vp := sv.Viewport()
	top := vp.Top
	mapRows := vp.MapRows
	if mapRows <= 0 || mapRows > rows {
		mapRows = rows
	}

	row := 0
	lno := top
	for row < mapRows && lno <= sv.LineCount() {
		dl := sv.Line(lno)
		cells := engine.DisplayCells(dl)
		ds, de := selSpan(dl, lno, sel) // selected display-column interval
		// The gutter occupies the first row only; continuation rows start at
		// column 0 and span the full width (nvi draws the number once).
		i, first := 0, true
		for (i < len(cells) || first) && row < mapRows {
			w, x := textW, coff+gutter
			if !first {
				w, x = cols, coff
			}
			if gutter > 0 && first {
				g.drawGutter(lno, coff, roff+row, gutter)
			}
			for j := i; j < i+w && j < len(cells); j++ {
				st := cells[j].Style
				if j >= ds && j < de {
					st |= engine.StyleReverse
				}
				// Continuation cells (Rune == 0) follow a wide glyph; leave them
				// blank but advance the column so wrap math stays aligned.
				if cells[j].Rune != 0 {
					g.set(x, roff+row, cells[j].Rune, st)
				} else if st&engine.StyleReverse != 0 {
					g.set(x, roff+row, ' ', st)
				}
				x++
			}
			i += w
			row++
			first = false
		}
		lno++
	}
	// Tilde filler for map rows past the end of the buffer.
	for ; row < mapRows; row++ {
		g.set(coff, roff+row, '~', engine.StyleNormal)
	}
	// Blank filler below a reduced z[count] map.
	for ; row < rows; row++ {
		for x := 0; x < cols; x++ {
			g.set(coff+x, roff+row, ' ', engine.StyleNormal)
		}
	}

	g.drawStatus(sv, coff, statusRow, cols)

	// Vertical-split divider: a '|' in the sacrificed column to the right of this
	// pane, on the text rows only (nvi vs_vsplit). A full-width screen ends at the
	// terminal edge and gets none.
	if coff+cols < termW {
		for r := roff; r < statusRow; r++ {
			g.set(coff+cols, r, '|', engine.StyleNormal)
		}
	}

	if sv.Active() {
		g.placeCursorPane(sv, coff, roff, statusRow, gutter, cols, mapRows)
	}
}

// drawStatus draws a split pane's status/colon/message line at (coff, row), cols
// wide, as the inter-screen divider: the text is drawn in reverse video while
// the trailing pad stays normal, matching nvi's vs_modeline (standout text +
// clrtoeol). The blank divider column between vertically split screens is left
// untouched.
func (g *Grid) drawStatus(sv engine.ScreenView, coff, row, cols int) {
	msg, _ := sv.Message()
	rs := []rune(msg)
	for x := 0; x < cols; x++ {
		st := engine.StyleNormal
		r := ' '
		if x < len(rs) {
			r = rs[x]
			st = engine.StyleReverse
		}
		g.set(coff+x, row, r, st)
	}
}

// placeCursorPane positions the cursor for the active split pane, offsetting the
// single-pane placeCursor math by the pane's row/column origin.
func (g *Grid) placeCursorPane(sv engine.ScreenView, coff, roff, statusRow, gutter, cols, mapRows int) {
	if sv.Mode() == engine.ModeExColon {
		msg, _ := sv.Message()
		g.CursorX = coff + engine.DisplayStringColumns(msg, 8)
		g.CursorY = statusRow
		g.CursorVisible = true
		return
	}
	cur := sv.Cursor()
	if mp, ok := sv.MatchHighlight(); ok {
		cur = mp // showmatch: flash the cursor at the matching bracket
	}
	top := sv.Viewport().Top

	y := 0
	for ln := top; ln < cur.Line; ln++ {
		y += wrapRowsOf(sv.Line(ln), cols, gutter)
	}
	dx := engine.CursorDisplayColumn(sv.Line(cur.Line), cur.Col, sv.Mode())
	sub, sx := engine.WrapCellPos(dx, cols, gutter)
	y += sub
	x := coff + sx

	if y < 0 || y >= mapRows {
		g.CursorVisible = false
		return
	}
	g.CursorX = x
	g.CursorY = roff + y
	g.CursorVisible = true
}

// divideStr is nvi's ex-output divider (vs_divider DIVIDESTR).
const divideStr = "+=+=+=+=+=+=+=+"

// composeOverlay overlays one page of command output at the bottom: a divider
// (only where the output begins), the lines, then a continue prompt on the last
// row. The cursor is hidden.
func (g *Grid) composeOverlay(lines []string, prompt string, first bool) {
	n := len(lines)
	sep := 0
	if first {
		sep = 1
	}
	top := g.Rows - (n + sep + 1)
	if top < 0 {
		top = 0
	}
	for y := top; y < g.Rows; y++ {
		for x := 0; x < g.Cols; x++ {
			g.set(x, y, ' ', engine.StyleNormal) // clear the block row
		}
	}
	y := top
	if first {
		d := divideStr
		if len(d) > g.Cols {
			d = d[:g.Cols]
		}
		g.drawText(d, y)
		y++
	}
	for i := 0; i < n && y < g.Rows-1; i++ {
		g.drawText(lines[i], y)
		y++
	}
	if prompt == "" {
		prompt = "Press any key to continue [: to enter more ex commands]: "
	}
	g.drawText(prompt, g.Rows-1)
	g.CursorVisible = false
}

// composeExMode renders ex (Q) mode the way a GUI equivalent of nvi's line
// terminal looks: the whole window is a scrolling transcript that grows
// downward, with the current input line (the ":" prompt plus what is being
// typed) immediately after the output -- not pinned to the bottom. When the
// content overflows, the oldest lines scroll off the top, and the cursor sits on
// the input line just after the prompt.
func (g *Grid) composeExMode(v engine.View) {
	prompt, _ := v.Message()
	lines := append(append([]string{}, v.ExTranscript()...), prompt)

	start := 0
	if len(lines) > g.Rows {
		start = len(lines) - g.Rows // overflow: show the tail (scroll up)
	}
	y := 0
	for _, line := range lines[start:] {
		g.drawText(line, y)
		y++
	}
	g.CursorX = engine.DisplayStringColumns(prompt, 8)
	g.CursorY = y - 1 // the input line is the last one drawn
	g.CursorVisible = true
}

// composeEditor draws the wrapped buffer text, gutter, tilde filler, status
// line, and positions the cursor. sel, if non-nil, is drawn highlighted.
func (g *Grid) composeEditor(v engine.View, sel *Selection) {
	rows := textRows(g.Rows)
	vp := v.Viewport()
	top := vp.Top
	mapRows := vp.MapRows
	if mapRows <= 0 || mapRows > rows {
		mapRows = rows
	}
	gutter := engine.GutterWidth(v.LineCount(), v.Number())
	textW := g.Cols - gutter
	if textW < 1 {
		textW = 1
	}

	row := 0
	lno := top
	for row < mapRows && lno <= v.LineCount() {
		dl := v.Line(lno)
		cells := engine.DisplayCells(dl)
		ds, de := selSpan(dl, lno, sel) // selected display-column interval
		// The gutter occupies the first row only; continuation rows start at
		// column 0 and span the full width (nvi draws the number once).
		i, first := 0, true
		for (i < len(cells) || first) && row < mapRows {
			w, x := textW, gutter
			if !first {
				w, x = g.Cols, 0
			}
			if gutter > 0 && first {
				g.drawGutter(lno, 0, row, gutter)
			}
			for j := i; j < i+w && j < len(cells); j++ {
				st := cells[j].Style
				if j >= ds && j < de {
					st |= engine.StyleReverse
				}
				// Continuation cells (Rune == 0) follow a wide glyph; the wide
				// rune is drawn at the leading cell, so leave these blank but keep
				// the column advancing so wrap math stays aligned.
				if cells[j].Rune != 0 {
					g.set(x, row, cells[j].Rune, st)
				} else if st&engine.StyleReverse != 0 {
					g.set(x, row, ' ', st)
				}
				x++
			}
			i += w
			row++
			first = false
		}
		lno++
	}
	// Tilde filler for map rows past the end of the buffer.
	for ; row < mapRows; row++ {
		g.set(0, row, '~', engine.StyleNormal)
	}
	// Blank filler below a reduced z[count] map.
	for ; row < rows; row++ {
		for x := 0; x < g.Cols; x++ {
			g.set(x, row, ' ', engine.StyleNormal)
		}
	}

	msg, _ := v.Message()
	g.drawText(msg, rows)

	g.placeCursor(v, mapRows, rows, gutter, g.Cols)
}

func (g *Grid) drawGutter(lno int64, coff, row, gutter int) {
	label := strconv.FormatInt(lno, 10)
	pad := gutter - 1 - len(label)
	x := coff
	for i := 0; i < pad; i++ {
		g.set(x, row, ' ', engine.StyleNormal)
		x++
	}
	for _, r := range label {
		g.set(x, row, r, engine.StyleNormal)
		x++
	}
	g.set(x, row, ' ', engine.StyleNormal)
}

func (g *Grid) placeCursor(v engine.View, mapRows, statusRow, gutter, cols int) {
	if v.Mode() == engine.ModeExColon {
		msg, _ := v.Message()
		g.CursorX = engine.DisplayStringColumns(msg, 8)
		g.CursorY = statusRow
		g.CursorVisible = true
		return
	}
	cur := v.Cursor()
	if mp, ok := v.MatchHighlight(); ok {
		cur = mp // showmatch: flash the cursor at the matching bracket
	}
	top := v.Viewport().Top

	y := 0
	for ln := top; ln < cur.Line; ln++ {
		y += wrapRowsOf(v.Line(ln), cols, gutter)
	}
	dx := engine.CursorDisplayColumn(v.Line(cur.Line), cur.Col, v.Mode())
	sub, sx := engine.WrapCellPos(dx, cols, gutter)
	y += sub
	x := sx

	if y < 0 || y >= mapRows {
		g.CursorVisible = false
		return
	}
	g.CursorX = x
	g.CursorY = y
	g.CursorVisible = true
}

// selSpan returns the half-open display-column interval [ds, de) of sel on the
// logical line lno, or (0, 0) if the line is not within the selection.
func selSpan(dl engine.DisplayLine, lno int64, sel *Selection) (int, int) {
	if sel == nil {
		return 0, 0
	}
	a, b := sel.A, sel.B
	if a.Line > b.Line || (a.Line == b.Line && a.Col > b.Col) {
		a, b = b, a
	}
	if lno < a.Line || lno > b.Line {
		return 0, 0
	}
	startCol := 0
	if lno == a.Line {
		startCol = a.Col
	}
	endCol := len(dl.Text)
	if lno == b.Line {
		endCol = b.Col
	}
	ds := engine.DisplayColumn(dl, startCol)
	de := engine.DisplayColumn(dl, endCol)
	if de < ds {
		de = ds
	}
	return ds, de
}

// Locate maps a screen cell (x, y) to the buffer caret position it sits on,
// inverting the editor layout (gutter, wrapping, scroll). It is the counterpart
// to composeEditor used by a GUI to turn a mouse click into a caret. A click
// past the last line returns the end of the last line.
func Locate(v engine.View, rows, cols, x, y int) engine.Pos {
	tr := textRows(rows)
	if y < 0 {
		y = 0
	}
	if y >= tr {
		y = tr - 1
	}
	gutter := engine.GutterWidth(v.LineCount(), v.Number())

	row := 0
	lno := v.Viewport().Top
	for lno <= v.LineCount() {
		dl := v.Line(lno)
		segs := wrapRowsOf(dl, cols, gutter)
		for seg := 0; seg < segs; seg++ {
			if row == y {
				// Continuation rows carry no gutter: their cells map from
				// screen column 0 (nvi's once-per-line number layout).
				off := x
				if seg == 0 {
					off = x - gutter
				}
				dcol := engine.WrapRowStart(seg, cols, gutter) + off
				if dcol < 0 {
					dcol = 0
				}
				return engine.Pos{Line: lno, Col: caretRuneIndex(dl, dcol)}
			}
			row++
			if row >= tr {
				break
			}
		}
		if row >= tr {
			break
		}
		lno++
	}
	last := v.LineCount()
	return engine.Pos{Line: last, Col: len(v.Line(last).Text)}
}

// CellOf maps a buffer caret position to the screen cell it occupies, the
// inverse of Locate. visible reports whether the position is within the laid-out
// area; when false x and y are meaningless. A GUI uses this to place overlays
// (e.g. spelling squiggles) anchored to buffer positions.
func CellOf(v engine.View, rows, cols int, p engine.Pos) (x, y int, visible bool) {
	tr := textRows(rows)
	gutter := engine.GutterWidth(v.LineCount(), v.Number())
	top := v.Viewport().Top
	if p.Line < top {
		return 0, 0, false
	}
	row := 0
	for lno := top; lno <= v.LineCount(); lno++ {
		dl := v.Line(lno)
		if lno == p.Line {
			dcol := engine.DisplayColumn(dl, p.Col)
			sub, sx := engine.WrapCellPos(dcol, cols, gutter)
			yy := row + sub
			if yy < 0 || yy >= tr {
				return 0, 0, false
			}
			return sx, yy, true
		}
		row += wrapRowsOf(dl, cols, gutter)
		if row >= tr {
			return 0, 0, false
		}
	}
	return 0, 0, false
}

// caretRuneIndex converts a display column to the caret rune index on dl: the
// index of the rune whose cell contains dcol, or len(runes) at/after the end.
func caretRuneIndex(dl engine.DisplayLine, dcol int) int {
	if dcol <= 0 {
		return 0
	}
	acc := 0
	for i, w := range dl.Widths {
		if dcol < acc+int(w) {
			return i
		}
		acc += int(w)
	}
	return len(dl.Widths)
}

// wrapRowsOf returns how many screen rows a display line occupies at screen
// width cols with a number gutter of g columns (drawn on the first row only).
func wrapRowsOf(dl engine.DisplayLine, cols, g int) int {
	return engine.WrapRowCount(engine.DisplayLineWidth(dl), cols, g)
}
