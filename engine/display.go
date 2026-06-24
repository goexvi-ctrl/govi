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
// tabstop unless list mode is on, in which case they occupy two columns (^I).
// Control characters render as a two-cell ^X form; East Asian wide and
// fullwidth runes (e.g. 日) occupy two columns; other runes occupy one. This is
// the editor's wcwidth equivalent and is independent of the rune's UTF-8 byte
// length (the buffer already holds decoded runes).
func runeWidth(r rune, col, tabstop int, list bool) int {
	switch {
	case r == '\t':
		if list {
			return 2
		}
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

// DisplayStringColumns returns how many terminal columns string s occupies when
// drawn left-to-right with the given tabstop (control characters as ^X, wide
// runes as two columns).
func DisplayStringColumns(s string, tabstop int) int {
	col := 0
	for _, r := range s {
		col += runeWidth(r, col, tabstop, false)
	}
	return col
}

// makeDisplayLine builds a DisplayLine from logical buffer runes, computing the
// per-rune display width using the given tabstop and list option. Text aliases
// the caller's slice and must be treated as read-only by the frontend.
func makeDisplayLine(runes []rune, tabstop int, list bool) DisplayLine {
	widths := make([]int8, len(runes))
	col := 0
	for i, r := range runes {
		w := runeWidth(r, col, tabstop, list)
		widths[i] = int8(w)
		col += w
	}
	return DisplayLine{Text: runes, Widths: widths, List: list}
}

// DisplayLineWidth returns the total display width of dl, including the trailing
// $ when list mode is on.
func DisplayLineWidth(dl DisplayLine) int {
	w := 0
	for _, n := range dl.Widths {
		w += int(n)
	}
	if dl.List {
		w++
	}
	return w
}

// Cell is one display cell produced from a DisplayLine: the glyph to paint and
// its style. Tabs and control characters expand into multiple Cells.
type Cell struct {
	Rune  rune
	Style Style
}

// DisplayCells expands a DisplayLine into the flat sequence of cells a frontend
// paints left to right, applying tab expansion and control-character
// representation. List mode shows tabs as ^I and appends a trailing $.
func DisplayCells(dl DisplayLine) []Cell {
	cells := make([]Cell, 0, len(dl.Text)+1)
	col := 0
	for i, r := range dl.Text {
		st := styleAt(dl.Spans, i)
		switch {
		case r == '\t':
			if dl.List {
				cells = append(cells, Cell{Rune: '^', Style: st})
				cells = append(cells, Cell{Rune: 'I', Style: st})
				col += 2
			} else {
				w := int(dl.Widths[i])
				for k := 0; k < w; k++ {
					cells = append(cells, Cell{Rune: ' ', Style: st})
				}
				col += w
			}
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
	if dl.List {
		cells = append(cells, Cell{Rune: '$', Style: StyleNormal})
	}
	return cells
}

// FormatVisibleControls renders runes for on-screen display of the colon
// command line and similar single-line inputs: tabs and control characters
// appear in ^X form (nvi colon-line / :set list style, without the trailing $).
func FormatVisibleControls(runes []rune) string {
	out := make([]rune, 0, len(runes)*2)
	for _, r := range runes {
		switch {
		case r == '\t':
			out = append(out, '^', 'I')
		case r < 0x20:
			out = append(out, '^', r+0x40)
		case r == 0x7f:
			out = append(out, '^', '?')
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

// FormatListLine formats buffer runes for ex :list output (and :print/:number
// when the list option is set): tabs and control characters in ^X form, plus a
// trailing $.
func FormatListLine(runes []rune) string {
	if len(runes) == 0 {
		return "$"
	}
	return FormatVisibleControls(runes) + "$"
}

// DisplayColumn returns the display column (0-based) at which logical rune
// index col begins, accounting for tab/control expansion. A col at or past the
// end of the line returns the column just past the last cell (not including the
// list-mode trailing $).
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

// CursorDisplayColumn returns where the cursor should appear for logical rune
// index col. In command mode the cursor sits on the last display cell of the
// character (nvi vs_line.c: scno-1); in insert/replace it sits on the first
// (scno-chlen).
func CursorDisplayColumn(dl DisplayLine, col int, mode Mode) int {
	start := DisplayColumn(dl, col)
	if col < 0 || col >= len(dl.Widths) {
		return start
	}
	w := int(dl.Widths[col])
	if w < 1 {
		w = 1
	}
	if mode == ModeInsert || mode == ModeReplace {
		return start
	}
	return start + w - 1
}

func styleAt(spans []Span, i int) Style {
	for _, s := range spans {
		if i >= s.Start && i < s.End {
			return s.Style
		}
	}
	return StyleNormal
}