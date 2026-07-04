package engine

import (
	"govi/engine/mark"
	"govi/engine/register"
	"govi/engine/undo"
)

// vimode is the vi command/insert state machine. It corresponds to nvi's vi
// command loop and the vikeys dispatch table (vi/vi.c, vi/v_cmd.c), accumulating
// counts, register selection, and operator+motion commands across keystrokes,
// and driving insert mode. It lives in the engine package (rather than a
// subpackage) because it is intrinsically coupled to the per-view screen state;
// the embedding boundary that matters for GUIs is Frontend/View, which stays
// independent of this.
type vimode struct {
	// command-building state
	count     int
	haveCount bool
	reg       rune // selected register (0 = none)
	awaitReg  bool // saw '"', next key is the register name
	op        rune // pending operator (0 = none)
	opCount   int
	opReg     rune
	pending   rune // command awaiting a single char (f F t T r m ` ')

	// search-as-motion: a / or ? typed while an operator is pending defers the
	// operator until the search line completes (d/pat, y/pat). searchOp holds the
	// operator (0 = a plain search, just move the cursor); searchStart is the
	// cursor at the time, the start of the operated range.
	searchOp      rune
	searchOpReg   rune
	searchOpCount int
	searchStart   Pos

	// z command: [line]z[window][type] (nvi v_z.c).
	zLine      int64 // count1 line (0 = current line)
	zCount2    int   // digits after z
	zCount2Set bool  // saw at least one digit after z

	// f/t repeat state for ; and ,
	findCmd  rune
	findChar rune

	// cursor-column maintenance: preserveCol means this command keeps the
	// desired display column (j/k/^F...); setEOL means it makes the column
	// sticky to the end of line ($).
	preserveCol bool
	setEOL      bool

	// insert/replace mode
	inserting   bool
	replaceMode bool
	insertText  []rune // text typed this insert (for dot)
	insertEnter Pos    // where insert began (bounds ^U)
	overtyped   []rune // R-mode: original chars overtyped this insert (noOrig = appended past EOL), for backspace restore
	insertCmd   rune   // command that entered insert (i/a/o/O/c/R...)
	insertCount int    // repeat count for the insertion
	literalNext bool   // ^V was typed; insert the next key literally
	hexMode     bool   // ^X was typed; collecting hex digits
	hexBuf      []rune // accumulated hex digits
	savedInsert []rune // text of the last completed insertion (for NUL replay)

	// dot repeat
	rec       []KeyEvent
	dot       []KeyEvent
	replaying bool
	changed   bool // current command modified the buffer

	// numbered-register put ring: putReg is the register the current command's
	// put drew from (0 = none); dotPutReg is that register when the dot command
	// is a numbered put ("1p..."9p), so '.' can walk the delete ring (nvi: "1p
	// then . puts "2, "3, ...). 0 when the dot command is not a numbered put.
	putReg    rune
	dotPutReg rune

	// undo state (nvi semantics). Undo/redo has a direction: a literal 'u'
	// toggles the direction and takes one step, while '.' repeats the last
	// command -- so after an undo/redo it takes another step in the SAME
	// direction. Thus uuu... toggles undo/redo, and u then . . walks in one
	// direction, until another 'u' flips it. lastStepRedo is the direction of
	// the most recent step (true = redo/forward); dotUndo marks that the last
	// command was an undo/redo so '.' continues stepping.
	lastStepRedo bool
	dotUndo      bool
}

// newVimode initializes the machine. lastStepRedo starts true so the first 'u'
// (which toggles the direction) performs an undo.
func newVimode() *vimode { return &vimode{lastStepRedo: true} }

func effCount(c int) int {
	if c <= 0 {
		return 1
	}
	return c
}

func (m *vimode) idle() bool {
	return !m.haveCount && m.op == 0 && m.reg == 0 && !m.awaitReg && m.pending == 0 && !m.inserting
}

func (m *vimode) pendingState() bool {
	return m.haveCount || m.op != 0 || m.reg != 0 || m.awaitReg || m.pending != 0 || m.inserting
}

// key is the entry point for every keystroke while not on the colon line.
func (m *vimode) key(e *Engine, ev KeyEvent) {
	if !m.replaying {
		if !m.inserting && m.pending == 0 && !m.awaitReg && m.idle() {
			m.rec = m.rec[:0]
		}
		m.rec = append(m.rec, ev)
	}

	switch {
	case m.inserting:
		m.insertKey(e, ev)
	case m.awaitReg:
		m.awaitReg = false
		if ev.Rune != 0 {
			m.reg = ev.Rune
		}
	case m.pending != 0:
		m.charArg(e, ev)
	default:
		m.commandKey(e, ev)
	}

	if !m.pendingState() {
		m.finishCommand(e)
	}
}

