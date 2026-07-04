package engine

// motion is the result of evaluating a vi motion: the target position plus how
// a pending operator should treat the span. linewise motions affect whole
// lines; for charwise motions, inclusive means the rune at the target is part
// of the operated span (e.g. e, f, $), exclusive means it is not (e.g. w, h, 0).
type motion struct {
	to        Pos
	linewise  bool
	inclusive bool
	// promote marks the line-oriented exclusive motions (} { ) ( ]] [[) that
	// become linewise for an operator when the motion ends in column 0 and the
	// start is at or before the line's first non-blank (POSIX vi rule).
	promote bool
	// endFlag marks a promote motion that terminated on a line boundary
	// (end-of-line/empty-line/end-of-file rather than on an in-line character).
	// nvi's sentence motions set the equivalent "cs.cs_flags != 0" condition,
	// which forces line mode for an operator when the cursor starts in column 0
	// (see v_sentence.c VM_LMODE).
	endFlag bool
}

// Synthetic motion keys for the mark motions, which carry a char argument.
const (
	markCharMotion = rune(0xE000) // `m  -> exact position
	markLineMotion = rune(0xE001) // 'm  -> first non-blank of mark's line
)

func isMotionKey(r rune) bool {
	switch r {
	case 'h', 'l', ' ', '0', '^', '$', '|',
		'w', 'b', 'e', 'W', 'B', 'E',
		'j', 'k', '+', '-', 'G', 'H', 'M', 'L',
		';', ',',
		'%', '(', ')', '{', '}', '_',
		sectionFwdMotion, sectionBackMotion:
		return true
	}
	return false
}

func lineMotion(from, to int64) motion {
	return motion{to: Pos{Line: to}, linewise: true}
}

// verticalMotion builds a j/k motion that maintains the desired display column.
// For cursor movement it sets the preserveCol flag so the desired column is not
// reset afterward.
func (e *Engine) verticalMotion(target int64) motion {
	if e.vi.op == 0 {
		e.vi.preserveCol = true
	}
	col := e.scr.maintainedCol(clampLine(e.scr, target))
	return motion{to: Pos{Line: target, Col: col}, linewise: true}
}

