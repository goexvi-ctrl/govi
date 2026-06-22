package engine

// Additional motions: % (match), sentences ( ), paragraphs { }, sections
// [[ ]], and _ . These mirror nvi's vi/v_match.c, v_sentence.c, v_paragraph.c,
// and v_section.c.

// Synthetic motion keys for the two-character section commands.
const (
	sectionFwdMotion  = rune(0xE002) // ]]
	sectionBackMotion = rune(0xE003) // [[
)

func (s *screen) runeAtPos(p Pos) rune {
	ln := s.lineRunes(p.Line)
	if p.Col >= len(ln) {
		return '\n'
	}
	return ln[p.Col]
}

func isBracket(r rune) rune {
	switch r {
	case '(':
		return ')'
	case ')':
		return '('
	case '[':
		return ']'
	case ']':
		return '['
	case '{':
		return '}'
	case '}':
		return '{'
	}
	return 0
}

func isOpenBracket(r rune) bool { return r == '(' || r == '[' || r == '{' }

// matchMotion implements %: find the next bracket on the line and jump to its
// match, tracking nesting across lines. Inclusive for operators.
func (e *Engine) matchMotion() (motion, bool) {
	s := e.scr
	line := s.lineRunes(s.cursor.Line)
	start := -1
	var ch rune
	for i := s.cursor.Col; i < len(line); i++ {
		if isBracket(line[i]) != 0 {
			start = i
			ch = line[i]
			break
		}
	}
	if start < 0 {
		return motion{}, false
	}
	want := isBracket(ch)
	forward := isOpenBracket(ch)
	p := Pos{Line: s.cursor.Line, Col: start}
	depth := 0
	for {
		r := s.runeAtPos(p)
		if r == ch {
			depth++
		} else if r == want {
			depth--
			if depth == 0 {
				return motion{to: p, inclusive: true}, true
			}
		}
		var ok bool
		if forward {
			p, ok = s.stepFwd(p)
		} else {
			p, ok = s.stepBack(p)
		}
		if !ok {
			return motion{}, false
		}
	}
}

// findOpenMatch returns the position of the open bracket matching the close
// bracket at p, scanning backward with nesting. ok is false if p is not a close
// bracket or no match is found.
func (e *Engine) findOpenMatch(p Pos) (Pos, bool) {
	s := e.scr
	ch := s.runeAtPos(p)
	want := isBracket(ch)
	if want == 0 || isOpenBracket(ch) {
		return Pos{}, false
	}
	depth := 0
	for {
		r := s.runeAtPos(p)
		if r == ch {
			depth++
		} else if r == want {
			depth--
			if depth == 0 {
				return p, true
			}
		}
		np, ok := s.stepBack(p)
		if !ok {
			return Pos{}, false
		}
		p = np
	}
}

func (s *screen) isBlankLine(lno int64) bool {
	return s.lineLen(lno) == 0
}

// paragraphFwd implements }: move to the next paragraph boundary (a blank line)
// below the cursor, or the last line. Exclusive.
func (e *Engine) paragraphFwd(count int) (motion, bool) {
	s := e.scr
	lno := s.cursor.Line
	for n := 0; n < count; n++ {
		lno++
		for lno < s.lineCount() && !s.isBlankLine(lno) {
			lno++
		}
		if lno >= s.lineCount() {
			lno = s.lineCount()
			break
		}
	}
	if lno >= s.lineCount() {
		// Land at end of last line.
		last := s.lineCount()
		return motion{to: Pos{Line: last, Col: max(0, s.lineLen(last))}, promote: true}, true
	}
	return motion{to: Pos{Line: lno, Col: 0}, promote: true}, true
}

// paragraphBack implements {: move to the previous blank line above the cursor,
// or line 1. Exclusive.
func (e *Engine) paragraphBack(count int) (motion, bool) {
	s := e.scr
	lno := s.cursor.Line
	for n := 0; n < count; n++ {
		lno--
		for lno > 1 && !s.isBlankLine(lno) {
			lno--
		}
		if lno <= 1 {
			lno = 1
			break
		}
	}
	return motion{to: Pos{Line: lno, Col: 0}, promote: true}, true
}

// underscoreMotion implements _: count-1 lines down, to the first non-blank.
// Linewise.
func (e *Engine) underscoreMotion(count int) (motion, bool) {
	s := e.scr
	lno := s.cursor.Line + int64(count) - 1
	lno = clampLine(s, lno)
	return motion{to: Pos{Line: lno, Col: s.firstNonBlank(lno)}, linewise: true}, true
}

func isSentenceEnd(r rune) bool { return r == '.' || r == '!' || r == '?' }