// finishCommand records the dot command when a buffer-changing command fully
// completes, and clears transient state.
func (m *vimode) finishCommand(e *Engine) {
	// Maintain the desired display column for vertical motions: j/k/^F/...
	// preserve it; every other command resets it to the cursor's column (or to
	// end-of-line after $).
	if m.preserveCol {
		if m.setEOL {
			e.scr.desiredEOL = true
		}
	} else {
		e.scr.desiredCol = e.scr.displayCursorColOf(e.scr.cursor.Line, e.scr.cursor.Col)
		e.scr.desiredEOL = m.setEOL
	}
	m.preserveCol = false
	m.setEOL = false

	if m.changed {
		if !m.replaying && len(m.rec) > 0 {
			m.dot = append(m.dot[:0], m.rec...)
			// Remember whether this dot command is a numbered-register put, so
			// a following '.' walks the delete ring rather than repeating "1.
			if m.putReg >= '1' && m.putReg <= '9' {
				m.dotPutReg = m.putReg
			} else {
				m.dotPutReg = 0
			}
		}
		// A new change ends the undo/redo sequence and makes '.' repeat this
		// change; the next 'u' should undo (so reset direction to redo, which
		// the 'u' toggle flips to undo).
		m.dotUndo = false
		m.lastStepRedo = true
	}
	m.changed = false
	m.count = 0
	m.haveCount = false
	m.reg = 0
	m.op = 0
	m.opCount = 0
	m.opReg = 0
	m.putReg = 0
}

// commandKey dispatches one key in command mode.
func (m *vimode) commandKey(e *Engine, ev KeyEvent) {
	s := e.scr
	s.msg, s.msgKind = "", MsgNone

	if ev.Key == KeyEscape {
		m.cancelCommand(e)
		return
	}

	if ev.Mods&ModCtrl != 0 {
		m.ctrlKey(e, ev.Rune)
		return
	}

	r := normalizeKey(ev)
	if r == 0 {
		return
	}

	// Count digits. A leading '0' is the column-0 motion, not a count.
	if r >= '1' && r <= '9' || (r == '0' && m.haveCount) {
		m.count = m.count*10 + int(r-'0')
		m.haveCount = true
		return
	}

	// A doubled operator (dd, cc, yy) is the linewise current-line form. This
	// must be checked before the operator-start case so the second key doesn't
	// just restart the operator.
	if m.op != 0 && r == m.op {
		total := effCount(m.opCount) * effCount(m.count)
		op, reg := m.op, m.opReg
		m.op, m.opCount, m.opReg = 0, 0, 0
		m.count, m.haveCount = 0, false
		// A line-mode count that runs past the last line is an error in nvi (the
		// buffer is untouched), not a clamp to EOF: "100dd" in a 5-line file
		// beeps and does nothing rather than deleting to the end.
		last := s.cursor.Line + int64(total) - 1
		if last > s.lineCount() {
			s.msg, s.msgKind = "Movement past the end-of-file", MsgError
			e.fe.Bell()
			return
		}
		mot := lineMotion(s.cursor.Line, last)
		mot.doubled = true // dd/cc/yy: land on the first non-blank, not column 0
		m.operate(e, op, reg, mot)
		return
	}

	switch r {
	case '"':
		m.awaitReg = true
		return
	case '/', '?':
		// A search used as a motion: defer any pending operator until the search
		// line is entered (d/pat, y/pat). With no operator this is a plain search.
		if m.op != 0 {
			m.searchOp = m.op
			m.searchOpReg = m.opReg
			m.searchOpCount = effCount(m.opCount) * effCount(m.count)
			m.searchStart = s.cursor
			m.op, m.opCount, m.opReg = 0, 0, 0
			m.count, m.haveCount = 0, false
		} else {
			m.searchOp = 0
		}
		if r == '/' {
			e.enterCmdline('/')
		} else {
			e.enterCmdline('?')
		}
		return
	case 'd', 'c', 'y', '!', '>', '<':
		m.startOperator(r)
		return
	case 'n', 'N':
		// n/N repeat the last search. As a plain command they move the cursor;
		// as an operator target (dn, yN) they are a search motion (QA-1).
		if m.op != 0 {
			m.searchRepeatMotion(e, r == 'N')
		} else {
			m.editKey(e, r)
		}
		return
	case 'z':
		if m.haveCount {
			m.zLine = int64(m.count)
		} else {
			m.zLine = 0
		}
		m.count, m.haveCount = 0, false
		m.zCount2, m.zCount2Set = 0, false
		m.pending = 'z'
		return
	case 'f', 'F', 't', 'T', 'r', 'm', '`', '\'', 'Z', '[', ']', '@', '#':
		m.pending = r
		return
	}

	// Motions (also valid as operator targets).
	if isMotionKey(r) {
		m.doMotion(e, r, 0)
		return
	}

	if m.op != 0 {
		// Operator followed by a non-motion: abort the operator (vi beeps).
		m.op, m.opCount, m.opReg = 0, 0, 0
		e.fe.Bell()
		return
	}

	m.editKey(e, r)
}