// computeMotion evaluates a motion key. count is the (already combined) repeat
// count; explicit reports whether a count was actually typed (matters for G/H/L);
// charArg is the target character for f/F/t/T and the mark name for the mark
// motions.
func (e *Engine) computeMotion(key rune, count int, explicit bool, charArg rune) (motion, bool) {
	s := e.scr
	cur := s.cursor

	switch key {
	case 'h':
		col := cur.Col - count
		if col < 0 {
			col = 0
		}
		return motion{to: Pos{Line: cur.Line, Col: col}}, true
	case 'l', ' ':
		col := cur.Col + count
		if max := s.lineLen(cur.Line); col > max {
			col = max
		}
		return motion{to: Pos{Line: cur.Line, Col: col}}, true
	case '0':
		return motion{to: Pos{Line: cur.Line, Col: 0}}, true
	case '^':
		col := s.firstNonBlank(cur.Line)
		// On a line with no non-blank, nvi's ^ stops on the last blank rather than
		// column 0 (firstNonBlank returns 0 for an all-blank line).
		if line := s.lineRunes(cur.Line); len(line) > 0 && col == 0 && (line[0] == ' ' || line[0] == '\t') {
			col = len(line) - 1
		}
		return motion{to: Pos{Line: cur.Line, Col: col}}, true
	case '|':
		col := count - 1
		if col < 0 {
			col = 0
		}
		return motion{to: Pos{Line: cur.Line, Col: col}}, true
	case '$':
		line := cur.Line + int64(count) - 1
		if line > s.lineCount() {
			line = s.lineCount()
		}
		col := s.lineLen(line) - 1
		if col < 0 {
			col = 0
		}
		if e.vi.op == 0 {
			e.vi.setEOL = true // the cursor sticks to EOL for following j/k
		}
		return motion{to: Pos{Line: line, Col: col}, inclusive: true}, true
	case 'j':
		target := cur.Line + int64(count)
		return e.verticalMotion(target), true
	case 'k':
		target := cur.Line - int64(count)
		return e.verticalMotion(target), true
	case '+':
		line := cur.Line + int64(count)
		return motion{to: Pos{Line: line, Col: s.firstNonBlank(line)}, linewise: true}, true
	case '-':
		line := cur.Line - int64(count)
		return motion{to: Pos{Line: line, Col: s.firstNonBlank(line)}, linewise: true}, true
	case 'G':
		line := s.lineCount()
		if explicit {
			line = int64(count)
			// An explicit line number past the last line is an error in nvi (the
			// cursor stays put), not a clamp to the last line.
			if line > s.lineCount() {
				s.msg, s.msgKind = "Movement past the end-of-file", MsgError
				return motion{}, false
			}
		}
		return motion{to: Pos{Line: line, Col: s.firstNonBlank(clampLine(s, line))}, linewise: true}, true
	case 'H':
		line := s.top + int64(count) - 1
		return motion{to: Pos{Line: line, Col: s.firstNonBlank(clampLine(s, line))}, linewise: true}, true
	case 'L':
		bottom := s.top + int64(s.effectiveMapRows()) - 1
		if bottom > s.lineCount() {
			bottom = s.lineCount()
		}
		line := bottom - int64(count) + 1
		return motion{to: Pos{Line: line, Col: s.firstNonBlank(clampLine(s, line))}, linewise: true}, true
	case 'M':
		bottom := s.top + int64(s.effectiveMapRows()) - 1
		if bottom > s.lineCount() {
			bottom = s.lineCount()
		}
		// nvi rounds the middle toward the bottom on an even displayed-line
		// count: (top+bottom+1)/2 (vs_sm_position P_MIDDLE).
		line := (s.top + bottom + 1) / 2
		return motion{to: Pos{Line: line, Col: s.firstNonBlank(clampLine(s, line))}, linewise: true}, true
	case 'w':
		return e.wordForward(cur, count, false, false), true
	case 'W':
		return e.wordForward(cur, count, true, false), true
	case 'b':
		return e.wordBack(cur, count, false), true
	case 'B':
		return e.wordBack(cur, count, true), true
	case 'e':
		return e.wordEnd(cur, count, false), true
	case 'E':
		return e.wordEnd(cur, count, true), true
	case 'f', 'F', 't', 'T':
		return e.findMotion(key, charArg, count)
	case ';':
		if e.vi.findCmd == 0 {
			return motion{}, false
		}
		return e.findMotion(e.vi.findCmd, e.vi.findChar, count)
	case ',':
		if e.vi.findCmd == 0 {
			return motion{}, false
		}
		return e.findMotion(reverseFind(e.vi.findCmd), e.vi.findChar, count)
	case '%':
		return e.matchMotion()
	case '(':
		return e.sentenceBack(count)
	case ')':
		return e.sentenceFwd(count)
	case '{':
		return e.paragraphBack(count)
	case '}':
		return e.paragraphFwd(count)
	case '_':
		return e.underscoreMotion(count)
	case sectionFwdMotion:
		return e.sectionFwd(count)
	case sectionBackMotion:
		return e.sectionBack(count)
	case markCharMotion:
		if mk, ok := s.marks.Get(charArg); ok {
			return motion{to: Pos{Line: mk.Line, Col: mk.Col}}, true
		}
		return motion{}, false
	case markLineMotion:
		if mk, ok := s.marks.Get(charArg); ok {
			return motion{to: Pos{Line: mk.Line, Col: s.firstNonBlank(clampLine(s, mk.Line))}, linewise: true}, true
		}
		return motion{}, false
	}
	return motion{}, false
}

func clampLine(s *screen, line int64) int64 {
	if line < 1 {
		return 1
	}
	if line > s.lineCount() {
		return s.lineCount()
	}
	return line
}

func reverseFind(cmd rune) rune {
	switch cmd {
	case 'f':
		return 'F'
	case 'F':
		return 'f'
	case 't':
		return 'T'
	case 'T':
		return 't'
	}
	return cmd
}

// findMotion implements f/F/t/T within the current line. f/t search forward, F/T
// backward; t/T stop one short of the target. f and t are inclusive for
// operators.
func (e *Engine) findMotion(cmd, ch rune, count int) (motion, bool) {
	s := e.scr
	line := s.lineRunes(s.cursor.Line)
	col := s.cursor.Col
	switch cmd {
	case 'f', 't':
		pos := col
		for n := 0; n < count; n++ {
			start := pos + 1
			if cmd == 't' && n == 0 {
				// 't' may already be adjacent; still advance past current.
			}
			found := -1
			for i := start; i < len(line); i++ {
				if line[i] == ch {
					found = i
					break
				}
			}
			if found < 0 {
				return motion{}, false
			}
			pos = found
		}
		target := pos
		if cmd == 't' {
			target = pos - 1
		}
		return motion{to: Pos{Line: s.cursor.Line, Col: target}, inclusive: true}, true
	case 'F', 'T':
		pos := col
		for n := 0; n < count; n++ {
			found := -1
			for i := pos - 1; i >= 0; i-- {
				if line[i] == ch {
					found = i
					break
				}
			}
			if found < 0 {
				return motion{}, false
			}
			pos = found
		}
		target := pos
		if cmd == 'T' {
			target = pos + 1
		}
		return motion{to: Pos{Line: s.cursor.Line, Col: target}}, true
	}
	return motion{}, false
}
