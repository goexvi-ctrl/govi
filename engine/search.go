package engine

import (
	"fmt"
	"strings"
	"unicode"

	"govi/engine/regex"
)

// Search support. vi searches are line-oriented: a pattern is matched within a
// line, scanning forward or backward from the cursor and wrapping around the
// file (the 'wrapscan' behavior). The last pattern and direction are remembered
// for n and N. This corresponds to nvi's common/search.c.

type searchDir int

const (
	searchFwd searchDir = iota
	searchBack
)

// compilePattern compiles p with the engine's current options. An empty pattern
// reuses the last one.
func (e *Engine) compilePattern(p string) (*regex.Regex, error) {
	if p == "" {
		if e.scr.lastPattern == "" {
			return nil, fmt.Errorf("No previous search pattern")
		}
		p = e.scr.lastPattern
	}
	magic := e.scr.opts.Bool("magic")
	// iclower (nvi re_compile): the search is case-insensitive as long as no
	// upper-case letter appears in the pattern. Scanned before ~ expansion,
	// like nvi, which computes the flags on the pattern as passed in.
	ic := e.scr.opts.Bool("ignorecase")
	if !ic && e.scr.opts.Bool("iclower") && !strings.ContainsFunc(p, unicode.IsUpper) {
		ic = true
	}
	// nvi re_conv: a ~ in a pattern stands for the last substitute replacement
	// text, spliced in verbatim before compiling. The expanded pattern is what
	// gets saved, so a later empty pattern reuses the expansion (nvi saves the
	// converted RE the same way).
	p = expandPatternTilde(p, e.scr.lastSubstRepl, magic)
	re, err := regex.Compile(p, regex.Options{Magic: magic, IgnoreCase: ic})
	if err != nil {
		// nvi re_error: msgq "RE error: %s" with the regerror text (msgq
		// supplies the trailing period).
		return nil, fmt.Errorf("RE error: %v.", err)
	}
	e.scr.lastPattern = p
	return re, nil
}

// expandPatternTilde implements nvi re_conv's ~ handling: in magic mode an
// unescaped ~ stands for the last substitute replacement text and \~ is a
// literal tilde; in nomagic the sense flips (~ literal, \~ expands). The
// replacement text is spliced in verbatim -- any regex specials in it take
// effect, as in nvi.
func expandPatternTilde(p, repl string, magic bool) string {
	if !strings.ContainsRune(p, '~') {
		return p
	}
	rs := []rune(p)
	var b strings.Builder
	for i := 0; i < len(rs); i++ {
		if rs[i] == '\\' && i+1 < len(rs) {
			if rs[i+1] == '~' {
				if magic {
					b.WriteString(`\~`) // literal tilde
				} else {
					b.WriteString(repl)
				}
			} else {
				b.WriteRune(rs[i])
				b.WriteRune(rs[i+1])
			}
			i++
			continue
		}
		if rs[i] == '~' && magic {
			b.WriteString(repl)
			continue
		}
		b.WriteRune(rs[i])
	}
	return b.String()
}

// searchFrom finds the next match of re from the given position in the given
// direction, wrapping around the file. Returns the match position.
func (e *Engine) searchFrom(re *regex.Regex, from Pos, dir searchDir) (Pos, bool) {
	s := e.scr
	n := s.lineCount()

	ws := s.opts.Bool("wrapscan")

	if dir == searchFwd {
		// Start just past the cursor on the current line, then following lines,
		// wrapping back to the start if wrapscan is set.
		for i := int64(0); i <= n; i++ {
			if e.Interrupted() {
				return Pos{}, false
			}
			lno := from.Line + i
			startCol := 0
			if i == 0 {
				startCol = from.Col + 1
			}
			if lno > n {
				if !ws {
					break
				}
				lno -= n // wrap
			}
			line := s.lineRunes(lno)
			if m, ok := re.MatchAt(line, startCol); ok {
				return Pos{Line: lno, Col: m.Start}, true
			}
		}
		return Pos{}, false
	}

	// Backward.
	for i := int64(0); i <= n; i++ {
		if e.Interrupted() {
			return Pos{}, false
		}
		lno := from.Line - i
		if lno < 1 {
			if !ws {
				break
			}
			for lno < 1 {
				lno += n
			}
		}
		line := s.lineRunes(lno)
		limit := len(line)
		if i == 0 {
			limit = from.Col - 1
		}
		if limit >= 0 {
			if m, ok := re.MatchLast(line, limit); ok {
				return Pos{Line: lno, Col: m.Start}, true
			}
		}
	}
	return Pos{}, false
}

