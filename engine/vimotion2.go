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

func isCsBlank(r rune) bool { return r == ' ' || r == '\t' }

func isSentenceCloser(r rune) bool {
	return r == ')' || r == ']' || r == '"' || r == '\''
}

// Character-stream cursor flags, mirroring nvi's VCS (vi/getc.c).
const (
	csOK  = 0
	csEMP = 1 // empty line (blank or whitespace-only)
	csEOF = 2 // end of file
	csEOL = 3 // end of line
	csSOF = 4 // start of file
)

// vcs is a character-stream cursor that walks the buffer one rune at a time,
// reporting end-of-line, empty-line, and file boundaries as flags. It is a
// direct port of nvi's VCS / cs_* routines (vi/getc.c). Empty lines include
// whitespace-only lines, since the sentence routines ignore whitespace.
type vcs struct {
	s     *screen
	lno   int64
	cno   int
	line  []rune
	ch    rune
	flags int
}

func vcsLineEmpty(line []rune) bool {
	for _, r := range line {
		if !isCsBlank(r) {
			return false
		}
	}
	return true
}

// csInit positions a stream cursor at (lno, cno); mirrors cs_init.
func (s *screen) csInit(lno int64, cno int) *vcs {
	c := &vcs{s: s, lno: lno, cno: cno, line: s.lineRunes(lno)}
	if len(c.line) == 0 || vcsLineEmpty(c.line) {
		c.cno = 0
		c.flags = csEMP
	} else {
		c.flags = csOK
		if c.cno >= len(c.line) {
			c.cno = len(c.line) - 1
		}
		c.ch = c.line[c.cno]
	}
	return c
}

// seek repositions the cursor at a known in-text rune (flags==csOK); used to
// retry from a remembered sentence end.
func (c *vcs) seek(lno int64, cno int) {
	c.lno = lno
	c.line = c.s.lineRunes(lno)
	if len(c.line) == 0 || vcsLineEmpty(c.line) {
		c.cno = 0
		c.flags = csEMP
		return
	}
	c.flags = csOK
	if cno >= len(c.line) {
		cno = len(c.line) - 1
	}
	c.cno = cno
	c.ch = c.line[cno]
}

// next advances one rune; mirrors cs_next.
func (c *vcs) next() {
	switch c.flags {
	case csEMP, csEOL:
		c.lno++
		if c.lno > c.s.lineCount() {
			c.lno--
			c.flags = csEOF
			break
		}
		c.line = c.s.lineRunes(c.lno)
		if len(c.line) == 0 || vcsLineEmpty(c.line) {
			c.cno = 0
			c.flags = csEMP
		} else {
			c.flags = csOK
			c.cno = 0
			c.ch = c.line[0]
		}
	case csOK:
		if c.cno == len(c.line)-1 {
			c.flags = csEOL
		} else {
			c.cno++
			c.ch = c.line[c.cno]
		}
	case csEOF:
		// Movement sink.
	}
}

// prev retreats one rune; mirrors cs_prev.
func (c *vcs) prev() {
	switch c.flags {
	case csEMP, csEOL:
		if c.lno == 1 {
			c.flags = csSOF
			break
		}
		c.lno--
		c.line = c.s.lineRunes(c.lno)
		if len(c.line) == 0 || vcsLineEmpty(c.line) {
			c.cno = 0
			c.flags = csEMP
		} else {
			c.flags = csOK
			c.cno = len(c.line) - 1
			c.ch = c.line[c.cno]
		}
	case csEOF, csOK:
		if c.cno == 0 {
			if c.lno == 1 {
				c.flags = csSOF
			} else {
				c.flags = csEOL
			}
		} else {
			c.cno--
			c.ch = c.line[c.cno]
		}
	case csSOF:
		// Movement sink.
	}
}

// fblank eats forward to the next non-whitespace character; mirrors cs_fblank.
func (c *vcs) fblank() {
	for {
		c.next()
		if c.flags == csEOL || c.flags == csEMP || (c.flags == csOK && isCsBlank(c.ch)) {
			continue
		}
		break
	}
}

// bblank eats backward to the previous non-whitespace character; mirrors cs_bblank.
func (c *vcs) bblank() {
	for {
		c.prev()
		if c.flags == csEOL || c.flags == csEMP || (c.flags == csOK && isCsBlank(c.ch)) {
			continue
		}
		break
	}
}

// sentenceResult converts a stream-cursor position into a motion target.
func (e *Engine) sentenceResult(c *vcs) (motion, bool) {
	s := e.scr
	lno := c.lno
	if lno < 1 {
		lno = 1
	}
	if lno > s.lineCount() {
		lno = s.lineCount()
	}
	col := c.cno
	ll := s.lineLen(lno)
	if c.flags == csEMP || ll == 0 {
		col = 0
	} else if col >= ll {
		col = ll - 1
	} else if col < 0 {
		col = 0
	}
	return motion{to: Pos{Line: lno, Col: col}, promote: true, endFlag: c.flags != csOK}, true
}

