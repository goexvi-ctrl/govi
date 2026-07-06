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

// ScrollLines scrolls the viewport by delta lines (positive = toward the end of
// the file) without moving the cursor, for GUI wheel/trackpad scrolling. Unlike
// a vi command it does not keep the cursor on screen -- the view scrolls freely
// like any windowed app; the next edit or motion brings the cursor back into
// view. top is clamped so the buffer always fills from a real line.
func (e *Engine) ScrollLines(delta int) {
	setScreenTop(e.scr, e.scr.top+int64(delta))
}

// setScreenTop moves a screen's viewport top to the given line, clamped into
// the buffer, without touching the cursor.
func setScreenTop(s *screen, top int64) {
	if top < 1 {
		top = 1
	}
	if n := s.store.Lines(); top > n {
		top = n
	}
	s.top = top
}

// Panes: the GUI addresses split screens by their index in display order
// (top-to-bottom, then left-to-right), the same order View.Screens() reports.
// These back per-pane scroll bars and mouse-driven pane switching/resizing.

// PaneCount returns the number of displayed screens (split panes).
func (e *Engine) PaneCount() int { return len(e.screens) }

// ScrollLinesPane is ScrollLines for the pane at index i (which need not be
// the active one -- macOS scrolls the view under the pointer).
func (e *Engine) ScrollLinesPane(i, delta int) {
	if i < 0 || i >= len(e.screens) {
		return
	}
	s := e.screens[i]
	setScreenTop(s, s.top+int64(delta))
}

// SetPaneTop scrolls pane i so line top is the first visible line, clamped
// into the buffer. Backs absolute scroller-thumb positioning, which would
// drift if expressed as deltas.
func (e *Engine) SetPaneTop(i int, top int64) {
	if i < 0 || i >= len(e.screens) {
		return
	}
	setScreenTop(e.screens[i], top)
}

// FocusPane makes pane i the active screen (GUI click-to-focus). Like ^W
// (switchScreen) both the screen left and the one entered get the transient
// status line.
func (e *Engine) FocusPane(i int) {
	if i < 0 || i >= len(e.screens) || i == e.cur {
		return
	}
	old := e.scr
	e.setCur(i)
	e.setStatusMsg(old)
	e.setStatusMsg(e.scr)
}

// DragDividerRows moves the divider below pane i (its status row) by delta
// rows: positive drags it down, growing the panes above it at the expense of
// the panes below. The whole border segment moves: every pane whose top or
// bottom border is this divider and whose column span is connected to pane
// i's resizes together (so the divider between a pane and a vertically split
// half still drags, resizing all three). Clamped so every pane on the
// shrinking side keeps minScreenRows text rows; without any pane below it is
// a no-op. Unlike :resize it sets no transient status (a drag repaints
// continuously).
func (e *Engine) DragDividerRows(i, delta int) {
	if i < 0 || i >= len(e.screens) || delta == 0 {
		return
	}
	s := e.screens[i]
	above, below := e.borderScreensRows(s, s.roff+s.rows)
	if len(below) == 0 {
		return
	}
	if delta > 0 {
		for _, p := range below {
			if m := p.rows - minScreenRows; delta > m {
				delta = m
			}
		}
	} else {
		for _, p := range above {
			if m := p.rows - minScreenRows; -delta > m {
				delta = -m
			}
		}
	}
	if delta == 0 {
		return
	}
	for _, p := range above {
		p.rows += delta
	}
	for _, p := range below {
		p.roff += delta
		p.rows -= delta
	}
	resizedScreens(append(above, below...)...)
}

// DragDividerCols moves the divider column right of pane i by delta columns:
// positive drags it right, growing the panes on its left at the expense of
// the panes on its right; like DragDividerRows the whole connected border
// segment moves. Clamped so every pane on the shrinking side keeps
// minScreenCols text columns; without any pane to the right it is a no-op.
func (e *Engine) DragDividerCols(i, delta int) {
	if i < 0 || i >= len(e.screens) || delta == 0 {
		return
	}
	s := e.screens[i]
	left, right := e.borderScreensCols(s, s.coff+s.cols)
	if len(right) == 0 {
		return
	}
	if delta > 0 {
		for _, p := range right {
			if m := p.cols - minScreenCols; delta > m {
				delta = m
			}
		}
	} else {
		for _, p := range left {
			if m := p.cols - minScreenCols; -delta > m {
				delta = -m
			}
		}
	}
	if delta == 0 {
		return
	}
	for _, p := range left {
		p.cols += delta
	}
	for _, p := range right {
		p.coff += delta
		p.cols -= delta
	}
	resizedScreens(append(left, right...)...)
}

// borderScreensRows collects the screens that share the horizontal divider at
// display row r (s's status row): above are those whose bottom border it is,
// below those starting just under it. Membership is the transitive closure of
// column-span overlap starting from s, so a border shared with a split half
// picks up every pane along the connected segment -- but a coincidentally
// aligned divider in an unconnected column stack does not move. Spans are
// half-open [coff, coff+cols+1), covering the pane's own divider column.
func (e *Engine) borderScreensRows(s *screen, r int) (above, below []*screen) {
	lo, hi := s.coff, s.coff+s.cols+1
	in := map[*screen]bool{s: true}
	for changed := true; changed; {
		changed = false
		for _, t := range e.screens {
			if in[t] || (t.roff+t.rows != r && t.roff != r+1) {
				continue
			}
			if t.coff >= hi || t.coff+t.cols+1 <= lo {
				continue
			}
			in[t] = true
			changed = true
			if t.coff < lo {
				lo = t.coff
			}
			if t.coff+t.cols+1 > hi {
				hi = t.coff + t.cols + 1
			}
		}
	}
	for _, t := range e.screens { // keep display order
		switch {
		case !in[t]:
		case t.roff+t.rows == r:
			above = append(above, t)
		case t.roff == r+1:
			below = append(below, t)
		}
	}
	return
}

// borderScreensCols is borderScreensRows for the vertical divider at column x
// (the column sacrificed right of s): left are the screens ending at it,
// right those starting just past it, connected by display-band overlap
// (half-open [roff, roff+rows+1), text rows plus the status row).
func (e *Engine) borderScreensCols(s *screen, x int) (left, right []*screen) {
	lo, hi := s.roff, s.roff+s.rows+1
	in := map[*screen]bool{s: true}
	for changed := true; changed; {
		changed = false
		for _, t := range e.screens {
			if in[t] || (t.coff+t.cols != x && t.coff != x+1) {
				continue
			}
			if t.roff >= hi || t.roff+t.rows+1 <= lo {
				continue
			}
			in[t] = true
			changed = true
			if t.roff < lo {
				lo = t.roff
			}
			if t.roff+t.rows+1 > hi {
				hi = t.roff + t.rows + 1
			}
		}
	}
	for _, t := range e.screens { // keep display order
		switch {
		case !in[t]:
		case t.coff+t.cols == x:
			left = append(left, t)
		case t.coff == x+1:
			right = append(right, t)
		}
	}
	return
}

// resizedScreens re-derives per-screen display state after a geometry change,
// the same reset resizeScreen does: drop any z[count]-reduced map and default
// scroll amount, and bring the cursor back into the (new) viewport.
func resizedScreens(scs ...*screen) {
	for _, sc := range scs {
		sc.mapRows = sc.rows
		sc.minMapRows = sc.rows
		sc.defScroll = 0
		sc.clampCursor()
		sc.scrollToCursor()
	}
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