// searchFailErr reports why a searchFrom returned no match: an interrupt (the
// user pressed ^C mid-scan) takes precedence over a genuine miss so the user
// sees "Interrupted" rather than a misleading "not found".
func (e *Engine) searchFailErr() error {
	if e.Interrupted() {
		return errInterrupted
	}
	// nvi reports just "Pattern not found" -- it does not echo the pattern.
	return fmt.Errorf("Pattern not found")
}

// searchAddr resolves an ex search line-address (/pat/ or ?pat?): it finds the
// line matching pat searching from the line adjacent to cur in dir, wrapping per
// wrapscan, and returns that line number. The current line is excluded at the
// start (it can only re-match after a wrap), matching nvi.
func (e *Engine) searchAddr(pat string, cur int64, dir searchDir) (int64, error) {
	re, err := e.compilePattern(pat)
	if err != nil {
		return 0, err
	}
	e.scr.lastSearchDir = dir
	var from Pos
	if dir == searchFwd {
		from = Pos{Line: cur, Col: len(e.scr.lineRunes(cur))} // skip the rest of cur
	} else {
		from = Pos{Line: cur, Col: 0} // backward: limit -1 skips cur
	}
	pos, ok := e.searchFrom(re, from, dir)
	if !ok {
		return 0, e.searchFailErr()
	}
	return pos.Line, nil
}

// startSearch runs a / or ? search for pattern and moves the cursor to the
// match. dir is the search direction.
func (e *Engine) startSearch(pattern string, dir searchDir) error {
	re, err := e.compilePattern(pattern)
	if err != nil {
		return err
	}
	e.scr.lastSearchDir = dir
	prev := e.scr.cursor
	pos, ok := e.searchFrom(re, e.scr.cursor, dir)
	if !ok {
		return e.searchFailErr()
	}
	e.scr.cursor = pos
	e.scr.clampCursor()
	// startSearch backs ^A (v_searchw), which is V_ABS: always set the mark.
	e.setPrevContext(prev, e.scr.cursor, absAlways)
	return nil
}

// runSearchLine handles a completed / or ? command line: it resolves the search
// (with any trailing line offset and ; chaining, nvi vi/v_search.c), then either
// applies a pending operator (d/pat, y/pat) or moves the cursor. line is the text
// after the leading prompt delimiter.
func (e *Engine) runSearchLine(line string, dir searchDir) error {
	m := e.vi
	op := m.searchOp
	m.searchOp = 0
	target, linewise, err := e.searchLine(line, dir)
	if err != nil {
		return err
	}
	if op == 0 {
		prev := e.scr.cursor
		e.scr.cursor = target
		e.scr.clampCursor()
		// / and ? are V_ABS_C: set the previous context if the line or column moved.
		e.setPrevContext(prev, e.scr.cursor, absCol)
		return nil
	}
	// Apply the deferred operator over [searchStart, target]. A search with no
	// offset is an exclusive characterwise motion; a line offset makes it
	// linewise (nvi).
	e.scr.cursor = m.searchStart
	// A search target in column 0 of a later line promotes the exclusive span to
	// linewise when the operator started at/before the first non-blank (POSIX vi
	// exclusive-linewise rule), so d/pat deletes whole lines rather than leaving
	// an empty first line.
	mot := motion{to: target, linewise: linewise, promote: true}
	m.operate(e, op, m.searchOpReg, mot)
	m.changed = false
	m.count, m.haveCount = 0, false
	return nil
}

