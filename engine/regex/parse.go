package regex

import "fmt"

type parser struct {
	src     []rune
	pos     int
	magic   bool
	ngroups int
	closed  map[int]bool // groups whose \) has been parsed (valid backref targets)
}

func (p *parser) eof() bool  { return p.pos >= len(p.src) }
func (p *parser) peek() rune { if p.eof() { return 0 }; return p.src[p.pos] }
func (p *parser) peekAt(i int) rune {
	if p.pos+i >= len(p.src) {
		return 0
	}
	return p.src[p.pos+i]
}
func (p *parser) next() rune { r := p.src[p.pos]; p.pos++; return r }
func (p *parser) skip(n int)  { p.pos += n }

// atGroupClose reports whether the parser is at a "\)" sequence.
func (p *parser) atGroupClose() bool { return p.peek() == '\\' && p.peekAt(1) == ')' }

// atAlt reports whether the parser is at a "\|" alternation separator.
func (p *parser) atAlt() bool { return p.peek() == '\\' && p.peekAt(1) == '|' }

func (p *parser) parseAlternation(atStart bool) (node, error) {
	first, err := p.parseConcat(atStart)
	if err != nil {
		return nil, err
	}
	if !p.atAlt() {
		return first, nil
	}
	alts := []node{first}
	for p.atAlt() {
		p.next() // backslash
		p.next() // |
		n, err := p.parseConcat(true)
		if err != nil {
			return nil, err
		}
		alts = append(alts, n)
	}
	return &altNode{alts: alts}, nil
}

func (p *parser) parseConcat(atStart bool) (node, error) {
	var seq []node
	first := atStart
	for !p.eof() && !p.atGroupClose() && !p.atAlt() {
		n, err := p.parsePiece(first)
		if err != nil {
			return nil, err
		}
		seq = append(seq, n)
		first = false
	}
	if len(seq) == 1 {
		return seq[0], nil
	}
	return &concatNode{nodes: seq}, nil
}

func (p *parser) parsePiece(first bool) (node, error) {
	atom, err := p.parseAtom(first)
	if err != nil {
		return nil, err
	}
	// Quantifiers.
	for {
		switch {
		case p.magic && p.peek() == '*':
			p.next()
			atom = &starNode{sub: atom}
		case !p.magic && p.peek() == '\\' && p.peekAt(1) == '*':
			p.next()
			p.next()
			atom = &starNode{sub: atom}
		case p.peek() == '\\' && p.peekAt(1) == '{':
			p.next()
			p.next()
			lo, hi, err := p.parseInterval()
			if err != nil {
				return nil, err
			}
			atom = &intervalNode{sub: atom, lo: lo, hi: hi}
		default:
			return atom, nil
		}
	}
}

func (p *parser) parseInterval() (int, int, error) {
	lo, hadLo := p.parseInt()
	if !hadLo {
		return 0, 0, fmt.Errorf("regex: bad interval")
	}
	hi := lo
	if p.peek() == ',' {
		p.next()
		if n, had := p.parseInt(); had {
			hi = n
		} else {
			hi = -1 // unbounded
		}
	}
	if !(p.peek() == '\\' && p.peekAt(1) == '}') {
		return 0, 0, fmt.Errorf("regex: unterminated interval")
	}
	p.next()
	p.next()
	return lo, hi, nil
}

func (p *parser) parseInt() (int, bool) {
	start := p.pos
	n := 0
	for !p.eof() && p.peek() >= '0' && p.peek() <= '9' {
		n = n*10 + int(p.next()-'0')
	}
	return n, p.pos > start
}

func (p *parser) parseAtom(first bool) (node, error) {
	r := p.peek()
	switch {
	case r == '\\':
		return p.parseEscape()
	case r == '.':
		p.next()
		if p.magic {
			return anyNode{}, nil
		}
		return &litNode{r: '.'}, nil
	case r == '[':
		if p.magic {
			return p.parseClass()
		}
		p.next()
		return &litNode{r: '['}, nil
	case r == '^':
		p.next()
		if first {
			return bolNode{}, nil
		}
		return &litNode{r: '^'}, nil
	case r == '$':
		p.next()
		if p.isEndAnchor() {
			return eolNode{}, nil
		}
		return &litNode{r: '$'}, nil
	default:
		p.next()
		return &litNode{r: r}, nil
	}
}

// isEndAnchor reports whether the '$' just consumed position is an end anchor:
// at end of pattern, or right before \) or \|.
func (p *parser) isEndAnchor() bool {
	// p.pos points just past '$'.
	if p.pos >= len(p.src) {
		return true
	}
	if p.src[p.pos] == '\\' && p.pos+1 < len(p.src) {
		n := p.src[p.pos+1]
		return n == ')' || n == '|'
	}
	return false
}

func (p *parser) parseEscape() (node, error) {
	p.next() // consume backslash
	if p.eof() {
		return &litNode{r: '\\'}, nil
	}
	e := p.next()
	switch e {
	case '(':
		idx := p.ngroups + 1
		p.ngroups++
		sub, err := p.parseAlternation(true)
		if err != nil {
			return nil, err
		}
		if !p.atGroupClose() {
			return nil, fmt.Errorf("regex: unmatched \\(")
		}
		p.next()
		p.next()
		if p.closed == nil {
			p.closed = map[int]bool{}
		}
		p.closed[idx] = true // the group is now a valid backref target
		return &groupNode{idx: idx, sub: sub}, nil
	// \< \> (word boundaries) and \n \t (escapes) are vi search-layer constructs
	// folded into this self-contained port; Spencer's core spells word
	// boundaries only as [[:<:]] / [[:>:]] (also accepted, see parseClass).
	case '<':
		return wordStartNode{}, nil
	case '>':
		return wordEndNode{}, nil
	case '1', '2', '3', '4', '5', '6', '7', '8', '9':
		// A backreference is valid only to a group that has already been closed
		// (matching Spencer/nvi, which error on \1 before \(...\), self-reference
		// inside a group, or a reference to a nonexistent group).
		idx := int(e - '0')
		if !p.closed[idx] {
			return nil, fmt.Errorf(`regex: \%d: invalid back reference`, idx)
		}
		return &backrefNode{idx: idx}, nil
	case 'n':
		return &litNode{r: '\n'}, nil
	case 't':
		return &litNode{r: '\t'}, nil
	}
	// In nomagic mode, \. \[ \* take on their special meaning.
	if !p.magic {
		switch e {
		case '.':
			return anyNode{}, nil
		case '[':
			return p.parseClass()
		}
	}
	// Otherwise an escaped character is that literal character.
	return &litNode{r: e}, nil
}
