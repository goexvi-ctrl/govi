package engine

import (
	"unicode"

	"govi/engine/register"
)

func orderPos(a, b Pos) (Pos, Pos) {
	if a.Line < b.Line || (a.Line == b.Line && a.Col <= b.Col) {
		return a, b
	}
	return b, a
}

func minmaxLine(a, b int64) (int64, int64) {
	if a <= b {
		return a, b
	}
	return b, a
}

// operate applies an operator (d/c/y/!/~) over the span described by mot.
func (m *vimode) operate(e *Engine, op, reg rune, mot motion) {
	s := e.scr
	// The ! filter operator always works on whole lines and defers to the
	// colon line for its command.
	if op == '!' {
		l1, l2 := minmaxLine(s.cursor.Line, mot.to.Line)
		e.startFilter(clampLine(s, l1), clampLine(s, l2))
		return
	}
	// The < and > shift operators always work on whole lines.
	if op == '>' || op == '<' {
		l1, l2 := minmaxLine(s.cursor.Line, mot.to.Line)
		l1, l2 = clampLine(s, l1), clampLine(s, l2)
		dir := 1
		if op == '<' {
			dir = -1
		}
		e.shiftLines(l1, l2, dir)
		s.cursor = Pos{Line: l1, Col: s.firstNonBlank(l1)}
		m.changed = true
		return
	}
	if mot.linewise {
		l1, l2 := minmaxLine(s.cursor.Line, mot.to.Line)
		l1, l2 = clampLine(s, l1), clampLine(s, l2)
		m.operateLines(e, op, reg, l1, l2)
		return
	}

	// nvi VM_LMODE (v_sentence.c): a sentence motion cuts whole lines when the
	// cursor starts in column 0 and the motion ended on a line boundary. The
	// column-0-target case is handled by the exclusive adjustment below.
	if mot.promote && mot.endFlag && s.cursor.Col == 0 {
		l1, l2 := minmaxLine(s.cursor.Line, mot.to.Line)
		l1, l2 = clampLine(s, l1), clampLine(s, l2)
		m.operateLines(e, op, reg, l1, l2)
		return
	}

	p1, p2 := orderPos(s.cursor, mot.to)
	if mot.inclusive {
		p2.Col++
	} else if p2.Col == 0 && p2.Line > p1.Line {
		// Exclusive-motion adjustment (POSIX vi): an exclusive motion ending in
		// column 0 is pulled back to the end of the previous line, so e.g. dw on
		// a line's last word does not swallow the newline.
		p2.Line--
		p2.Col = s.lineLen(p2.Line)
		// For the line-oriented motions, if the start is at or before the first
		// non-blank the whole span becomes linewise.
		if mot.promote && p1.Col <= s.firstNonBlank(p1.Line) {
			m.operateLines(e, op, reg, p1.Line, p2.Line)
			return
		}
	}
	m.operateChars(e, op, reg, p1, p2)
}

func (m *vimode) operateLines(e *Engine, op, reg rune, l1, l2 int64) {
	s := e.scr
	txt := e.collectLines(l1, l2)
	txt.Kind = register.LineWise
	switch op {
	case 'y':
		s.regs.StoreYank(reg, txt)
		s.cursor = Pos{Line: l1, Col: s.firstNonBlank(l1)}
	case 'd':
		e.beginChange()
		s.regs.StoreDelete(reg, txt)
		e.deleteLines(l1, l2)
		e.endChange()
		tl := clampLine(s, l1)
		s.cursor = Pos{Line: tl, Col: s.firstNonBlank(tl)}
		m.changed = true
	case 'c':
		e.beginChange()
		s.regs.StoreDelete(reg, txt)
		e.deleteLines(l1, l2)
		// Leave one empty line to type into.
		e.insertEmptyLineAt(l1)
		s.cursor = Pos{Line: l1, Col: 0}
		m.startInsert(e, s.cursor, false, 'c')
	case '~':
		e.beginChange()
		for ln := l1; ln <= l2; ln++ {
			line := s.lineRunes(ln)
			nl := cloneR(line)
			for i := range nl {
				nl[i] = toggleCaseRune(nl[i])
			}
			s.setLine(ln, nl)
		}
		e.endChange()
		s.cursor = Pos{Line: l1, Col: s.firstNonBlank(l1)}
		m.changed = true
	}
}