// Sentence motion state, mirroring nvi's enum in v_sentencef.
const (
	sentNone = iota
	sentPeriod
	sentBlank
)

// sentenceFwd implements ): move forward count sentences. A sentence ends at a
// '.', '!' or '?' followed by EOL or two blanks (one tab); an empty line is a
// boundary in its own right. Ported from nvi's v_sentencef (vi/v_sentence.c).
func (e *Engine) sentenceFwd(count int) (motion, bool) {
	s := e.scr
	start := s.cursor
	cs := s.csInit(start.Line, start.Col)
	cnt := count

	// If in white-space, the next start of sentence counts as one.
	if cs.flags == csEMP || (cs.flags == csOK && isCsBlank(cs.ch)) {
		cs.fblank()
		cnt--
		if cnt == 0 {
			if start.Line != cs.lno || start.Col != cs.cno {
				return e.sentenceResult(cs)
			}
			return motion{}, false
		}
	}

	state := sentNone
	for {
		cs.next()
		if cs.flags == csEOF {
			break
		}
		if cs.flags == csEOL {
			if state == sentPeriod || state == sentBlank {
				cnt--
				if cnt == 0 {
					cs.next()
					if cs.flags == csOK && isCsBlank(cs.ch) {
						cs.fblank()
					}
					return e.sentenceResult(cs)
				}
			}
			state = sentNone
			continue
		}
		if cs.flags == csEMP { // An empty line is two sentences.
			cnt--
			if cnt == 0 {
				return e.sentenceResult(cs)
			}
			cs.fblank()
			cnt--
			if cnt == 0 {
				return e.sentenceResult(cs)
			}
			state = sentNone
			continue
		}
		switch cs.ch {
		case '.', '?', '!':
			state = sentPeriod
		case ')', ']', '"', '\'':
			if state != sentPeriod {
				state = sentNone
			}
		case ' ', '\t':
			if cs.ch == '\t' && state == sentPeriod {
				state = sentBlank
			}
			if state == sentPeriod {
				state = sentBlank
			} else {
				if state == sentBlank {
					cnt--
					if cnt == 0 {
						cs.fblank()
						return e.sentenceResult(cs)
					}
				}
				state = sentNone
			}
		default:
			state = sentNone
		}
	}

	// EOF is a movement sink, but it is an error not to have moved.
	if cs.lno == start.Line && cs.cno == start.Col {
		return motion{}, false
	}
	return e.sentenceResult(cs)
}

// sentenceBack implements (: move backward count sentences. Ported from nvi's
// v_sentenceb (vi/v_sentence.c).
func (e *Engine) sentenceBack(count int) (motion, bool) {
	s := e.scr
	start := s.cursor
	// Historic vi permitted hitting SOF repeatedly.
	if start.Line == 1 && start.Col == 0 {
		return motion{}, false
	}
	cs := s.csInit(start.Line, start.Col)
	cnt := count

	// In empty lines, skip back to the previous non-white-space character; in
	// text, skip back to the previous white-space character.
	if cs.flags == csEMP {
		cs.bblank()
		for {
			cs.prev()
			if cs.flags != csEOL {
				break
			}
		}
	} else if cs.flags == csOK && !isCsBlank(cs.ch) {
		for {
			cs.prev()
			if cs.flags != csOK || isCsBlank(cs.ch) {
				break
			}
		}
	}

	last := false
	var slno int64
	var scno int
	for {
		cs.prev()
		if cs.flags == csSOF { // SOF is a movement sink.
			break
		}
		if cs.flags == csEOL {
			last = true
			continue
		}
		doRet := false
		if cs.flags == csEMP {
			cnt--
			if cnt == 0 {
				doRet = true
			} else {
				cs.bblank()
				last = false
				continue
			}
		} else {
			switch cs.ch {
			case '.', '?', '!':
				if !last {
					last = false
					continue
				}
				cnt--
				if cnt != 0 {
					last = false
					continue
				}
				doRet = true
			case '\t':
				last = true
				continue
			default:
				last = isCsBlank(cs.ch) || isSentenceCloser(cs.ch)
				continue
			}
		}
		if !doRet {
			continue
		}

		// Move to the start of the sentence, skipping closers and blanks.
		slno = cs.lno
		scno = cs.cno
		for {
			cs.next()
			if cs.flags == csOK && isSentenceCloser(cs.ch) {
				continue
			}
			break
		}
		if cs.flags != csOK || isCsBlank(cs.ch) {
			cs.fblank()
		}

		// If we ended up where we started, an empty line may precede a real
		// sentence; if so this boundary counts, otherwise retry.
		if start.Line != cs.lno || start.Col != cs.cno {
			return e.sentenceResult(cs)
		}
		for {
			cs.prev()
			if cs.flags == csEOL || (cs.flags == csOK && isCsBlank(cs.ch)) {
				continue
			}
			break
		}
		if cs.flags == csEMP {
			return e.sentenceResult(cs)
		}
		cnt++
		cs.seek(slno, scno)
		last = false
	}
	return e.sentenceResult(cs)
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