// cancelCommand implements <ESC> in vi command mode (nvi v_cmd's esc: handling).
// It abandons a partially entered command so the next key starts fresh: any
// pending operator, register selection, and count are discarded. nvi silently
// cancels a "partial" command (one where a non-numeric component -- an operator
// or register -- was entered) but rings the bell for a count-only or idle <ESC>
// (POSIX requires the alert, and nvi refuses to silently swallow a bare count).
func (m *vimode) cancelCommand(e *Engine) {
	partial := m.op != 0 || m.reg != 0
	m.op, m.opCount, m.opReg = 0, 0, 0
	m.reg = 0
	m.count, m.haveCount = 0, false
	if !partial {
		e.fe.Bell()
	}
}

// ctrlKey handles control-key commands (scrolling, movement aliases, info).
func (m *vimode) ctrlKey(e *Engine, r rune) {
	s := e.scr
	count := effCount(m.count)
	switch r {
	case 'f': // forward a full screen (nvi v_pagedown)
		m.pageDown(e, pageOffset(count, s.windowVal()), true)
	case 'b': // back a full screen (nvi v_pageup)
		m.pageUp(e, pageOffset(count, s.windowVal()), true)
	case 'd': // down half a screen (nvi v_hpagedown)
		if m.haveCount {
			s.defScroll = count
		}
		m.pageDown(e, s.halfPage(), false)
	case 'u': // up half a screen (nvi v_hpageup)
		if m.haveCount {
			s.defScroll = count
		}
		m.pageUp(e, s.halfPage(), false)
	case 'e': // scroll viewport down one line (nvi v_linedown)
		m.scrollDown(e, count)
	case 'y': // scroll viewport up one line (nvi v_lineup)
		m.scrollUp(e, count)
	case 'j', 'n': // move down (aliases for j)
		m.moveVertical(e, s.cursor.Line+int64(count))
	case 'p': // move up (alias for k)
		m.moveVertical(e, s.cursor.Line-int64(count))
	case 'g': // file information
		e.fileInfo()
	case 'a': // search forward for the word under the cursor
		if err := e.searchCurrentWord(false); err != nil {
			s.msg, s.msgKind = err.Error(), MsgError
		}
	case '^': // switch to the alternate file
		if err := e.editAlternate(); err != nil {
			s.msg, s.msgKind = err.Error(), MsgError
		}
	case ']': // jump to the tag for the word under the cursor
		if err := e.tagJumpWord(); err != nil {
			s.msg, s.msgKind = err.Error(), MsgError
		}
	case 't': // pop the tag stack
		if err := e.tagPop(); err != nil {
			s.msg, s.msgKind = err.Error(), MsgError
		}
	case 'w': // ^W: switch to the next split screen (nvi v_screen)
		e.switchScreen()
	case 'l', 'r': // ^L / ^R: force a full redraw (nvi v_redraw), recovering the
		// display from tty output another program produced
		e.redrawRequested = true
	case '\\': // ^\ switch to ex mode (nvi v_cmd.c 034)
		e.handleCtrlBackslash()
	case 'z': // suspend editor (^Z)
		if err := e.doSuspend(false); err != nil {
			s.msg, s.msgKind = err.Error(), MsgError
		}
	}
	// Control commands consume any preceding count.
	m.count = 0
	m.haveCount = false
}

