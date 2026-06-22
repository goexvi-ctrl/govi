package engine

import (
	"fmt"

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
	re, err := regex.Compile(p, regex.Options{Magic: e.scr.opts.magic, IgnoreCase: e.scr.opts.ignorecase})
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

	ws := s.opts.wrapscan

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
