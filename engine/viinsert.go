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
	s.showModeLabel = insertShowMode(cmd, replace)
	s.clampCursor()
}

func (m *vimode) insertKey(e *Engine, ev KeyEvent) {
	// ^V quotes the next key: insert it literally with no special handling.
	if m.literalNext {
		m.literalNext = false
		if r, ok := literalRune(ev); ok {
			m.insertRune(e, r)
			m.insertText = append(m.insertText, r)
		}
		return
	}

	// ^X collects a hexadecimal character code until a non-hex key.
	if m.hexMode {
		if isHexDigit(ev.Rune) {
			m.hexBuf = append(m.hexBuf, ev.Rune)
			return
		}
		m.finishHex(e)
		// fall through to process the terminating key normally
	}

	// Insert-mode control commands.
	if ev.Mods&ModCtrl != 0 && ev.Key == KeyNone {
		switch ev.Rune {
		case 'v': // literal next
			m.literalNext = true
		case 'w': // erase the word before the cursor
			m.insertWordErase(e)
		case 't': // shift the current line right by shiftwidth
			m.insertShift(e, +1)
		case 'd': // shift the current line left by shiftwidth
			m.insertShift(e, -1)
		case 'h': // erase the previous character
			m.insertBackspace(e)
		case 'x': // begin a hexadecimal character entry
			m.hexMode = true
			m.hexBuf = m.hexBuf[:0]
		case 'z': // ^Z: leave insert and suspend (historic nvi discards input)
			m.leaveInsertForSuspend(e)
			if err := e.doSuspend(false); err != nil {
				e.scr.msg, e.scr.msgKind = err.Error(), MsgError
			}
		case '@': // NUL: replay the previous insertion
			for _, r := range m.savedInsert {
				m.insertRune(e, r)
				m.insertText = append(m.insertText, r)
			}
		}
		return
	}

	switch {
	case ev.Key == KeyEscape:
		e.maybeExpandAbbrev()
		m.finishInsert(e)
	case ev.Key == KeyEnter || ev.Rune == '\r' || ev.Rune == '\n':
		e.maybeExpandAbbrev()
		m.insertNewline(e)
		m.insertText = append(m.insertText, '\n')
	case ev.Key == KeyBackspace || ev.Rune == 0x7f || ev.Rune == '\b':
		m.insertBackspace(e)
	case ev.Rune != 0:
		// Typing a non-word character triggers abbreviation expansion of the
		// word just completed.
		if !isWordRune(ev.Rune) {
			e.maybeExpandAbbrev()
		}
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

	// showmatch: briefly flash the matching open bracket.
	if (r == ')' || r == ']' || r == '}') && s.opts.Bool("showmatch") {
		if mp, ok := e.findOpenMatch(Pos{Line: s.cursor.Line, Col: col}); ok {
			s.matchActive = true
			s.matchPos = mp
		}
	}
}

func (m *vimode) insertNewline(e *Engine) {
	s := e.scr
	line := s.lineRunes(s.cursor.Line)
	col := clampIdx(s.cursor.Col, len(line))
	head := cloneR(line[:col])
	tail := cloneR(line[col:])
	s.setLine(s.cursor.Line, head)

	var indent []rune
	if s.opts.Bool("autoindent") {
		indent = leadingWhitespace(line)
	}
	newContent := append(cloneR(indent), tail...)
	s.appendLine(s.cursor.Line, newContent)
	s.cursor = Pos{Line: s.cursor.Line + 1, Col: len(indent)}
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

// insertWordErase implements ^W: delete the whitespace and word before the
// cursor.
func (m *vimode) insertWordErase(e *Engine) {
	s := e.scr
	line := s.lineRunes(s.cursor.Line)
	col := clampIdx(s.cursor.Col, len(line))
	i := col
	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}
	if i > 0 {
		if isWordRune(line[i-1]) {
			for i > 0 && isWordRune(line[i-1]) {
				i--
			}
		} else {
			for i > 0 && !isWordRune(line[i-1]) && line[i-1] != ' ' && line[i-1] != '\t' {
				i--
			}
		}
	}
	nl := append(cloneR(line[:i]), line[col:]...)
	s.setLine(s.cursor.Line, nl)
	s.cursor.Col = i
}

// insertShift implements ^T / ^D: shift the current line's indentation by one
// shiftwidth, moving the cursor with the text.
func (m *vimode) insertShift(e *Engine, dir int) {
	s := e.scr
	line := s.lineRunes(s.cursor.Line)
	ts, sw := s.opts.Int("tabstop"), s.opts.Int("shiftwidth")
	width, i := 0, 0
	for i < len(line) {
		if line[i] == ' ' {
			width++
			i++
		} else if line[i] == '\t' {
			width += ts - width%ts
			i++
		} else {
			break
		}
	}
	newWidth := width + dir*sw
	if newWidth < 0 {
		newWidth = 0
	}
	indent := makeIndent(newWidth, ts)
	nl := append(cloneR(indent), line[i:]...)
	s.setLine(s.cursor.Line, nl)
	// Adjust the cursor by the change in the indent's rune length.
	s.cursor.Col += len(indent) - i
	if s.cursor.Col < 0 {
		s.cursor.Col = 0
	}
}

func isHexDigit(r rune) bool {
	return r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F'
}

// finishHex inserts the character whose hex code was collected after ^X.
func (m *vimode) finishHex(e *Engine) {
	m.hexMode = false
	if len(m.hexBuf) == 0 {
		return
	}
	var v int64
	for _, r := range m.hexBuf {
		v = v*16 + int64(hexVal(r))
	}
	m.hexBuf = m.hexBuf[:0]
	r := rune(v)
	m.insertRune(e, r)
	m.insertText = append(m.insertText, r)
}

func hexVal(r rune) int {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0')
	case r >= 'a' && r <= 'f':
		return int(r-'a') + 10
	case r >= 'A' && r <= 'F':
		return int(r-'A') + 10
	}
	return 0
}

// leaveInsertForSuspend exits insert mode for ^Z without applying an insert
// count repeat or updating the NUL-replay buffer.
func (m *vimode) leaveInsertForSuspend(e *Engine) {
	s := e.scr
	m.hexMode = false
	m.hexBuf = m.hexBuf[:0]
	m.literalNext = false
	changed := len(m.insertText) > 0 || m.insertCmd == 'o' || m.insertCmd == 'O' || m.insertCmd == 'c'
	m.inserting = false
	m.replaceMode = false
	s.mode = ModeCommand
	s.showModeLabel = "Command"
	if s.cursor.Col > 0 {
		s.cursor.Col--
	}
	e.endChange()
	s.clampCursor()
	if changed {
		m.changed = true
	}
}

func (m *vimode) finishInsert(e *Engine) {
	s := e.scr

	// A pending ^X hex entry is completed by ESC.
	if m.hexMode {
		m.finishHex(e)
	}

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

	// Remember this insertion for a later NUL replay.
	if len(m.insertText) > 0 {
		m.savedInsert = append(m.savedInsert[:0], m.insertText...)
	}

	m.inserting = false
	m.replaceMode = false
	s.mode = ModeCommand
	s.showModeLabel = "Command"
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
