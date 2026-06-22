package engine

// Insert and replace mode. Entered from command-mode commands (i I a A o O c R
// s S), exited with ESC. Buffer edits happen through the screen primitives
// inside the change bracket opened when insert began, so the whole insertion is
// one undo unit.

// enterInsert opens a change and starts insert/replace mode at pos.
func (e *Engine) enterInsert(m *vimode, pos Pos, replace bool, cmd rune) {
	e.beginChange()
	m.startInsert(e, pos, replace, cmd)
}

// startInsert begins insert/replace mode without opening a change bracket; the
// caller (e.g. the c operator) has already opened one.
func (m *vimode) startInsert(e *Engine, pos Pos, replace bool, cmd rune) {
	s := e.scr
	m.inserting = true
	m.replaceMode = replace
	m.insertText = m.insertText[:0]
	m.insertCmd = cmd
	m.insertCount = effCount(m.count)
	m.count, m.haveCount = 0, false
	s.cursor = pos
	s.mode = ModeInsert
	if replace {
		s.mode = ModeReplace
	}
	s.clampCursor()
}

func (m *vimode) insertKey(e *Engine, ev KeyEvent) {
	switch {
	case ev.Key == KeyEscape:
		m.finishInsert(e)
	case ev.Key == KeyEnter || ev.Rune == '\r' || ev.Rune == '\n':
		m.insertNewline(e)
		m.insertText = append(m.insertText, '\n')
	case ev.Key == KeyBackspace || ev.Rune == 0x7f || ev.Rune == '\b':
		m.insertBackspace(e)
	case ev.Rune != 0:
		m.insertRune(e, ev.Rune)
		m.insertText = append(m.insertText, ev.Rune)
	}
}

func (m *vimode) insertRune(e *Engine, r rune) {
	s := e.scr
	line := s.lineRunes(s.cursor.Line)
	col := clampIdx(s.cursor.Col, len(line))
	if m.replaceMode && col < len(line) {
		nl := cloneR(line)
		nl[col] = r
		s.setLine(s.cursor.Line, nl)
	} else {
		nl := make([]rune, 0, len(line)+1)
		nl = append(nl, line[:col]...)
		nl = append(nl, r)
		nl = append(nl, line[col:]...)
		s.setLine(s.cursor.Line, nl)
	}
	s.cursor.Col = col + 1
}

func (m *vimode) insertNewline(e *Engine) {
	s := e.scr
	line := s.lineRunes(s.cursor.Line)
	col := clampIdx(s.cursor.Col, len(line))
	head := cloneR(line[:col])
	tail := cloneR(line[col:])
	s.setLine(s.cursor.Line, head)
	s.appendLine(s.cursor.Line, tail)
	s.cursor = Pos{Line: s.cursor.Line + 1, Col: 0}
}

func (m *vimode) insertBackspace(e *Engine) {
	s := e.scr
	if s.cursor.Col > 0 {
		line := s.lineRunes(s.cursor.Line)
		col := clampIdx(s.cursor.Col, len(line))
		nl := append(cloneR(line[:col-1]), line[col:]...)
		s.setLine(s.cursor.Line, nl)
		s.cursor.Col = col - 1
		return
	}
	if s.cursor.Line > 1 {
		prev := s.cursor.Line - 1
		prevLen := s.lineLen(prev)
		merged := append(cloneR(s.lineRunes(prev)), s.lineRunes(s.cursor.Line)...)
		s.setLine(prev, merged)
		s.deleteLine(s.cursor.Line)
		s.cursor = Pos{Line: prev, Col: prevLen}
	}
}

func (m *vimode) finishInsert(e *Engine) {
	s := e.scr

	// Repeat the inserted text for a count (e.g. 3ifoo<ESC>), for single-line
	// insertions.
	if m.insertCount > 1 && len(m.insertText) > 0 && !containsNewline(m.insertText) {
		for i := 1; i < m.insertCount; i++ {
			for _, r := range m.insertText {
				m.insertRune(e, r)
			}
		}
	}

	changed := len(m.insertText) > 0 || m.insertCmd == 'o' || m.insertCmd == 'O' || m.insertCmd == 'c'

	m.inserting = false
	m.replaceMode = false
	s.mode = ModeCommand
	if s.cursor.Col > 0 {
		s.cursor.Col--
	}
	e.endChange()
	s.clampCursor()
	if changed {
		m.changed = true
	}
}

func containsNewline(r []rune) bool {
	for _, c := range r {
		if c == '\n' {
			return true
		}
	}
	return false
}