// doUndoCommand implements nvi's undo. A literal 'u' (fromDot == false) toggles
// the undo/redo direction and steps; a dot-repeat (fromDot == true) steps again
// in the current direction without toggling. So 'uuu' alternates undo/redo while
// 'u..' continues in one direction until the next 'u' flips it.
func (m *vimode) doUndoCommand(e *Engine, fromDot bool) {
	s := e.scr
	redo := m.lastStepRedo
	if !fromDot {
		redo = !m.lastStepRedo // literal 'u' toggles direction
	}
	var cur undo.Pos
	var ok bool
	if redo {
		cur, ok = s.log.Redo()
	} else {
		cur, ok = s.log.Undo()
	}
	if !ok {
		e.fe.Bell()
		return // leave the direction unchanged at a history boundary
	}
	s.cursor = Pos{Line: cur.Line, Col: cur.Col}
	s.clampCursor()
	s.modified = true
	m.lastStepRedo = redo
	m.dotUndo = true
}

// moveVertical moves the cursor to targetLine keeping the maintained display
// column, marking the command as column-preserving.
func (m *vimode) moveVertical(e *Engine, targetLine int64) {
	s := e.scr
	s.cursor.Line = clampLine(s, targetLine)
	s.cursor.Col = s.maintainedCol(s.cursor.Line)
	m.preserveCol = true
}

// pageOffset is the ^F/^B scroll distance (nvi v_pagedown/v_pageup):
// count * window - 2, never less than one line. The two-line overlap is
// subtracted only once, matching historic vi.
func pageOffset(count, window int) int {
	off := count * window
	if off <= 2 {
		return 1
	}
	return off - 2
}

// pageDown scrolls the viewport toward EOF by offset physical screen rows
// (nvi vs_sm_up): a wrapped line counts one row per sub-row, so the paging
// distance and the cursor's landing line are computed in screen rows, not
// buffer lines. The screen scrolls as far as the file allows; the cursor
// moves down by the full row count from its own screen row (nvi's SMAP
// pointer keeps its row index while the map shifts underneath, then walks the
// remainder), capped at the bottom row of the final screen. A full page (^F)
// that scrolls completely instead puts the cursor on the new top row.
func (m *vimode) pageDown(e *Engine, offset int, full bool) {
	s := e.scr
	if offset < 1 {
		offset = 1
	}
	mapH := s.effectiveMapRows()
	topA := rowAddr{lno: s.top}
	curA := s.cursorRowAddr()

	// Rows available below the current bottom row, capped at offset: the
	// screen scrolls one row per existing row past the bottom.
	_, walked := s.advanceRows(topA, mapH-1+offset)
	scroll := walked - (mapH - 1)
	if scroll < 0 {
		scroll = 0 // the file already ends within the screen
	}
	newTopA, _ := s.advanceRows(topA, scroll)
	// govi's viewport starts at a whole buffer line; when the scroll stops
	// inside a wrapped line the whole line stays visible (cataloged #44: no
	// sub-line top).
	s.top = newTopA.lno

	var target rowAddr
	if full && scroll == offset {
		target = newTopA // ^F full page: cursor to the top row of the new screen
	} else {
		target, _ = s.advanceRows(curA, offset)
		if maxA, _ := s.advanceRows(rowAddr{lno: s.top}, mapH-1); rowAfter(target, maxA) {
			target = maxA // nvi stops the cursor walk at the bottom map row
		}
	}
	s.cursor.Line = target.lno
	if col := s.rowStartCol(target); col != 0 {
		s.cursor.Col = col // landed on a continuation row: its first character
	} else {
		s.cursor.Col = s.firstNonBlank(target.lno)
	}
}

// pageUp scrolls the viewport toward SOF by offset physical screen rows (nvi
// vs_sm_down), the mirror of pageDown. A full page (^B) with a complete
// scroll puts the cursor on the last row of the new screen; otherwise the
// cursor moves up by the full row count, capped at the top row.
func (m *vimode) pageUp(e *Engine, offset int, full bool) {
	s := e.scr
	if offset < 1 {
		offset = 1
	}
	mapH := s.effectiveMapRows()
	topA := rowAddr{lno: s.top}
	curA := s.cursorRowAddr()

	newTopA, scroll := s.retreatRows(topA, offset)
	s.top = newTopA.lno // whole-line viewport top, as in pageDown

	var target rowAddr
	if full && scroll == offset {
		// ^B full page: cursor to the last real row of the new screen.
		target, _ = s.advanceRows(newTopA, mapH-1)
	} else {
		target, _ = s.retreatRows(curA, offset)
		if rowAfter(newTopA, target) {
			target = newTopA // nvi stops the cursor walk at the top map row
		}
	}
	s.cursor.Line = target.lno
	if col := s.rowStartCol(target); col != 0 {
		s.cursor.Col = col
	} else {
		s.cursor.Col = s.firstNonBlank(target.lno)
	}
}

