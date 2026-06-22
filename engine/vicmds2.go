package engine

import (
	"fmt"
	"strconv"
)

// editIncrement implements the # command: find the number at or after the
// cursor on the current line and add delta*count to it.
func (e *Engine) editIncrement(delta int64, count int) error {
	s := e.scr
	line := s.lineRunes(s.cursor.Line)
	col := s.cursor.Col

	i := col
	for i < len(line) && !(line[i] >= '0' && line[i] <= '9') {
		i++
	}
	if i >= len(line) {
		return fmt.Errorf("Cursor not in a number")
	}
	start := i
	for start > 0 && line[start-1] >= '0' && line[start-1] <= '9' {
		start--
	}
	if start > 0 && line[start-1] == '-' {
		start--
	}
	end := i
	for end < len(line) && line[end] >= '0' && line[end] <= '9' {
		end++
	}
	val, err := strconv.ParseInt(string(line[start:end]), 10, 64)
	if err != nil {
		return fmt.Errorf("Number too large")
	}
	val += delta * int64(count)
	newStr := strconv.FormatInt(val, 10)

	e.beginChange()
	nl := append(append(cloneR(line[:start]), []rune(newStr)...), line[end:]...)
	s.setLine(s.cursor.Line, nl)
	s.cursor.Col = start + len(newStr) - 1
	e.endChange()
	return nil
}

// editAlternate implements ^^: switch to the alternate (previously edited) file.
func (e *Engine) editAlternate() error {
	if e.altFile == "" {
		return fmt.Errorf("No alternate file")
	}
	if e.scr.modified {
		return fmt.Errorf("No write since last change")
	}
	return e.Open(e.altFile)
}
