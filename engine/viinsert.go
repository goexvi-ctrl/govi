package engine

import "unicode/utf8"

// noOrig marks an overtyped slot that had no original character (R-mode typing
// past the original end of line, i.e. an append); backspace there deletes rather
// than restores.
const noOrig = rune(-1)

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
	m.overtyped = m.overtyped[:0]
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
	m.insertEnter = s.cursor // where this insertion began (bounds ^U)
	// Autoindent-erase (^D) state. A plain insert on an existing line has no
	// autoindent characters (nvi: tp->ai is 0 unless the line was auto-opened),
	// so ^D there is a literal; o/O and insert-<CR> set aiCount afterward.
	m.aiCount, m.aiCarat, m.aiRestore = 0, 0, nil
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

	// ^X collects up to 6 hex digits -- enough for any Unicode code point -- and
	// inserts that character. It ends at 6 digits or on the first non-hex key.
	if m.hexMode {
		if isHexDigit(ev.Rune) {
			m.hexBuf = append(m.hexBuf, ev.Rune)
			if len(m.hexBuf) >= 6 {
				m.finishHex(e)
			}
			return
		}
		m.finishHex(e)
		// fall through to process the terminating key normally
	}

	// The ^^D / 0^D autoindent-erase forms require the '^' or '0' to be the key
	// immediately before ^D (nvi keys off carat state and cursor position). Clear
	// it for this key up front; the normal-char branch re-arms it, and ^D reads
	// the captured value.
	prevCarat := m.aiCarat
	m.aiCarat = 0

	// Insert-mode control commands.
	if ev.Mods&ModCtrl != 0 && ev.Key == KeyNone {
		switch ev.Rune {
		case 'v': // literal next
			m.literalNext = true
		case 'w': // erase the word before the cursor
			m.insertWordErase(e)
		case 'u': // erase back to the start of the inserted text on this line
			m.insertLineErase(e)
		case 't': // indent to the next shiftwidth boundary at the cursor
			m.insertIndent(e)
		case 'd': // erase autoindent (^D), or a literal ^D past the indent
			m.insertCtrlD(e, prevCarat)
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
		default:
			// An unhandled control key (^A ^B ^G ^K ^P ...) is inserted literally,
			// rendered in caret notation (nvi vi/v_txt.c: a control with no insert
			// binding lands in the buffer), instead of being discarded.
			if r, ok := ctrlRune(ev); ok && r != 0 {
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
	case ev.Key == KeyLeft:
		m.insertArrowHoriz(e, -1)
	case ev.Key == KeyRight:
		m.insertArrowHoriz(e, +1)
	case ev.Key == KeyUp:
		m.insertArrowVert(e, -1)
	case ev.Key == KeyDown:
		m.insertArrowVert(e, +1)
	case ev.Rune != 0:
		// Typing a non-word character triggers abbreviation expansion of the
		// word just completed.
		if !isWordRune(ev.Rune) {
			e.maybeExpandAbbrev()
		}
		// Arm the ^^D / 0^D forms: a '^' or '0' typed within the autoindent makes
		// the next key, if ^D, erase all the indent (nvi K_CARAT / K_ZERO).
		if (ev.Rune == '^' || ev.Rune == '0') && e.scr.opts.Bool("autoindent") &&
			e.scr.cursor.Col <= m.aiCount {
			m.aiCarat = ev.Rune
		}
		m.insertRune(e, ev.Rune)
		m.insertText = append(m.insertText, ev.Rune)
		m.maybeWrapMargin(e)
	}
}

// maybeWrapMargin implements automatic line breaking during insert: once the
// cursor passes the break column it breaks the line at the last blank before the
// current word, moving that word to a new line. wrapmargin (nvi O_WRAPMARGIN)
// sets the break column relative to the right edge (cols - wrapmargin); wraplen
// (O_WRAPLEN) sets it absolutely. If both are set wrapmargin wins, matching nvi.
func (m *vimode) maybeWrapMargin(e *Engine) {
	s := e.scr
	wm := s.opts.Int("wrapmargin")
	wl := s.opts.Int("wraplen")
	var breakCol int
	switch {
	case wm > 0 && s.cols > 0:
		breakCol = s.cols - wm
	case wl > 0:
		breakCol = wl
	default:
		return
	}
	line := s.lineRunes(s.cursor.Line)
	col := clampIdx(s.cursor.Col, len(line))
	dl := makeDisplayLine(line, s.opts.Int("tabstop"), s.opts.Bool("list"))
	if DisplayColumn(dl, col) <= breakCol {
		return
	}
	// Break at the last blank before the current word; a word with no blank
	// before it is left long (nvi does not split a single over-long word).
	i := col - 1
	for i >= 0 && line[i] != ' ' && line[i] != '\t' {
		i--
	}
	if i < 0 {
		return
	}
	s.setLine(s.cursor.Line, cloneR(line[:i]))
	s.appendLine(s.cursor.Line, cloneR(line[i+1:]))
	s.cursor = Pos{Line: s.cursor.Line + 1, Col: col - (i + 1)}
}

func (m *vimode) insertRune(e *Engine, r rune) {
	s := e.scr
	line := s.lineRunes(s.cursor.Line)
	col := clampIdx(s.cursor.Col, len(line))
	if m.replaceMode && col < len(line) {
		m.overtyped = append(m.overtyped, line[col]) // remember the original for backspace
		nl := cloneR(line)
		nl[col] = r
		s.setLine(s.cursor.Line, nl)
	} else {
		if m.replaceMode {
			m.overtyped = append(m.overtyped, noOrig) // appended past EOL: nothing to restore
		}
		nl := make([]rune, 0, len(line)+1)
		nl = append(nl, line[:col]...)
		nl = append(nl, r)
		nl = append(nl, line[col:]...)
		s.setLine(s.cursor.Line, nl)
	}
	s.cursor.Col = col + 1

	// showmatch: briefly flash the matching open bracket. nvi flashes only
	// for ')' and '}' (v_txt.c K_RIGHTPAREN/K_RIGHTBRACE); including ']' is
	// vim's set.
	if (r == ')' || r == '}') && s.opts.Bool("showmatch") {
		if mp, ok := e.findOpenMatch(Pos{Line: s.cursor.Line, Col: col}); ok {
			s.matchActive = true
			s.matchPos = mp
		}
	}
}

// insertArrowHoriz and insertArrowVert move the cursor during insert mode when
// an arrow key is pressed, staying in insert. This is a deliberate improvement
// over nvi, which input-maps the arrows to "ESC <motion> a" (cl_term.c); govi
// instead moves the cursor cleanly and keeps one continuous insertion (matching
// vim's visible behavior). Each move starts a fresh insert segment: it re-bounds
// ^U/^W to the new position and cancels any pending autoindent-erase state, so
// those keys never reach across a cursor jump.
func (m *vimode) insertArrowHoriz(e *Engine, dir int) {
	s := e.scr
	col := s.cursor.Col + dir
	if col < 0 {
		col = 0 // at column 0 a left arrow stays put (no line wrap, like vi default)
	}
	if max := s.lineLen(s.cursor.Line); col > max {
		col = max // in insert the cursor may sit one past the last rune
	}
	s.cursor.Col = col
	m.insertArrowReset(e, false)
}

func (m *vimode) insertArrowVert(e *Engine, dir int) {
	s := e.scr
	target := s.cursor.Line + int64(dir)
	if target < 1 || target > s.lineCount() {
		return // at the first/last line the arrow is a no-op
	}
	col := s.cursor.Col
	if max := s.lineLen(target); col > max {
		col = max
	}
	s.cursor = Pos{Line: target, Col: col}
	m.insertArrowReset(e, true)
}

// insertArrowReset re-anchors the insert session after an arrow move.
func (m *vimode) insertArrowReset(e *Engine, changedLine bool) {
	m.insertEnter = e.scr.cursor
	m.aiCarat = 0
	if changedLine {
		m.aiCount = 0 // the autoindent count is specific to the line we left
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
		if m.aiRestore != nil {
			// A preceding ^^D erased this line's indent but asked to reinstate it
			// on the next autoindented line (nvi nochange).
			indent = m.aiRestore
		} else {
			indent = leadingWhitespace(line)
		}
	}
	m.aiRestore = nil
	newContent := append(cloneR(indent), tail...)
	s.appendLine(s.cursor.Line, newContent)
	s.cursor = Pos{Line: s.cursor.Line + 1, Col: len(indent)}
	m.aiCount = len(indent) // the copied indent is autoindent for ^D
}

// insertBackspace implements ^H / Backspace in insert mode: erase the
// character before the cursor, or at column 0 join the current line onto the
// previous one. At the very start of the buffer (line 1, col 0) there is
// nothing to erase and no line to join, so this is a deliberate no-op; govi
// does not ring the bell there as traditional vi does.
func (m *vimode) insertBackspace(e *Engine) {
	s := e.scr
	// Replace mode: backspace steps left and restores the overtyped original
	// (nvi R behavior), rather than deleting a character. Once back at the insert
	// start (nothing left overtyped) it does nothing.
	if m.replaceMode {
		if len(m.overtyped) == 0 || s.cursor.Col == 0 {
			return
		}
		orig := m.overtyped[len(m.overtyped)-1]
		m.overtyped = m.overtyped[:len(m.overtyped)-1]
		line := s.lineRunes(s.cursor.Line)
		col := clampIdx(s.cursor.Col, len(line))
		if orig == noOrig {
			s.setLine(s.cursor.Line, append(cloneR(line[:col-1]), line[col:]...)) // delete the appended char
		} else {
			nl := cloneR(line)
			nl[col-1] = orig
			s.setLine(s.cursor.Line, nl)
		}
		s.cursor.Col = col - 1
		return
	}
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
// cursor. nvi (v_txt.c K_VWERASE) offers three word definitions, selected by the
// altwerase and ttywerase options and demonstrated by how "/a/b/c" is erased:
//   - default (historic vi): two character classes plus <blank>s delimit words,
//     so ^W erases just "c";
//   - ttywerase: only <blank>s delimit, so ^W erases the whole "/a/b/c";
//   - altwerase: like the default but the first erased character's class is
//     ignored, so ^W erases "/c".
// The erase stops at the insertion start (nvi bounds it at tp->offset), so it
// cannot delete text that predates the insertion.
func (m *vimode) insertWordErase(e *Engine) {
	s := e.scr
	line := s.lineRunes(s.cursor.Line)
	col := clampIdx(s.cursor.Col, len(line))
	blank := func(r rune) bool { return r == ' ' || r == '\t' }
	lo := 0
	if s.cursor.Line == m.insertEnter.Line && m.insertEnter.Col <= col {
		lo = m.insertEnter.Col
	}
	i := col
	// Skip trailing <blank>s first (all three modes do this).
	for i > lo && blank(line[i-1]) {
		i--
	}
	switch {
	case s.opts.Bool("ttywerase"):
		for i > lo && !blank(line[i-1]) {
			i--
		}
	case s.opts.Bool("altwerase"):
		// Erase one character regardless of class, then stay within its class.
		if i > lo {
			i--
			if i > lo && !blank(line[i-1]) {
				cls := isWordRune(line[i-1])
				for i > lo && !blank(line[i-1]) && isWordRune(line[i-1]) == cls {
					i--
				}
			}
		}
	default:
		if i > lo {
			cls := isWordRune(line[i-1])
			for i > lo && !blank(line[i-1]) && isWordRune(line[i-1]) == cls {
				i--
			}
		}
	}
	nl := append(cloneR(line[:i]), line[col:]...)
	s.setLine(s.cursor.Line, nl)
	s.cursor.Col = i
}

// insertLineErase implements insert-mode ^U: erase from the cursor back to the
// start of the text inserted on the current line (insertEnter), or to column 0
// when the insertion began on an earlier line (nvi txt.c TXT_BS to the insert
// start).
func (m *vimode) insertLineErase(e *Engine) {
	s := e.scr
	line := s.lineRunes(s.cursor.Line)
	col := clampIdx(s.cursor.Col, len(line))
	lo := 0
	if s.cursor.Line == m.insertEnter.Line && m.insertEnter.Col <= col {
		lo = m.insertEnter.Col
	}
	nl := append(cloneR(line[:lo]), line[col:]...)
	s.setLine(s.cursor.Line, nl)
	s.cursor.Col = lo
}

// insertIndent implements insert-mode ^T (nvi v_txt.c txt_dent, isindent=1):
// advance the cursor's screen column to the next shiftwidth boundary. Any
// <blank>s immediately before the cursor are first consumed, then the gap from
// the remaining text to the target column is filled with <tab>s (each worth a
// full tabstop) and trailing <space>s, inserted AT THE CURSOR. This differs
// from vim, which shifts the line's leading indent regardless of the cursor.
func (m *vimode) insertIndent(e *Engine) {
	s := e.scr
	line := s.lineRunes(s.cursor.Line)
	col := clampIdx(s.cursor.Col, len(line))
	ts, sw := s.opts.Int("tabstop"), s.opts.Int("shiftwidth")
	scrcol := func(n int) int {
		c := 0
		for i := 0; i < n; i++ {
			if line[i] == '\t' {
				c += ts - c%ts
			} else {
				c++
			}
		}
		return c
	}
	target := scrcol(col)
	target += sw - target%sw

	lo := col
	for lo > 0 && (line[lo-1] == ' ' || line[lo-1] == '\t') {
		lo--
	}
	var fill []rune
	for cno := scrcol(lo); cno < target; {
		if step := ts - cno%ts; cno+step <= target {
			fill = append(fill, '\t')
			cno += step
		} else {
			fill = append(fill, ' ')
			cno++
		}
	}
	nl := cloneR(line[:lo])
	nl = append(nl, fill...)
	nl = append(nl, line[col:]...)
	s.setLine(s.cursor.Line, nl)
	atBoundary := col == m.aiCount // ^T at the end of the indent extends it (nvi ai_reset)
	s.cursor.Col = lo + len(fill)
	if atBoundary {
		m.aiCount = s.cursor.Col
	}
}

// insertCtrlD implements insert-mode ^D (nvi v_txt.c, K_CNTRLD): it erases
// autoindent characters, and is otherwise a literal control character. carat is
// the '^' or '0' typed immediately before, selecting the ^^D / 0^D forms:
//   - plain ^D erases back to the previous shiftwidth column within the indent;
//   - 0^D erases all the autoindent;
//   - ^^D erases all the autoindent but restores it on the next opened line.
// With no autoindent option, at column 0, or once the cursor has moved past the
// autoindent, ^D falls through to a literal ^D, matching historic vi.
func (m *vimode) insertCtrlD(e *Engine, carat rune) {
	s := e.scr
	literal := func() {
		m.insertRune(e, 0x04)
		m.insertText = append(m.insertText, 0x04)
	}
	if !s.opts.Bool("autoindent") {
		literal()
		return
	}
	if s.cursor.Col == 0 {
		return // first column, nothing to erase: ignore (historic practice)
	}
	switch carat {
	case '^': // ^^D: erase all the indent, restore it on the next line.
		if m.aiCount == 0 || s.cursor.Col > m.aiCount+1 {
			literal()
			return
		}
		m.aiRestore = cloneR(s.lineRunes(s.cursor.Line)[:m.aiCount])
		m.eraseToLeftMargin(e)
	case '0': // 0^D: erase all the indent.
		if m.aiCount == 0 || s.cursor.Col > m.aiCount+1 {
			literal()
			return
		}
		m.eraseToLeftMargin(e)
	default: // ^D: erase back one shiftwidth.
		if m.aiCount == 0 || s.cursor.Col > m.aiCount {
			literal()
			return
		}
		m.insertDedent(e)
	}
}

// eraseToLeftMargin removes the leading whitespace up to the cursor (the
// autoindent plus the triggering '^' or '0'), leaving the cursor at column 0.
// This is nvi's txt() "leftmargin" step for the 0^D / ^^D forms.
func (m *vimode) eraseToLeftMargin(e *Engine) {
	s := e.scr
	line := s.lineRunes(s.cursor.Line)
	col := clampIdx(s.cursor.Col, len(line))
	s.setLine(s.cursor.Line, cloneR(line[col:]))
	s.cursor.Col = 0
	m.aiCount = 0
}

// insertDedent rebuilds the leading autoindent so the first non-blank sits at the
// previous shiftwidth boundary (nvi txt_dent with isindent=0). Everything before
// the cursor is autoindent whitespace, so it is replaced wholesale.
func (m *vimode) insertDedent(e *Engine) {
	s := e.scr
	line := s.lineRunes(s.cursor.Line)
	col := clampIdx(s.cursor.Col, len(line))
	ts, sw := s.opts.Int("tabstop"), s.opts.Int("shiftwidth")
	if sw <= 0 {
		sw = ts
	}
	current := 0
	for i := 0; i < col; i++ {
		if line[i] == '\t' {
			current += ts - current%ts
		} else {
			current++
		}
	}
	if current == 0 {
		return
	}
	target := ((current - 1) / sw) * sw
	indent := makeIndent(target, ts)
	nl := append(cloneR(indent), line[col:]...)
	s.setLine(s.cursor.Line, nl)
	s.cursor.Col = len(indent)
	m.aiCount = len(indent)
}

func isHexDigit(r rune) bool {
	return r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F'
}

// finishHex inserts the character whose hex code (up to 6 digits) was collected
// after ^X. This is a deliberate modern extension of nvi's ^X: any Unicode code
// point can be entered -- 2 digits for a byte, 4 for the BMP, 6 for astral
// planes. An out-of-range or surrogate value is inserted as U+FFFD.
func (m *vimode) finishHex(e *Engine) {
	m.hexMode = false
	if len(m.hexBuf) == 0 {
		return
	}
	v := 0
	for _, r := range m.hexBuf {
		v = v*16 + hexVal(r)
	}
	m.hexBuf = m.hexBuf[:0]
	r := rune(v)
	if !utf8.ValidRune(r) {
		r = utf8.RuneError
	}
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
	// insertions. For o/O each repeat opens a fresh line first, so "2onew" makes
	// two lines rather than "newnew" on one (nvi); the in-line inserts (i a I A
	// s ...) simply retype the text on the same line.
	if m.insertCount > 1 && len(m.insertText) > 0 && !containsNewline(m.insertText) {
		openEach := m.insertCmd == 'o' || m.insertCmd == 'O'
		// A counted R overtypes on the first pass but INSERTS the repeat passes
		// (nvi: "2Rab" on "alpha" gives "ababpha", the line growing by 2), so
		// drop replace mode before repeating.
		if m.insertCmd == 'R' {
			m.replaceMode = false
		}
		for i := 1; i < m.insertCount; i++ {
			if openEach {
				m.insertNewline(e)
			}
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