// scrollDown implements ^E: roll the viewport down n lines, leaving the cursor
// on its line until it would scroll off the top, then keeping it on the top row.
func (m *vimode) scrollDown(e *Engine, n int) {
	s := e.scr
	if n < 1 {
		n = 1
	}
	newTop := s.top + int64(n)
	if maxTop := s.topForBottom(s.lineCount()); newTop > maxTop {
		newTop = maxTop
	}
	if newTop < 1 {
		newTop = 1
	}
	s.top = newTop
	if s.cursor.Line < newTop {
		s.cursor.Line = newTop
		s.cursor.Col = s.maintainedCol(newTop)
	}
	m.preserveCol = true
}

// scrollUp implements ^Y: roll the viewport up n lines, leaving the cursor on
// its line until it would scroll off the bottom, then keeping it on the bottom row.
func (m *vimode) scrollUp(e *Engine, n int) {
	s := e.scr
	if n < 1 {
		n = 1
	}
	newTop := s.top - int64(n)
	if newTop < 1 {
		newTop = 1
	}
	s.top = newTop
	if bl := s.bottomLine(newTop); s.cursor.Line > bl {
		s.cursor.Line = bl
		s.cursor.Col = s.maintainedCol(bl)
	}
	m.preserveCol = true
}

// consumeReg returns the selected register and clears the selection so it does
// not leak into the next command (vi: "x applies to the next command only).
func (m *vimode) consumeReg() rune { r := m.reg; m.reg = 0; return r }

func (m *vimode) startOperator(op rune) {
	m.op = op
	m.opCount = m.count
	m.opReg = m.consumeReg()
	m.count = 0
	m.haveCount = false
}

// doMotion computes and applies a motion key, as either cursor movement or the
// target of a pending operator.
func (m *vimode) doMotion(e *Engine, key rune, charArg rune) {
	// cw / cW act like ce / cE when the cursor is on a non-blank: the change
	// stops at the end of the word rather than swallowing trailing whitespace.
	if m.op == 'c' && (key == 'w' || key == 'W') && e.scr.classAt(e.scr.cursor) != clBlank {
		if key == 'w' {
			key = 'e'
		} else {
			key = 'E'
		}
	}

	explicit := m.haveCount
	total := effCount(m.count)
	if m.op != 0 {
		total = effCount(m.opCount) * effCount(m.count)
		if m.haveCount || m.opCount > 0 {
			explicit = true
		}
	}
	mot, ok := e.computeMotion(key, total, explicit, charArg)
	m.count = 0
	m.haveCount = false
	m.applyMotionOrOp(e, mot, ok)
}

func (m *vimode) applyMotionOrOp(e *Engine, mot motion, ok bool) {
	if !ok {
		e.fe.Bell()
		m.op, m.opCount, m.opReg = 0, 0, 0
		return
	}
	if m.op != 0 {
		op, reg := m.op, m.opReg
		m.op, m.opCount, m.opReg = 0, 0, 0
		m.operate(e, op, reg, mot)
		return
	}
	e.scr.cursor = mot.to
	e.scr.clampCursor()
}

