package engine

import "strings"

// GUI editing primitives. These back graphical-host affordances (mouse
// click-to-position, selection copy/cut, and replacing a selection by typing or
// pasting) that have no vi command equivalent. They are not used by the
// terminal frontend. Each one produces a proper undo unit and keeps the cursor
// clamped and scrolled into view, matching the engine's own editing paths.
//
// Positions here are carets: Pos.Col is a rune index in 0..len(line), so a
// range [a, b) is half-open. The grid frontend maps screen cells to these
// caret positions (see frontend/grid.Locate).

// WordBoundaryFunc, given a line's runes and a rune index col within it,
// returns the half-open range [start, end) that a double-click should select.
// start == end means "no word here" (e.g. an empty line), in which case the
// host selects nothing.
//
// This is the single extension point for what constitutes a "word". The default
// (DefaultWordBoundary) groups runes by vi's classes (identifier runes vs.
// punctuation vs. blanks). A host can install a richer rule via SetWordBoundary
// -- for example a language-aware tokenizer, or one that treats '-' as a word
// character -- without touching the rest of the editor.
type WordBoundaryFunc func(line []rune, col int) (start, end int)

// SetWordBoundary installs the function used for double-click word selection.
// Passing nil restores the default.
func (e *Engine) SetWordBoundary(fn WordBoundaryFunc) {
	if fn == nil {
		fn = DefaultWordBoundary
	}
	e.wordBoundary = fn
}

// DefaultWordBoundary selects the maximal run of same-class runes around col,
// where the classes are: identifier runes (letters, digits, underscore),
// blanks (space/tab), and any other run of punctuation. This matches vi's word
// notion and the common editor double-click behavior.
func DefaultWordBoundary(line []rune, col int) (int, int) {
	n := len(line)
	if n == 0 {
		return 0, 0
	}
	if col >= n {
		col = n - 1
	}
	if col < 0 {
		col = 0
	}
	cls := clickClass(line[col])
	start, end := col, col+1
	for start > 0 && clickClass(line[start-1]) == cls {
		start--
	}
	for end < n && clickClass(line[end]) == cls {
		end++
	}
	return start, end
}

func clickClass(r rune) int {
	switch {
	case r == ' ' || r == '\t':
		return clBlank
	case isWordRune(r):
		return clWord
	default:
		return clPunct
	}
}

// WordRange returns the caret range a double-click at (line, col) selects, using
// the engine's word-boundary function.
func (e *Engine) WordRange(line int64, col int) (Pos, Pos) {
	s := e.scr
	line = clampLine(s, line)
	runes := s.lineRunes(line)
	start, end := e.wordBoundary(runes, col)
	return Pos{Line: line, Col: start}, Pos{Line: line, Col: end}
}

// LineSelectRange returns the caret range a triple-click on line selects: the
// whole logical line including its trailing newline (so a copy round-trips a
// full line). On the last line, which has no following line, it ends at the
// line's end instead.
func (e *Engine) LineSelectRange(line int64) (Pos, Pos) {
	s := e.scr
	n := s.store.Lines()
	line = clampLine(s, line)
	if line < n {
		return Pos{Line: line, Col: 0}, Pos{Line: line + 1, Col: 0}
	}
	return Pos{Line: line, Col: 0}, Pos{Line: line, Col: len(s.lineRunes(line))}
}

// MoveCursorTo positions the cursor at line/col, clamping into the buffer, and
// scrolls it into view. Backs click-to-position.
func (e *Engine) MoveCursorTo(line int64, col int) {
	s := e.scr
	if line < 1 {
		line = 1
	}
	if n := s.store.Lines(); line > n {
		line = n
	}
	s.cursor = Pos{Line: line, Col: col}
	s.clampCursor()
	s.scrollToCursor()
}

// RangeText returns the text in the half-open range [a, b) with embedded
// newlines between lines. Backs copy/cut to the host clipboard.
func (e *Engine) RangeText(a, b Pos) string {
	a, b = orderPos(a, b)
	txt := e.collectChars(a, b)
	parts := make([]string, len(txt.Lines))
	for i, ln := range txt.Lines {
		parts[i] = string(ln)
	}
	return strings.Join(parts, "\n")
}

// DeleteRange deletes [a, b) as one undo unit, leaving the cursor at a. Backs
// cut and deleting a selection with Backspace/Delete.
func (e *Engine) DeleteRange(a, b Pos) {
	a, b = orderPos(a, b)
	e.beginChange()
	e.deleteChars(a, b)
	e.endChange()
	e.scr.cursor = a
	e.scr.clampCursor()
	e.scr.scrollToCursor()
}

// ReplaceSelectionType deletes [a, b) and enters insert mode at a with text as
// the first inserted run, so the user keeps typing (GUI replace-on-type). The
// deletion and insertion are one undo unit, closed when insert mode ends (ESC).
func (e *Engine) ReplaceSelectionType(a, b Pos, text string) {
	a, b = orderPos(a, b)
	e.beginChange()
	e.deleteChars(a, b)
	e.scr.cursor = a
	e.vi.startInsert(e, a, false, 'c')
	for _, r := range text {
		e.vi.insertRune(e, r)
		e.vi.insertText = append(e.vi.insertText, r)
	}
	e.scr.scrollToCursor()
}

// ReplaceSelectionText deletes [a, b), inserts text at a as one undo unit, and
// stays in command mode (GUI paste over a selection).
func (e *Engine) ReplaceSelectionText(a, b Pos, text string) {
	a, b = orderPos(a, b)
	e.beginChange()
	e.deleteChars(a, b)
	e.insertPlainText(a, text)
	e.endChange()
	e.scr.clampCursor()
	e.scr.scrollToCursor()
}

// InsertText inserts text at the cursor caret as one undo unit (GUI paste with
// no selection).
func (e *Engine) InsertText(text string) {
	s := e.scr
	at := Pos{Line: s.cursor.Line, Col: s.cursor.Col}
	e.beginChange()
	e.insertPlainText(at, text)
	e.endChange()
	s.clampCursor()
	s.scrollToCursor()
}

// insertPlainText inserts text (which may contain newlines) at caret position
// at, leaving the cursor just past the inserted text. The caller owns the
// change bracket.
func (e *Engine) insertPlainText(at Pos, text string) {
	s := e.scr
	parts := strings.Split(text, "\n")
	cur := s.lineRunes(at.Line)
	col := clampIdx(at.Col, len(cur))
	head, tail := cloneR(cur[:col]), cloneR(cur[col:])

	if len(parts) == 1 {
		nl := append(append(head, []rune(parts[0])...), tail...)
		s.setLine(at.Line, nl)
		s.cursor = Pos{Line: at.Line, Col: col + len([]rune(parts[0]))}
		return
	}

	s.setLine(at.Line, append(head, []rune(parts[0])...))
	insLine := at.Line
	for i := 1; i < len(parts)-1; i++ {
		s.appendLine(insLine, []rune(parts[i]))
		insLine++
	}
	lastR := []rune(parts[len(parts)-1])
	s.appendLine(insLine, append(cloneR(lastR), tail...))
	s.cursor = Pos{Line: insLine + 1, Col: len(lastR)}
}