// searchLine parses one or more ;-chained search steps with optional +/- line
// offsets and returns the final target position. linewise is true when the last
// step carried a line offset (nvi makes an offset search linewise for operators).
func (e *Engine) searchLine(line string, dir searchDir) (Pos, bool, error) {
	s := e.scr
	rs := []rune(line)
	i := 0
	from := s.cursor
	linewise := false
	for {
		delim := '/'
		if dir == searchBack {
			delim = '?'
		}
		// Read the pattern up to an unescaped delimiter.
		var pat strings.Builder
		for i < len(rs) {
			r := rs[i]
			if r == '\\' && i+1 < len(rs) {
				pat.WriteRune(r)
				pat.WriteRune(rs[i+1])
				i += 2
				continue
			}
			if r == delim {
				break
			}
			pat.WriteRune(r)
			i++
		}
		if i < len(rs) && rs[i] == delim {
			i++ // consume closing delimiter
		}
		re, err := e.compilePattern(pat.String())
		if err != nil {
			return Pos{}, false, err
		}
		s.lastSearchDir = dir
		pos, ok := e.searchFrom(re, from, dir)
		if !ok {
			return Pos{}, false, e.searchFailErr()
		}
		cur := pos
		linewise = false
		// Optional line offset: +N, -N, + or - (default 1).
		if i < len(rs) && (rs[i] == '+' || rs[i] == '-') {
			sign := rs[i]
			i++
			n := int64(0)
			hadNum := false
			for i < len(rs) && rs[i] >= '0' && rs[i] <= '9' {
				n = n*10 + int64(rs[i]-'0')
				i++
				hadNum = true
			}
			if !hadNum {
				n = 1
			}
			if sign == '-' {
				n = -n
			}
			cur = Pos{Line: clampLine(s, pos.Line+n), Col: 0}
			cur.Col = s.firstNonBlank(cur.Line)
			linewise = true
		}
		// A ; chain re-searches from the current match; the char after ; may
		// change direction.
		if i < len(rs) && rs[i] == ';' {
			i++
			if i < len(rs) && (rs[i] == '/' || rs[i] == '?') {
				if rs[i] == '?' {
					dir = searchBack
				} else {
					dir = searchFwd
				}
				i++
			}
			from = cur
			continue
		}
		return cur, linewise, nil
	}
}

// fileInfo implements ^G: show the file status (same text as :f).
func (e *Engine) fileInfo() {
	e.scr.msg = e.fileStatus()
	e.scr.msgKind = MsgInfo
}

// wordAt returns the word (sequence of word runes) covering or following column
// col on the given line, or "" if none.
func (s *screen) wordAt(lno int64, col int) string {
	line := s.lineRunes(lno)
	if len(line) == 0 {
		return ""
	}
	if col >= len(line) {
		col = len(line) - 1
	}
	// If not on a word rune, scan forward to one.
	for col < len(line) && !isWordRune(line[col]) {
		col++
	}
	if col >= len(line) {
		return ""
	}
	start := col
	for start > 0 && isWordRune(line[start-1]) {
		start--
	}
	end := col
	for end < len(line) && isWordRune(line[end]) {
		end++
	}
	return string(line[start:end])
}

// curKeyword returns the ^A search keyword starting at column col, matching
// nvi's v_curword: skip leading blanks only, then take the char under the
// cursor plus the following run of word runes. The bool reports whether the
// keyword's first char is a word rune. ok is false past end-of-line.
func (s *screen) curKeyword(lno int64, col int) (kw string, firstWord, ok bool) {
	line := s.lineRunes(lno)
	for col < len(line) && (line[col] == ' ' || line[col] == '\t') {
		col++
	}
	if col >= len(line) {
		return "", false, false
	}
	start := col
	firstWord = isWordRune(line[col])
	end := col + 1
	for end < len(line) && isWordRune(line[end]) {
		end++
	}
	return string(line[start:end]), firstWord, true
}