// charArg handles the character following an f/F/t/T/r/m/`/'/Z/[/]/@/z command.
func (m *vimode) charArg(e *Engine, ev KeyEvent) {
	key := m.pending
	m.pending = 0
	c := ev.Rune
	if ev.Key == KeyEnter {
		c = '\r'
	}
	if ev.Key == KeyEscape {
		m.zLine = 0
		m.zCount2, m.zCount2Set = 0, false
		return // cancel
	}
	if c == 0 {
		return
	}
	switch key {
	case 'f', 'F', 't', 'T':
		m.findCmd, m.findChar = key, c
		m.doMotion(e, key, c)
	case 'r':
		total := effCount(m.count)
		m.count, m.haveCount = 0, false
		e.replaceChar(c, total, m)
	case 'm':
		e.scr.marks.Set(c, mark.Mark{Line: e.scr.cursor.Line, Col: e.scr.cursor.Col})
	case '`':
		m.doMotion(e, markCharMotion, c)
	case '\'':
		m.doMotion(e, markLineMotion, c)
	case 'Z':
		switch c {
		case 'Z': // ZZ: write if modified, then quit
			if err := e.exExecute("x"); err != nil {
				e.scr.msg, e.scr.msgKind = err.Error(), MsgError
			}
		default:
			// nvi has only ZZ; any other Z<x> (e.g. ZQ) is an unknown command.
			e.fe.Bell()
		}
	case '[':
		if c == '[' {
			m.doMotion(e, sectionBackMotion, 0)
		} else {
			e.fe.Bell()
		}
	case ']':
		if c == ']' {
			m.doMotion(e, sectionFwdMotion, 0)
		} else {
			e.fe.Bell()
		}
	case '@':
		e.execBuffer(c)
	case 'z':
		if c >= '0' && c <= '9' {
			m.zCount2 = m.zCount2*10 + int(c-'0')
			m.zCount2Set = true
			m.pending = 'z'
			return
		}
		e.screenPosition(m, c, m.zLine, m.zLine != 0, m.zCount2, m.zCount2Set)
		m.zLine = 0
		m.zCount2, m.zCount2Set = 0, false
	case '#':
		count := effCount(m.count)
		m.count, m.haveCount = 0, false
		var delta int64
		switch c {
		case '+', '#':
			delta = 1
		case '-':
			delta = -1
		default:
			e.fe.Bell()
			return
		}
		if err := e.editIncrement(delta, count); err != nil {
			e.scr.msg, e.scr.msgKind = err.Error(), MsgError
		} else {
			m.changed = true
		}
	}
}

// execBuffer implements @<buffer>: run the named register's contents as vi
// commands (nvi vi/v_at.c). @@ (or @*) repeats the last executed buffer. Only
// the named (a-z, A-Z) and numbered (1-9) buffers are valid; any other name
// (e.g. @:) is not a buffer and is a no-op, matching nvi's empty-buffer error.
func (e *Engine) execBuffer(name rune) {
	if name == '@' || name == '*' {
		if e.scr.lastAtBuf == 0 {
			e.fe.Bell()
			return
		}
		name = e.scr.lastAtBuf
	}
	if !validBufferName(name) {
		e.fe.Bell()
		return
	}
	txt := e.scr.regs.Get(name)
	if txt.Empty() {
		e.fe.Bell()
		return
	}
	e.scr.lastAtBuf = name
	for i, ln := range txt.Lines {
		if i > 0 {
			e.dispatchRune('\n')
		}
		for _, r := range ln {
			e.dispatchRune(r)
		}
	}
	// A linewise buffer ends every line with a <newline>, including the last,
	// so its replay finishes with an Enter that moves the cursor down a line
	// (nvi v_at.c, CB_LMODE). A charwise buffer joins lines without a trailer.
	if txt.Kind == register.LineWise {
		e.dispatchRune('\n')
	}
}

// validBufferName reports whether name is a buffer that @ can execute: the
// named buffers a-z / A-Z and the numbered buffers 1-9 (nvi CBNAME).
func validBufferName(name rune) bool {
	switch {
	case name >= 'a' && name <= 'z':
		return true
	case name >= 'A' && name <= 'Z':
		return true
	case name >= '1' && name <= '9':
		return true
	}
	return false
}

// screenPosition implements [line]z[window]<type> (nvi v_z.c / vs_sm_fill).
// haveLine selects count1; haveWin with win>0 sets a small scroll map (vs_crel)
// and always places the line at the top — z3., z3-, and z3<enter> are equivalent.
func (e *Engine) screenPosition(m *vimode, typ rune, line int64, haveLine bool, win int, haveWin bool) {
	s := e.scr
	target := s.cursor.Line
	if haveLine {
		if line < 1 {
			line = 1
		}
		if line > s.lineCount() {
			line = s.lineCount()
		}
		target = line
		s.cursor.Line = target
		s.clampCursor()
	}
	if haveWin && win > 0 {
		max := s.rows
		if w := s.opts.Int("window"); w > 0 && w < max {
			max = w
		}
		if win > max {
			win = max
		}
		s.minMapRows = win
		s.mapRows = win
		s.top = target
		if s.top < 1 {
			s.top = 1
		}
		return
	}
	// z without a window size resets the map to the window-option default
	// (nvi t_minrows), not necessarily the full screen.
	s.mapRows = s.windowVal()
	s.minMapRows = s.mapRows
	switch typ {
	case '\r', '\n':
		s.top = target
	case '+':
		// nvi v_z.c: [line]z+ puts the line at the top (like z<CR>); a bare z+
		// scrolls forward one screen (Z_PLUS scrolls t_rows lines, vs. ^F's
		// window-2), cursor to the new top line.
		if !haveLine {
			m.pageDown(e, s.effectiveMapRows(), true)
			return
		}
		s.top = target
	case '^':
		// nvi v_z.c: a bare z^ scrolls backward one screen (Z_CARAT, t_rows
		// lines, vs. ^B's window-2), cursor to the new bottom line. [line]z^ puts
		// the line at the bottom (the historic "previous screen" off-by-one of
		// the line form is not replicated).
		if !haveLine {
			m.pageUp(e, s.effectiveMapRows(), true)
			return
		}
		s.top = s.topForBottom(target)
	case '.':
		s.top = s.topForMiddle(target)
	case '-':
		s.top = s.topForBottom(target)
	default:
		e.fe.Bell()
		return
	}
	if s.top < 1 {
		s.top = 1
	}
}

