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

// fileInfo implements ^G: show the file name, modified flag, and position.
func (e *Engine) fileInfo() {
	s := e.scr
	name := s.name
	if name == "" {
		name = "[No file]"
	}
	mod := ""
	if s.modified {
		mod = " [Modified]"
	}
	n := s.lineCount()
	pct := int64(0)
	if n > 0 {
		pct = (s.cursor.Line * 100) / n
	}
	s.msg = fmt.Sprintf("%q%s: line %d of %d [%d%%]", name, mod, s.cursor.Line, n, pct)
	s.msgKind = MsgInfo
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

// searchCurrentWord implements ^A: search for the word under the cursor as a
// whole word. opposite searches backward.
func (e *Engine) searchCurrentWord(opposite bool) error {
	word := e.scr.wordAt(e.scr.cursor.Line, e.scr.cursor.Col)
	if word == "" {
		return fmt.Errorf("Cursor not in a word")
	}
	pattern := `\<` + regexEscape(word) + `\>`
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
