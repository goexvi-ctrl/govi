package engine

import "unicode"

// Character classes for word motions. A vi "word" is a maximal run of word
// runes (letters, digits, underscore) or a maximal run of other non-blank
// "punctuation" runes; a "WORD" (big) is any maximal run of non-blank runes.
const (
	clBlank = iota
	clWord
	clPunct
	clNL // a line boundary (the position just past a line's last rune)
)

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func (s *screen) classAt(p Pos) int {
	ln := s.lineRunes(p.Line)
	if p.Col >= len(ln) {
		return clNL
	}
	r := ln[p.Col]
	switch {
	case r == ' ' || r == '\t':
		return clBlank
	case isWordRune(r):
		return clWord
	default:
		return clPunct
	}
}

func (s *screen) classOf(p Pos, big bool) int {
	c := s.classAt(p)
	if big && c == clPunct {
		return clWord
	}
	return c
}

// stepFwd returns the next position one rune forward, crossing line boundaries.
// Positions range over col in [0, lineLen]; col == lineLen is the boundary.
func (s *screen) stepFwd(p Pos) (Pos, bool) {
	if p.Col < s.lineLen(p.Line) {
		return Pos{Line: p.Line, Col: p.Col + 1}, true
	}
	if p.Line < s.lineCount() {
		return Pos{Line: p.Line + 1, Col: 0}, true
	}
	return p, false
}

func (s *screen) stepBack(p Pos) (Pos, bool) {
	if p.Col > 0 {
		return Pos{Line: p.Line, Col: p.Col - 1}, true
	}
	if p.Line > 1 {
		prev := p.Line - 1
		return Pos{Line: prev, Col: s.lineLen(prev)}, true
	}
	return p, false
}

func (e *Engine) wordForward(cur Pos, count int, big, _ bool) motion {
	p := cur
	for i := 0; i < count; i++ {
		p = e.wordForwardOnce(p, big)
	}
	return motion{to: p}
}

func (e *Engine) wordForwardOnce(p Pos, big bool) Pos {
	s := e.scr
	c := s.classOf(p, big)
	// Skip the rest of the current word/punct run.
	if c == clWord || c == clPunct {
		for {
			np, ok := s.stepFwd(p)
			if !ok {
				return p
			}
			if s.classOf(np, big) != c {
				p = np
				break
			}
			p = np
		}
	}
	// Skip blanks and line boundaries; an empty line is itself a word.
	for {
		switch s.classOf(p, big) {
		case clBlank, clNL:
			np, ok := s.stepFwd(p)
			if !ok {
				return p
			}
			if np.Col == 0 && s.lineLen(np.Line) == 0 {
				return np
			}
			p = np
		default:
			return p
		}
	}
}

func (e *Engine) wordBack(cur Pos, count int, big bool) motion {
	p := cur
	for i := 0; i < count; i++ {
		p = e.wordBackOnce(p, big)
	}
	return motion{to: p}
}

func (e *Engine) wordBackOnce(p Pos, big bool) Pos {
	s := e.scr
	np, ok := s.stepBack(p)
	if !ok {
		return p
	}
	p = np
	for {
		c := s.classOf(p, big)
		if c != clBlank && c != clNL {
			break
		}
		np, ok := s.stepBack(p)
		if !ok {
			return p
		}
		p = np
	}
	// Walk to the start of this run.
	c := s.classOf(p, big)
	for {
		np, ok := s.stepBack(p)
		if !ok {
			return p
		}
		if s.classOf(np, big) != c {
			return p
		}
		p = np
	}
}

func (e *Engine) wordEnd(cur Pos, count int, big bool) motion {
	p := cur
	for i := 0; i < count; i++ {
		p = e.wordEndOnce(p, big)
	}
	return motion{to: p, inclusive: true}
}

func (e *Engine) wordEndOnce(p Pos, big bool) Pos {
	s := e.scr
	np, ok := s.stepFwd(p)
	if !ok {
		return p
	}
	p = np
	for {
		c := s.classOf(p, big)
		if c != clBlank && c != clNL {
			break
		}
		np, ok := s.stepFwd(p)
		if !ok {
			return p
		}
		p = np
	}
	// Walk to the end of this run.
	c := s.classOf(p, big)
	for {
		np, ok := s.stepFwd(p)
		if !ok {
			return p
		}
		if s.classOf(np, big) != c {
			return p
		}
		p = np
	}
}