// editKey handles single-key edit and miscellaneous commands.
func (m *vimode) editKey(e *Engine, r rune) {
	s := e.scr
	switch r {
	case ':':
		e.enterCmdline(':')
	case '/':
		e.enterCmdline('/')
	case '?':
		e.enterCmdline('?')
	case 'n':
		if err := e.repeatSearch(false); err != nil {
			s.msg, s.msgKind = err.Error(), MsgError
		}
	case 'N':
		if err := e.repeatSearch(true); err != nil {
			s.msg, s.msgKind = err.Error(), MsgError
		}
	case 'Q':
		e.enterExMode()
	case 'U':
		m.restoreLine(e)
	case 'u':
		m.doUndoCommand(e, false)
	case '.':
		m.repeatDot(e)
	case 'x':
		e.synthOperator(m, 'd', 'l')
	case 'X':
		e.synthOperator(m, 'd', 'h')
	case 'D':
		// nvi ignores a count on D (delete to end of the current line only);
		// "2D" does not also take the next line as vim would.
		m.count, m.haveCount = 0, false
		e.synthOperator(m, 'd', '$')
	case 'C':
		e.synthOperator(m, 'c', '$')
	case 'Y':
		e.synthLineOperator(m, 'y')
	case 'S':
		e.synthLineOperator(m, 'c')
	case 's':
		e.synthOperator(m, 'c', 'l')
	case 'p':
		e.put(m, true)
	case 'P':
		e.put(m, false)
	case 'J':
		e.joinLines(m)
	case '~':
		if s.opts.Bool("tildeop") {
			m.startOperator('~')
		} else {
			e.toggleCase(m)
		}
	case '&':
		if err := e.repeatSubst(); err != nil {
			s.msg, s.msgKind = err.Error(), MsgError
		} else {
			m.changed = true
		}
	case 'i':
		e.enterInsert(m, s.cursor, false, 'i')
	case 'I':
		s.cursor.Col = s.firstNonBlank(s.cursor.Line)
		e.enterInsert(m, s.cursor, false, 'I')
	case 'a':
		c := s.cursor
		if s.lineLen(c.Line) > 0 {
			c.Col++
		}
		e.enterInsert(m, c, false, 'a')
	case 'A':
		c := s.cursor
		c.Col = s.lineLen(c.Line)
		e.enterInsert(m, c, false, 'A')
	case 'o':
		e.openLine(m, true)
	case 'O':
		e.openLine(m, false)
	case 'R':
		e.enterInsert(m, s.cursor, true, 'R')
	default:
		// An unbound command key, e.g. ^? (the DEL/delete key). nvi reports this.
		s.msg, s.msgKind = keyDisplay(r)+" isn't a vi command", MsgError
		e.fe.Bell()
	}
}

// keyDisplay renders a key in vi's caret notation for messages: ^? for DEL,
// ^X for other control codes, the character itself otherwise.
func keyDisplay(r rune) string {
	switch {
	case r == 0x7f:
		return "^?"
	case r < 0x20:
		return "^" + string(rune('@'+r))
	default:
		return string(r)
	}
}

// restoreLine implements U: undo the run of change sets confined to the current
// line, restoring it to the state it had when the cursor last arrived. Like u,
// it leaves the undo direction such that a following u redoes one step.
func (m *vimode) restoreLine(e *Engine) {
	s := e.scr
	line := s.cursor.Line
	var cur undo.Pos
	undid := false
	for {
		c, ok := s.log.UndoLineOnly(line)
		if !ok {
			break
		}
		cur = c
		undid = true
	}
	if !undid {
		e.fe.Bell()
		return
	}
	s.cursor = Pos{Line: cur.Line, Col: cur.Col}
	s.clampCursor()
	s.modified = true
	m.lastStepRedo = false // a following 'u' redoes the most recent undone step
	m.dotUndo = true
}

