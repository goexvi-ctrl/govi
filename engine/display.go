package engine

// Display layout helpers. The engine is authoritative over how buffer runes map
// to display cells (tab expansion, control-character representation, rune width)
// so every frontend renders identically. These are pure functions with no
// terminal dependency; frontends call DisplayCells to turn a DisplayLine into a
// flat run of styled cells to paint.

// defaultTabstop is the column width of a tab until the options subsystem
// (Phase 6) supplies the 'tabstop' setting.
const defaultTabstop = 8

// runeWidth returns the number of display columns rune r occupies when it
// begins at display column col. Tabs advance to the next tabstop; control
// characters render as a two-cell ^X form; other runes occupy one column.
//
// TODO(phase6+): East Asian wide runes (width 2) and the 'tabstop' option.
func runeWidth(r rune, col int) int {
	switch {
	case r == '\t':
		return defaultTabstop - col%defaultTabstop
	case r < 0x20:
		return 2 // ^A .. ^Z etc.
	case r == 0x7f:
		return 2 // ^?
	default:
		return 1
	}
}

// makeDisplayLine builds a DisplayLine from logical buffer runes, computing the
// per-rune display width. Text aliases the caller's slice and must be treated
// as read-only by the frontend.
func makeDisplayLine(runes []rune) DisplayLine {
	widths := make([]int8, len(runes))
	col := 0
	for i, r := range runes {
		w := runeWidth(r, col)
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
			cells = append(cells, Cell{Rune: r, Style: st})
			col++
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