// sentenceFwd implements ): move to the start of the next sentence. A sentence
// ends at . ! ? followed by EOL or whitespace (allowing closing )]"' ); a blank
// line is also a boundary. Exclusive.
func (e *Engine) sentenceFwd(count int) (motion, bool) {
	s := e.scr
	p := s.cursor
	for n := 0; n < count; n++ {
		p = e.sentenceFwdOnce(p)
	}
	return motion{to: p, promote: true}, true
}

func (e *Engine) sentenceFwdOnce(p Pos) Pos {
	s := e.scr
	for {
		np, ok := s.stepFwd(p)
		if !ok {
			return p
		}
		// Blank line is a boundary.
		if np.Col == 0 && s.isBlankLine(np.Line) {
			return np
		}
		prev := s.runeAtPos(p)
		if isSentenceEnd(prev) {
			// Skip any closing punctuation.
			q := np
			for {
				r := s.runeAtPos(q)
				if r == ')' || r == ']' || r == '"' || r == '\'' {
					nq, ok := s.stepFwd(q)
					if !ok {
						break
					}
					q = nq
					continue
				}
				break
			}
			// A sentence boundary requires EOL, or (vi rule) two spaces/tabs
			// after the end punctuation.
			r := s.runeAtPos(q)
			boundary := r == '\n'
			if !boundary && (r == ' ' || r == '\t') {
				if nq, ok := s.stepFwd(q); ok {
					r2 := s.runeAtPos(nq)
					boundary = r2 == ' ' || r2 == '\t' || r2 == '\n'
				}
			}
			if boundary {
				// Skip the whitespace to the start of the next sentence.
				for {
					rr := s.runeAtPos(q)
					if rr == ' ' || rr == '\t' || rr == '\n' {
						nq, ok := s.stepFwd(q)
						if !ok {
							return q
						}
						q = nq
						continue
					}
					break
				}
				return q
			}
		}
		p = np
	}
}

// sentenceBack implements (: move to the start of the current/previous sentence.
func (e *Engine) sentenceBack(count int) (motion, bool) {
	s := e.scr
	p := s.cursor
	for n := 0; n < count; n++ {
		p = e.sentenceBackOnce(p)
	}
	return motion{to: p, promote: true}, true
}

func (e *Engine) sentenceBackOnce(p Pos) Pos {
	s := e.scr
	// Step back over any leading whitespace first.
	moved := false
	for {
		np, ok := s.stepBack(p)
		if !ok {
			return Pos{Line: 1, Col: 0}
		}
		r := s.runeAtPos(np)
		if !moved && (r == ' ' || r == '\t' || r == '\n') {
			p = np
			continue
		}
		moved = true
		// Look for a sentence end before np.
		prev := s.runeAtPos(np)
		if isSentenceEnd(prev) {
			// The sentence starts after this end + whitespace.
			q := p
			for {
				rr := s.runeAtPos(q)
				if rr == ' ' || rr == '\t' || rr == '\n' || rr == ')' || rr == ']' || rr == '"' || rr == '\'' {
					nq, ok := s.stepFwd(q)
					if !ok {
						break
					}
					q = nq
					continue
				}
				break
			}
			return q
		}
		if np.Col == 0 && s.isBlankLine(np.Line) {
			return np
		}
		p = np
	}
}

func (s *screen) isSectionStart(lno int64) bool {
	ln := s.lineRunes(lno)
	if len(ln) == 0 {
		return false
	}
	return ln[0] == '{' || ln[0] == '\f'
}

// sectionFwd implements ]]: move to the next section boundary line.
func (e *Engine) sectionFwd(count int) (motion, bool) {
	s := e.scr
	lno := s.cursor.Line
	for n := 0; n < count; n++ {
		lno++
		for lno < s.lineCount() && !s.isSectionStart(lno) {
			lno++
		}
		if lno >= s.lineCount() {
			lno = s.lineCount()
			break
		}
	}
	if lno >= s.lineCount() {
		last := s.lineCount()
		return motion{to: Pos{Line: last, Col: max(0, s.lineLen(last))}, promote: true}, true
	}
	return motion{to: Pos{Line: lno, Col: 0}, promote: true}, true
}

// sectionBack implements [[: move to the previous section boundary line.
func (e *Engine) sectionBack(count int) (motion, bool) {
	s := e.scr
	lno := s.cursor.Line
	for n := 0; n < count; n++ {
		lno--
		for lno > 1 && !s.isSectionStart(lno) {
			lno--
		}
		if lno <= 1 {
			lno = 1
			break
		}
	}
	return motion{to: Pos{Line: lno, Col: 0}, promote: true}, true
}
