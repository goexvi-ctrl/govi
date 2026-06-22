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

// Compose lays out view v into a rows x cols grid. rows is the full height
// including the status row.
func Compose(v engine.View, rows, cols int) Grid {
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	g := Grid{Rows: rows, Cols: cols, Cells: make([]Glyph, rows*cols)}

	if out := v.PendingOutput(); out != nil {
		g.composeOverlay(out)
		return g
	}
	if v.Mode() == engine.ModeExText {
		g.composeExMode(v)
		return g
	}
	g.composeEditor(v)
	return g
}

// composeOverlay shows multi-line command output (e.g. :set all) with a
// continue prompt on the bottom row; the cursor is hidden.
func (g *Grid) composeOverlay(lines []string) {
	avail := g.Rows - 1
	start := 0
	if len(lines) > avail {
		start = len(lines) - avail
	}
	y := 0
	for _, line := range lines[start:] {
		g.drawText(line, y)
		y++
	}
	g.drawText("[Press any key to continue]", g.Rows-1)
	g.CursorVisible = false
}

// composeExMode draws the ex-mode scrolling transcript with the prompt on the
// bottom line and the cursor at the end of the prompt.
func (g *Grid) composeExMode(v engine.View) {
	prompt, _ := v.Message()
	transcript := v.ExTranscript()
	avail := g.Rows - 1
	start := 0
	if len(transcript) > avail {
		start = len(transcript) - avail
	}
	y := 0
	for _, line := range transcript[start:] {
		g.drawText(line, y)
		y++
	}
	g.drawText(prompt, g.Rows-1)
	g.CursorX = len([]rune(prompt))
	g.CursorY = g.Rows - 1
	g.CursorVisible = true
}

// composeEditor draws the wrapped buffer text, gutter, tilde filler, status
// line, and positions the cursor.
func (g *Grid) composeEditor(v engine.View) {
	rows := textRows(g.Rows)
	top := v.Viewport().Top
	gutter := engine.GutterWidth(v.LineCount(), v.Number())
	textW := g.Cols - gutter
	if textW < 1 {
		textW = 1
	}

	row := 0
	lno := top
	for row < rows && lno <= v.LineCount() {
		cells := engine.DisplayCells(v.Line(lno))
		first := true
		for i := 0; (i < len(cells) || first) && row < rows; i += textW {
			if gutter > 0 && first {
				g.drawGutter(lno, row, gutter)
			}
			x := gutter
			for j := i; j < i+textW && j < len(cells); j++ {
				// Continuation cells (Rune == 0) follow a wide glyph; the wide
				// rune is drawn at the leading cell, so leave these blank but keep
				// the column advancing so wrap math stays aligned.
				if cells[j].Rune != 0 {
					g.set(x, row, cells[j].Rune, cells[j].Style)
				}
				x++
			}
			row++
			first = false
		}
		lno++
	}
	// Tilde filler for rows past the end of the buffer.
	for ; row < rows; row++ {
		g.set(0, row, '~', engine.StyleNormal)
	}

	msg, _ := v.Message()
	g.drawText(msg, rows)

	g.placeCursor(v, rows, gutter, textW)
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

func (g *Grid) placeCursor(v engine.View, rows, gutter, textW int) {
	if v.Mode() == engine.ModeExColon {
		msg, _ := v.Message()
		g.CursorX = len([]rune(msg))
		g.CursorY = rows
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

	if y < 0 || y >= rows {
		g.CursorVisible = false
		return
	}
	g.CursorX = x
	g.CursorY = y
	g.CursorVisible = true
}

// wrapRowsOf returns how many screen rows a display line occupies at textW.
func wrapRowsOf(dl engine.DisplayLine, textW int) int {
	w := 0
	for _, n := range dl.Widths {
		w += int(n)
	}
	if w <= 0 {
		return 1
	}
	if textW < 1 {
		textW = 1
	}
	return (w + textW - 1) / textW
}
