package engine

import (
	"strconv"

	"golang.org/x/text/width"
)

// GutterWidth returns the width of the line-number gutter for a buffer with the
// given line count, or 0 when numbering is off. It is shared by the engine's
// wrap/scroll math and the frontend so both agree on the text area width.
func GutterWidth(lineCount int64, number bool) int {
	if !number {
		return 0
	}
	digits := len(strconv.FormatInt(lineCount, 10))
	if digits < 5 {
		digits = 5
	}
	return digits + 1 // numbers right-aligned, then a space
}

// Display layout helpers. The engine is authoritative over how buffer runes map
// to display cells (tab expansion, control-character representation, rune width)
// so every frontend renders identically. These are pure functions with no
// terminal dependency; frontends call DisplayCells to turn a DisplayLine into a
// flat run of styled cells to paint.

// runeWidth returns the number of display columns rune r occupies when it
// begins at display column col, given the tabstop. Tabs advance to the next
// tabstop; control characters render as a two-cell ^X form; East Asian wide and
// fullwidth runes (e.g. 日) occupy two columns; other runes occupy one. This is
// the editor's wcwidth equivalent and is independent of the rune's UTF-8 byte
// length (the buffer already holds decoded runes).
func runeWidth(r rune, col, tabstop int) int {
	switch {
	case r == '\t':
		return tabstop - col%tabstop
	case r < 0x20:
		return 2 // ^A .. ^Z etc.
	case r == 0x7f:
		return 2 // ^?
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	}
	return 1
}

// makeDisplayLine builds a DisplayLine from logical buffer runes, computing the
// per-rune display width using the given tabstop. Text aliases the caller's
// slice and must be treated as read-only by the frontend.
func makeDisplayLine(runes []rune, tabstop int) DisplayLine {
	widths := make([]int8, len(runes))
	col := 0
	for i, r := range runes {
		w := runeWidth(r, col, tabstop)
		widths[i] = int8(w)
		col += w
	}
	return DisplayLine{Text: runes, Widths: widths}
}

// Cell is one display cell produced from a DisplayLine: the glyph to paint and
// its style. Tabs and control characters expand into multiple Cells.
type Cell struct {
	Rune  rune
	Style Style
}

// DisplayCells expands a DisplayLine into the flat sequence of cells a frontend
// paints left to right, applying tab expansion and control-character
// representation. It is the single shared renderer used by all frontends.
func DisplayCells(dl DisplayLine) []Cell {
	cells := make([]Cell, 0, len(dl.Text))
	col := 0
	for i, r := range dl.Text {
		st := styleAt(dl.Spans, i)
		switch {
		case r == '\t':
			w := int(dl.Widths[i])
			for k := 0; k < w; k++ {
				cells = append(cells, Cell{Rune: ' ', Style: st})
			}
			col += w
		case r < 0x20:
			cells = append(cells, Cell{Rune: '^', Style: st})
			cells = append(cells, Cell{Rune: r + 0x40, Style: st})
			col += 2
		case r == 0x7f:
			cells = append(cells, Cell{Rune: '^', Style: st})
			cells = append(cells, Cell{Rune: '?', Style: st})
			col += 2
		default:
			// A rune may occupy more than one column (East Asian wide). Emit the
			// glyph followed by continuation cells (Rune == 0) so the flat cell
			// list length equals the line's display width.
			w := int(dl.Widths[i])
			if w < 1 {
				w = 1
			}
			cells = append(cells, Cell{Rune: r, Style: st})
			for k := 1; k < w; k++ {
				cells = append(cells, Cell{Rune: 0, Style: st})
			}
			col += w
		}
	}
	return cells
}

// DisplayColumn returns the display column (0-based) at which logical rune
// index col begins, accounting for tab/control expansion. A col at or past the
// end of the line returns the column just past the last cell.
func DisplayColumn(dl DisplayLine, col int) int {
	if col < 0 {
		return 0
	}
	out := 0
	for i := 0; i < col && i < len(dl.Widths); i++ {
		out += int(dl.Widths[i])
	}
	return out
}

func styleAt(spans []Span, i int) Style {
	for _, s := range spans {
		if i >= s.Start && i < s.End {
			return s.Style
		}
	}
	return StyleNormal
}