func (m *vimode) operateChars(e *Engine, op, reg rune, p1, p2 Pos) {
	s := e.scr
	txt := e.collectChars(p1, p2)
	txt.Kind = register.CharWise
	switch op {
	case 'y':
		s.regs.StoreYank(reg, txt)
		s.cursor = p1
		s.clampCursor()
	case 'd':
		e.beginChange()
		s.regs.StoreDelete(reg, txt)
		e.deleteChars(p1, p2)
		e.endChange()
		s.cursor = p1
		s.clampCursor()
		m.changed = true
	case 'c':
		e.beginChange()
		s.regs.StoreDelete(reg, txt)
		e.deleteChars(p1, p2)
		s.cursor = p1
		m.startInsert(e, p1, false, 'c')
	case '~':
		e.beginChange()
		if p1.Line == p2.Line {
			line := cloneR(s.lineRunes(p1.Line))
			for i := p1.Col; i < p2.Col && i < len(line); i++ {
				line[i] = toggleCaseRune(line[i])
			}
			s.setLine(p1.Line, line)
		} else {
			for ln := p1.Line; ln <= p2.Line; ln++ {
				line := cloneR(s.lineRunes(ln))
				lo, hi := 0, len(line)
				if ln == p1.Line {
					lo = p1.Col
				}
				if ln == p2.Line {
					hi = p2.Col
				}
				for i := lo; i < hi && i < len(line); i++ {
					line[i] = toggleCaseRune(line[i])
				}
				s.setLine(ln, line)
			}
		}
		e.endChange()
		s.cursor = p1
		s.clampCursor()
		m.changed = true
	}
}

// collectLines returns a copy of buffer lines [l1, l2] as register text.
func (e *Engine) collectLines(l1, l2 int64) register.Text {
	var lines [][]rune
	for i := l1; i <= l2; i++ {
		src := e.scr.lineRunes(i)
		cp := make([]rune, len(src))
		copy(cp, src)
		lines = append(lines, cp)
	}
	return register.Text{Kind: register.LineWise, Lines: lines}
}

// collectChars returns the characterwise text in [p1, p2) (p2 exclusive).
func (e *Engine) collectChars(p1, p2 Pos) register.Text {
	s := e.scr
	if p1.Line == p2.Line {
		line := s.lineRunes(p1.Line)
		a, b := clampRange(p1.Col, p2.Col, len(line))
		return register.Text{Kind: register.CharWise, Lines: [][]rune{cloneR(line[a:b])}}
	}
	first := s.lineRunes(p1.Line)
	a := clampIdx(p1.Col, len(first))
	lines := [][]rune{cloneR(first[a:])}
	for i := p1.Line + 1; i < p2.Line; i++ {
		lines = append(lines, cloneR(s.lineRunes(i)))
	}
	last := s.lineRunes(p2.Line)
	b := clampIdx(p2.Col, len(last))
	lines = append(lines, cloneR(last[:b]))
	return register.Text{Kind: register.CharWise, Lines: lines}
}

// deleteChars removes the characterwise span [p1, p2).
func (e *Engine) deleteChars(p1, p2 Pos) {
	s := e.scr
	if p1.Line == p2.Line {
		line := s.lineRunes(p1.Line)
		a, b := clampRange(p1.Col, p2.Col, len(line))
		nl := append(cloneR(line[:a]), line[b:]...)
		s.setLine(p1.Line, nl)
		return
	}
	first := s.lineRunes(p1.Line)
	a := clampIdx(p1.Col, len(first))
	last := s.lineRunes(p2.Line)
	b := clampIdx(p2.Col, len(last))
	merged := append(cloneR(first[:a]), last[b:]...)
	s.setLine(p1.Line, merged)
	for i := p2.Line; i > p1.Line; i-- {
		s.deleteLine(i)
	}
}

// deleteLines removes buffer lines [l1, l2]. Deleting every line leaves the
// store genuinely empty (Lines() == 0), like nvi and like a fresh no-file
// buffer: the cursor still has somewhere to be because lineCount() reports a
// phantom blank line, but a subsequent :w produces a 0-byte file rather than a
// spurious "\n" (QA-23).
func (e *Engine) deleteLines(l1, l2 int64) {
	s := e.scr
	for i := l2; i >= l1; i-- {
		s.deleteLine(i)
	}
}

func (e *Engine) insertEmptyLineAt(lno int64) {
	s := e.scr
	if lno <= 1 {
		s.insertLine(1, []rune{})
		return
	}
	s.appendLine(lno-1, []rune{})
}

