package engine

import (
	"fmt"
	"strings"

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
	re, err := regex.Compile(p, regex.Options{Magic: e.scr.opts.Bool("magic"), IgnoreCase: e.scr.opts.Bool("ignorecase")})
	if err != nil {
		return nil, err
	}
	e.scr.lastPattern = p
	return re, nil
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
		return 0, fmt.Errorf("Pattern not found: %s", e.scr.lastPattern)
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
	pos, ok := e.searchFrom(re, e.scr.cursor, dir)
	if !ok {
		return fmt.Errorf("Pattern not found: %s", e.scr.lastPattern)
	}
	e.scr.cursor = pos
	e.scr.clampCursor()
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
		e.scr.cursor = target
		e.scr.clampCursor()
		return nil
	}
	// Apply the deferred operator over [searchStart, target]. A search with no
	// offset is an exclusive characterwise motion; a line offset makes it
	// linewise (nvi).
	e.scr.cursor = m.searchStart
	mot := motion{to: target, linewise: linewise}
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
			return Pos{}, false, fmt.Errorf("Pattern not found: %s", s.lastPattern)
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

// repeatSearch implements n (same direction) and N (opposite).
func (e *Engine) repeatSearch(opposite bool) error {
	if e.scr.lastPattern == "" {
		return fmt.Errorf("No previous search pattern")
	}
	re, err := e.compilePattern("")
	if err != nil {
		return err
	}
	dir := e.scr.lastSearchDir
	if opposite {
		if dir == searchFwd {
			dir = searchBack
		} else {
			dir = searchFwd
		}
	}
	pos, ok := e.searchFrom(re, e.scr.cursor, dir)
	if !ok {
		return fmt.Errorf("Pattern not found: %s", e.scr.lastPattern)
	}
	e.scr.cursor = pos
	e.scr.clampCursor()
	return nil
}
