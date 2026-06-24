package engine

import (
	"govi/engine/mark"
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
	insertEnter Pos    // where insert began
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
}

// commandKey dispatches one key in command mode.
func (m *vimode) commandKey(e *Engine, ev KeyEvent) {
	s := e.scr
	s.msg, s.msgKind = "", MsgNone

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
		mot := lineMotion(s.cursor.Line, s.cursor.Line+int64(total)-1)
		m.operate(e, op, reg, mot)
		return
	}

	switch r {
	case '"':
		m.awaitReg = true
		return
	case 'd', 'c', 'y', '!', '>', '<':
		m.startOperator(r)
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

// ctrlKey handles control-key commands (scrolling, movement aliases, info).
func (m *vimode) ctrlKey(e *Engine, r rune) {
	s := e.scr
	count := effCount(m.count)
	switch r {
	case 'f': // forward a screen
		m.moveVertical(e, s.cursor.Line+int64(max(1, s.rows-2)))
	case 'b': // back a screen
		m.moveVertical(e, s.cursor.Line-int64(max(1, s.rows-2)))
	case 'd': // down half a screen
		m.moveVertical(e, s.cursor.Line+int64(max(1, s.rows/2)))
	case 'u': // up half a screen
		m.moveVertical(e, s.cursor.Line-int64(max(1, s.rows/2)))
	case 'e': // scroll down one line
		s.top++
	case 'y': // scroll up one line
		if s.top > 1 {
			s.top--
		}
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
	case 'r': // redraw the screen (no-op here; the frontend repaints each input)
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

func (m *vimode) startOperator(op rune) {
	m.op = op
	m.opCount = m.count
	m.opReg = m.reg
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
		case 'Q': // ZQ: quit without writing
			e.removeRecovery()
			e.quit = true
		default:
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
		e.screenPosition(c, m.zLine, m.zLine != 0, m.zCount2, m.zCount2Set)
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
// commands.
func (e *Engine) execBuffer(name rune) {
	txt := e.scr.regs.Get(name)
	if txt.Empty() {
		e.fe.Bell()
		return
	}
	for i, ln := range txt.Lines {
		if i > 0 {
			e.dispatchRune('\n')
		}
		for _, r := range ln {
			e.dispatchRune(r)
		}
	}
}

// screenPosition implements [line]z[window]<type> (nvi v_z.c / vs_sm_fill).
// haveLine selects count1; haveWin with win>0 sets a small scroll map (vs_crel)
// and always places the line at the top — z3., z3-, and z3<enter> are equivalent.
func (e *Engine) screenPosition(typ rune, line int64, haveLine bool, win int, haveWin bool) {
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
	s.mapRows = s.rows
	s.minMapRows = s.rows
	switch typ {
	case '\r', '\n', '+':
		s.top = target
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
		return '0'
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
