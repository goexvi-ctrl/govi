package engine

import (
	"fmt"
	"strconv"
)

// insertShowMode returns the showmode label for an insert/replace session (nvi
// SM_APPEND / SM_CHANGE / SM_INSERT / SM_REPLACE).
func insertShowMode(cmd rune, replace bool) string {
	if replace {
		return "Replace"
	}
	switch cmd {
	case 'a', 'A':
		return "Append"
	case 'c', 'C', 's', 'S':
		return "Change"
	default:
		return "Insert"
	}
}

// statusLine builds the default vi status line when no transient message is
// active (nvi vs_modeline): optional ruler centered on the line and showmode
// (with a leading * when the buffer is modified) near the right edge.
func (v view) statusLine() string {
	o := &v.s.opts
	if !o.Bool("ruler") && !o.Bool("showmode") {
		return ""
	}
	cols := v.s.cols
	if cols < 1 {
		cols = 80
	}
	max := cols - 1 // nvi leaves the last column blank

	ruler := ""
	if o.Bool("ruler") {
		dl := v.Line(v.s.cursor.Line)
		col := DisplayColumn(dl, v.s.cursor.Col) + 1
		ruler = strconv.FormatInt(v.s.cursor.Line, 10) + "," + strconv.Itoa(col)
	}

	suffix := ""
	if o.Bool("showmode") {
		if v.s.dirty() {
			suffix = "*"
		}
		suffix += v.s.showModeLabel
	}

	return layoutStatusLine(max, ruler, suffix)
}

// layoutStatusLine places ruler and suffix on a fixed-width status row. max is
// the number of character cells available (cols-1).
func layoutStatusLine(max int, ruler, suffix string) string {
	if max < 1 {
		return ""
	}
	buf := make([]byte, max)
	for i := range buf {
		buf[i] = ' '
	}

	leftLen := 0
	if ruler != "" {
		midpoint := (max - (len(ruler)+1)/2) / 2
		pos := -1
		if leftLen < midpoint && midpoint+len(ruler) <= max {
			pos = midpoint
		} else if leftLen+2+len(ruler) <= max {
			pos = leftLen + 2
		}
		if pos >= 0 {
			copy(buf[pos:], ruler)
			leftLen = pos + len(ruler)
		}
	}

	if suffix != "" {
		endpoint := max - len(suffix)
		if endpoint >= leftLen+2 {
			copy(buf[endpoint:], suffix)
		}
	}
	return string(buf)
}

// rulerText formats line,col for tests and callers that need only the ruler
// portion.
func rulerText(line int64, col int) string {
	return fmt.Sprintf("%d,%d", line, col)
}