// searchCurrentWord implements ^A: search for the keyword under the cursor.
// A keyword starting on a word rune is matched as a whole word (\<...\>); one
// starting on a non-word rune is matched literally followed by a non-keyword
// (or end-of-line) delimiter, which makes repeated ^A idempotent. opposite
// searches backward.
func (e *Engine) searchCurrentWord(opposite bool) error {
	kw, firstWord, ok := e.scr.curKeyword(e.scr.cursor.Line, e.scr.cursor.Col)
	if !ok {
		return fmt.Errorf("Cursor not in a word")
	}
	var pattern string
	if firstWord {
		pattern = `\<` + regexEscape(kw) + `\>`
	} else {
		pattern = regexEscape(kw) + `\([^[:alnum:]_]\|$\)`
	}
	dir := searchFwd
	if opposite {
		dir = searchBack
	}
	return e.startSearch(pattern, dir)
}

// regexEscape backslash-escapes the BRE metacharacters in a literal word.
func regexEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '.', '*', '[', ']', '^', '$', '\\':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// repeatSearchTarget computes the position of the count-th n (same direction)
// or N (opposite) search repeat from the cursor, without moving it. ok is false
// when the pattern is not found; err is set only when there is no prior pattern.
func (e *Engine) repeatSearchTarget(opposite bool, count int) (Pos, bool, error) {
	if e.scr.lastPattern == "" {
		return Pos{}, false, fmt.Errorf("No previous search pattern")
	}
	re, err := e.compilePattern("")
	if err != nil {
		return Pos{}, false, err
	}
	dir := e.scr.lastSearchDir
	if opposite {
		if dir == searchFwd {
			dir = searchBack
		} else {
			dir = searchFwd
		}
	}
	if count < 1 {
		count = 1
	}
	from := e.scr.cursor
	var pos Pos
	for i := 0; i < count; i++ {
		p, ok := e.searchFrom(re, from, dir)
		if !ok {
			return Pos{}, false, nil
		}
		pos, from = p, p
	}
	return pos, true, nil
}

// repeatSearch implements n (same direction) and N (opposite).
func (e *Engine) repeatSearch(opposite bool) error {
	prev := e.scr.cursor
	pos, ok, err := e.repeatSearchTarget(opposite, 1)
	if err != nil {
		return err
	}
	if !ok {
		return e.searchFailErr()
	}
	e.scr.cursor = pos
	e.scr.clampCursor()
	// n and N are V_ABS_C: set the previous context if the line or column moved.
	e.setPrevContext(prev, e.scr.cursor, absCol)
	return nil
}

// searchRepeatMotion applies a pending operator over the span from the cursor to
// an n/N search repeat, mirroring "d/pat<CR>" used as a motion (a search repeat
// is just a search with the remembered pattern). The span is exclusive
// characterwise, like a search motion with no line offset. QA-1.
func (m *vimode) searchRepeatMotion(e *Engine, opposite bool) {
	s := e.scr
	op, reg := m.op, m.opReg
	count := effCount(m.opCount) * effCount(m.count)
	start := s.cursor
	target, ok, err := e.repeatSearchTarget(opposite, count)
	m.op, m.opCount, m.opReg = 0, 0, 0
	m.count, m.haveCount = 0, false
	if err != nil {
		s.msg, s.msgKind = err.Error(), MsgError
		return
	}
	if !ok {
		s.msg, s.msgKind = e.searchFailErr().Error(), MsgError
		return
	}
	s.cursor = start
	m.operate(e, op, reg, motion{to: target, promote: true})
	m.changed = false
}
