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

	if out := v.PendingOutput(); out != nil {
		g.composeOverlay(out, v.PendingOutputPrompt())
		return g
	}
	if v.Mode() == engine.ModeExText {
		g.composeExMode(v)
		return g
	}
	g.composeEditor(v, sel)
	return g
}

// composeOverlay shows one page of command output with a continue prompt on the
// bottom row; the cursor is hidden.
func (g *Grid) composeOverlay(lines []string, prompt string) {
	avail := g.Rows - 1
	y := 0
	for i := 0; i < len(lines) && y < avail; i++ {
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
	g.CursorX = len([]rune(prompt))
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
		first := true
		for i := 0; (i < len(cells) || first) && row < mapRows; i += textW {
			if gutter > 0 && first {
				g.drawGutter(lno, row, gutter)
			}
			x := gutter
			for j := i; j < i+textW && j < len(cells); j++ {
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

	g.placeCursor(v, mapRows, rows, gutter, textW)
}

func (g *Grid) drawGutter(lno int64, row, gutter int) {
	label := strconv.FormatInt(lno, 10)
	pad := gutter - 1 - len(label)
	x := 0
	for ; x < pad; x++ {
		g.set(x, row, ' ', engine.StyleNormal)
	}
	for _, r := range label {
		g.set(x, row, r, engine.StyleNormal)
		x++
	}
	g.set(x, row, ' ', engine.StyleNormal)
}

func (g *Grid) placeCursor(v engine.View, mapRows, statusRow, gutter, textW int) {
	if v.Mode() == engine.ModeExColon {
		msg, _ := v.Message()
		g.CursorX = len([]rune(msg))
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
		y += wrapRowsOf(v.Line(ln), textW)
	}
	dx := engine.DisplayColumn(v.Line(cur.Line), cur.Col)
	y += dx / textW
	x := gutter + dx%textW

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
	textW := cols - gutter
	if textW < 1 {
		textW = 1
	}

	row := 0
	lno := v.Viewport().Top
	for lno <= v.LineCount() {
		dl := v.Line(lno)
		segs := wrapRowsOf(dl, textW)
		for seg := 0; seg < segs; seg++ {
			if row == y {
				dcol := seg*textW + (x - gutter)
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
	textW := cols - gutter
	if textW < 1 {
		textW = 1
	}
	top := v.Viewport().Top
	if p.Line < top {
		return 0, 0, false
	}
	row := 0
	for lno := top; lno <= v.LineCount(); lno++ {
		dl := v.Line(lno)
		if lno == p.Line {
			dcol := engine.DisplayColumn(dl, p.Col)
			yy := row + dcol/textW
			if yy < 0 || yy >= tr {
				return 0, 0, false
			}
			return gutter + dcol%textW, yy, true
		}
		row += wrapRowsOf(dl, textW)
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

// wrapRowsOf returns how many screen rows a display line occupies at textW.
func wrapRowsOf(dl engine.DisplayLine, textW int) int {
	w := engine.DisplayLineWidth(dl)
	if w <= 0 {
		return 1
	}
	if textW < 1 {
		textW = 1
	}
	return (w + textW - 1) / textW
}