// put inserts the selected register's contents relative to the cursor (p/P),
// repeated by the command count.
func (e *Engine) put(m *vimode, after bool) {
	s := e.scr
	name := m.consumeReg()
	m.putReg = name
	txt := s.regs.Get(name)
	if txt.Empty() {
		e.fe.Bell()
		return
	}
	count := effCount(m.count)
	m.count, m.haveCount = 0, false

	e.beginChange()
	var firstStart Pos
	firstSet := false
	for i := 0; i < count; i++ {
		if txt.Kind == register.LineWise {
			e.putLines(after, txt)
		} else {
			start := e.putChars(after, txt)
			if !firstSet {
				firstStart, firstSet = start, true
			}
		}
		after = afterAfterFirstPut(after, txt)
	}
	// nvi leaves the cursor on the FIRST character of a characterwise put
	// (vi/v_put.c), unlike vim/POSIX which lands on the last. Linewise put
	// already positions at the first non-blank of the first put line.
	if firstSet {
		s.cursor = firstStart
	}
	e.endChange()
	m.changed = true
}

// afterAfterFirstPut keeps repeated linewise puts stacking below; characterwise
// repeats continue from the new cursor.
func afterAfterFirstPut(after bool, txt register.Text) bool {
	if txt.Kind == register.LineWise {
		return true
	}
	return after
}

func (e *Engine) putLines(after bool, txt register.Text) {
	s := e.scr
	at := s.cursor.Line
	if !after {
		at--
	}
	for i, ln := range txt.Lines {
		s.appendLine(at+int64(i), cloneR(ln))
	}
	first := at + 1
	s.cursor = Pos{Line: first, Col: s.firstNonBlank(clampLine(s, first))}
}

// putChars inserts a characterwise register and leaves the cursor on the LAST
// inserted char so a repeated (counted) put stacks correctly; it returns the
// position of the FIRST inserted char, which put() uses for the final cursor.
func (e *Engine) putChars(after bool, txt register.Text) Pos {
	s := e.scr
	col := s.cursor.Col
	if after && s.lineLen(s.cursor.Line) > 0 {
		col++
	}
	line := s.lineRunes(s.cursor.Line)
	col = clampIdx(col, len(line))
	head, tail := cloneR(line[:col]), cloneR(line[col:])
	start := Pos{Line: s.cursor.Line, Col: col}

	if len(txt.Lines) == 1 {
		ins := txt.Lines[0]
		nl := append(append(head, cloneR(ins)...), tail...)
		s.setLine(s.cursor.Line, nl)
		s.cursor.Col = col + len(ins) - 1
		if s.cursor.Col < col {
			s.cursor.Col = col
		}
		return start
	}

	// Multi-line characterwise put.
	s.setLine(s.cursor.Line, append(head, cloneR(txt.Lines[0])...))
	insLine := s.cursor.Line
	for i := 1; i < len(txt.Lines)-1; i++ {
		s.appendLine(insLine, cloneR(txt.Lines[i]))
		insLine++
	}
	lastIdx := len(txt.Lines) - 1
	lastContent := append(cloneR(txt.Lines[lastIdx]), tail...)
	s.appendLine(insLine, lastContent)
	s.cursor = Pos{Line: insLine + 1, Col: len(txt.Lines[lastIdx])}
	return start
}

// replaceChar implements r: replace count chars under/after the cursor with c.
func (e *Engine) replaceChar(c rune, count int, m *vimode) {
	s := e.scr
	line := s.lineRunes(s.cursor.Line)
	if s.cursor.Col+count > len(line) {
		e.fe.Bell()
		return
	}
	e.beginChange()
	if c == '\r' || c == '\n' {
		// r<Enter>: replace the count target chars with a line break, splitting
		// the line. The replaced chars are dropped; text after them moves to a new
		// line and the cursor lands at its start (nvi v_replace).
		before := cloneR(line[:s.cursor.Col])
		after := cloneR(line[s.cursor.Col+count:])
		s.setLine(s.cursor.Line, before)
		s.insertLine(s.cursor.Line+1, after)
		s.cursor.Line++
		s.cursor.Col = 0
		e.endChange()
		s.clampCursor()
		m.changed = true
		return
	}
	nl := cloneR(line)
	for i := 0; i < count; i++ {
		nl[s.cursor.Col+i] = c
	}
	s.setLine(s.cursor.Line, nl)
	s.cursor.Col += count - 1
	e.endChange()
	s.clampCursor()
	m.changed = true
}