// repeatDot replays the last buffer-changing command. A count before '.'
// overrides the count baked into the recorded command.
func (m *vimode) repeatDot(e *Engine) {
	// If the last command was an undo/redo, '.' steps again in the current
	// direction. The repeated command is 'u', which ignores any count, so a
	// leading count on '.' does a single step (matching nvi).
	if m.dotUndo {
		m.count, m.haveCount = 0, false
		m.doUndoCommand(e, true)
		return
	}
	if len(m.dot) == 0 {
		e.fe.Bell()
		return
	}
	// '.' after a numbered-register put walks the delete ring: "1p, ., . puts
	// "1, "2, "3 (historic vi). Advance the register (capped at "9) and rewrite
	// the recorded digit before replaying. dotPutReg being set guarantees the
	// dot command really is a numbered put, so the rewrite is safe.
	if m.dotPutReg >= '1' && m.dotPutReg <= '9' {
		if m.dotPutReg < '9' {
			m.dotPutReg++
		}
		setDotPutDigit(m.dot, m.dotPutReg)
	}
	events := m.dot
	if m.haveCount {
		events = overrideCount(events, m.count)
	}
	m.count, m.haveCount = 0, false

	saved := append([]KeyEvent(nil), events...)
	m.replaying = true
	for _, ev := range saved {
		m.key(e, ev)
	}
	m.replaying = false
}

// setDotPutDigit rewrites the register digit of a recorded numbered-register
// put ('"' followed by a digit) to d, so a repeated '.' advances the delete
// ring. The caller guarantees the events are such a put.
func setDotPutDigit(events []KeyEvent, d rune) {
	for i := 0; i+1 < len(events); i++ {
		if events[i].Rune == '"' {
			events[i+1].Rune = d
			return
		}
	}
}

// overrideCount strips the leading count digits of a recorded command and
// prepends the digits of n.
func overrideCount(events []KeyEvent, n int) []KeyEvent {
	i := 0
	for i < len(events) && events[i].Rune >= '1' && events[i].Rune <= '9' {
		i++
		for i < len(events) && events[i].Rune >= '0' && events[i].Rune <= '9' {
			i++
		}
		break
	}
	out := make([]KeyEvent, 0, len(events)+4)
	for _, d := range itoaRunes(n) {
		out = append(out, KeyEvent{Rune: d})
	}
	out = append(out, events[i:]...)
	return out
}

func itoaRunes(n int) []rune {
	if n <= 0 {
		return nil
	}
	var rev []rune
	for n > 0 {
		rev = append(rev, rune('0'+n%10))
		n /= 10
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

// beginChange / endChange bracket a buffer mutation as one undo unit.
func (e *Engine) beginChange() {
	e.scr.log.Begin(undo.Pos{Line: e.scr.cursor.Line, Col: e.scr.cursor.Col})
}

func (e *Engine) endChange() {
	e.scr.log.End(undo.Pos{Line: e.scr.cursor.Line, Col: e.scr.cursor.Col})
	e.scr.modified = true
	e.noteRecovery()
	// A committed buffer change starts a fresh undo group: the next 'u' should
	// undo it. vi commands also reset this in finishCommand, but changes made
	// through the ex/cmdline/filter path (e.g. !!cmd, :d) never reach
	// finishCommand, so resetting here keeps the undo direction correct for them.
	if e.vi != nil {
		e.vi.lastStepRedo = true
		e.vi.dotUndo = false
	}
}

// normalizeKey maps special keys to their command-mode rune equivalents so the
// dispatch can treat them uniformly.
func normalizeKey(ev KeyEvent) rune {
	switch ev.Key {
	case KeyLeft, KeyBackspace:
		return 'h'
	case KeyRight:
		return 'l'
	case KeyDown:
		return 'j'
	case KeyUp:
		return 'k'
	case KeyHome:
		// nvi maps the terminal's khome key to '^' (first nonblank,
		// cl/cl_term.c "go to sol"), not vim's column 0.
		return '^'
	case KeyEnd:
		return '$'
	case KeyEnter:
		return '+'
	case KeyDelete:
		return 'x'
	case KeyEscape:
		return 0 // cancel; handled by state reset
	case KeyNone:
		return ev.Rune
	}
	return ev.Rune
}