// joinLines implements J: join the current line with following lines.
func (e *Engine) joinLines(m *vimode) {
	s := e.scr
	joins := 1
	if m.haveCount && m.count > 1 {
		joins = m.count - 1
	}
	m.count, m.haveCount = 0, false

	e.beginChange()
	for k := 0; k < joins; k++ {
		if s.cursor.Line >= s.store.Lines() {
			break
		}
		a := cloneR(s.lineRunes(s.cursor.Line))
		b := s.lineRunes(s.cursor.Line + 1)
		i := 0
		for i < len(b) && (b[i] == ' ' || b[i] == '\t') {
			i++
		}
		b = b[i:]
		joinCol := len(a)
		var sep []rune
		if len(a) > 0 && a[len(a)-1] != ' ' && a[len(a)-1] != '\t' && (len(b) == 0 || b[0] != ')') {
			if c := a[len(a)-1]; c == '.' || c == '?' || c == '!' {
				sep = []rune{' ', ' '}
			} else {
				sep = []rune{' '}
			}
		}
		nl := append(append(a, sep...), b...)
		s.setLine(s.cursor.Line, nl)
		s.deleteLine(s.cursor.Line + 1)
		s.cursor.Col = joinCol
	}
	e.endChange()
	s.clampCursor()
	m.changed = true
}

// toggleCase implements ~: toggle case of count chars, advancing the cursor.
func (e *Engine) toggleCase(m *vimode) {
	s := e.scr
	count := effCount(m.count)
	m.count, m.haveCount = 0, false
	line := s.lineRunes(s.cursor.Line)
	if len(line) == 0 {
		return
	}
	e.beginChange()
	nl := cloneR(line)
	col := s.cursor.Col
	for i := 0; i < count && col < len(nl); i++ {
		nl[col] = toggleCaseRune(nl[col])
		col++
	}
	s.setLine(s.cursor.Line, nl)
	s.cursor.Col = col
	e.endChange()
	s.clampCursor()
	m.changed = true
}

func toggleCaseRune(r rune) rune {
	if unicode.IsUpper(r) {
		return unicode.ToLower(r)
	}
	if unicode.IsLower(r) {
		return unicode.ToUpper(r)
	}
	return r
}

// openLine implements o/O: open a new line and enter insert mode.
func (e *Engine) openLine(m *vimode, below bool) {
	s := e.scr
	var indent []rune
	if s.opts.Bool("autoindent") && s.store.Lines() > 0 {
		indent = leadingWhitespace(s.lineRunes(s.cursor.Line))
	}
	e.beginChange()
	if s.store.Lines() == 0 {
		s.log.Insert(1, cloneR(indent))
		s.cursor = Pos{Line: 1, Col: len(indent)}
	} else if below {
		s.appendLine(s.cursor.Line, cloneR(indent))
		s.cursor = Pos{Line: s.cursor.Line + 1, Col: len(indent)}
	} else {
		s.insertLine(s.cursor.Line, cloneR(indent))
		s.cursor = Pos{Line: s.cursor.Line, Col: len(indent)}
	}
	m.startInsert(e, s.cursor, false, map[bool]rune{true: 'o', false: 'O'}[below])
}

// leadingWhitespace returns a copy of the run of spaces/tabs at the start of a
// line, used for autoindent.
func leadingWhitespace(line []rune) []rune {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return cloneR(line[:i])
}

// synthOperator runs op over the motion given by motionKey, honoring the count.
// It backs x/X/D/C/s.
func (e *Engine) synthOperator(m *vimode, op, motionKey rune) {
	total := effCount(m.count)
	m.count, m.haveCount = 0, false
	mot, ok := e.computeMotion(motionKey, total, true, 0)
	if !ok {
		e.fe.Bell()
		return
	}
	reg := m.consumeReg()
	m.operate(e, op, reg, mot)
}

// synthLineOperator runs a linewise op over count lines from the cursor. It
// backs Y and S.
func (e *Engine) synthLineOperator(m *vimode, op rune) {
	total := effCount(m.count)
	m.count, m.haveCount = 0, false
	l2 := e.scr.cursor.Line + int64(total) - 1
	reg := m.consumeReg()
	m.operate(e, op, reg, lineMotion(e.scr.cursor.Line, l2))
}

func cloneR(r []rune) []rune {
	out := make([]rune, len(r))
	copy(out, r)
	return out
}

func clampIdx(i, n int) int {
	if i < 0 {
		return 0
	}
	if i > n {
		return n
	}
	return i
}

func clampRange(a, b, n int) (int, int) {
	a = clampIdx(a, n)
	b = clampIdx(b, n)
	if a > b {
		a, b = b, a
	}
	return a, b
}
