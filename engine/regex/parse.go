package regex

import "fmt"

type parser struct {
	src     []rune
	pos     int
	magic   bool
	alt     bool // \| alternation (internal cscope patterns only; see Options.Alt)
	ngroups int
	closed  map[int]bool // groups whose \) has been parsed (valid backref targets)
}

func (p *parser) eof() bool { return p.pos >= len(p.src) }
func (p *parser) peek() rune {
	if p.eof() {
		return 0
	}
	return p.src[p.pos]
}
func (p *parser) peekAt(i int) rune {
	if p.pos+i >= len(p.src) {
		return 0
	}
	return p.src[p.pos+i]
}
func (p *parser) next() rune { r := p.src[p.pos]; p.pos++; return r }
func (p *parser) skip(n int) { p.pos += n }

// atGroupClose reports whether the parser is at a "\)" sequence.
func (p *parser) atGroupClose() bool { return p.peek() == '\\' && p.peekAt(1) == ')' }

// atAlt reports whether the parser is at a "\|" alternation separator. For
// user patterns this is always false: POSIX BRE has no alternation, and in
// Spencer's regcomp (nvi's) a \| is simply an escaped ordinary character,
// matching a literal '|' (BRE \| is a GNU/vim extension). Only the internal
// cscope patterns compile with alternation enabled (Options.Alt).
func (p *parser) atAlt() bool { return p.alt && p.peek() == '\\' && p.peekAt(1) == '|' }

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
		// A leading ^ does not use up the "first simple RE" position: Spencer's
		// p_bre consumes the anchor before its loop, so what follows is still
		// first (a * there is ordinary).
		if _, isBol := n.(bolNode); !isBol {
			first = false
		}
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
	// A ^ anchor takes no repetition: in Spencer's BRE the ^ is consumed by
	// p_bre itself, so a following * begins the first simple RE, where it is
	// an ordinary character ("^*a" matches "*a" at the start of a line).
	if _, isBol := atom.(bolNode); isBol {
		return atom, nil
	}
	// At most one repetition per simple RE (Spencer p_simp_re): a second
	// * or \{ becomes the next piece's atom, where it is REG_BADRPT.
	switch {
	case p.magic && p.peek() == '*':
		p.next()
		return &starNode{sub: atom}, nil
	case !p.magic && p.peek() == '\\' && p.peekAt(1) == '*':
		p.next()
		p.next()
		return &starNode{sub: atom}, nil
	case p.peek() == '\\' && p.peekAt(1) == '{':
		p.next()
		p.next()
		lo, hi, err := p.parseInterval()
		if err != nil {
			return nil, err
		}
		return &intervalNode{sub: atom, lo: lo, hi: hi}, nil
	}
	return atom, nil
}

// dupMax is Spencer's DUPMAX: the largest count allowed in a \{m,n\} bound.
const dupMax = 255

// parseInterval parses the body of a \{m,n\} bound. Error texts are Spencer's
// regerror strings (REG_BADBR, REG_EBRACE), which is what nvi displays.
func (p *parser) parseInterval() (int, int, error) {
	badbr := func() (int, int, error) {
		// Spencer's error heuristic: skip ahead to the closing \}; a missing
		// close brace is EBRACE, anything else wrong in the body is BADBR.
		for !p.eof() && !(p.peek() == '\\' && p.peekAt(1) == '}') {
			p.next()
		}
		if p.eof() {
			return 0, 0, fmt.Errorf("braces not balanced")
		}
		return 0, 0, fmt.Errorf("invalid repetition count(s)")
	}
	lo, hadLo := p.parseInt()
	if !hadLo || lo > dupMax {
		return badbr()
	}
	hi := lo
	if p.peek() == ',' {
		p.next()
		if n, had := p.parseInt(); had {
			if n > dupMax || lo > n {
				return badbr()
			}
			hi = n
		} else {
			hi = -1 // unbounded
		}
	}
	if !(p.peek() == '\\' && p.peekAt(1) == '}') {
		return badbr()
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
		return p.parseEscape(first)
	case r == '*' && p.magic && !first:
		// A * that is neither the first simple RE nor attached to an atom
		// follows a repetition ("a**"): Spencer REG_BADRPT.
		return nil, fmt.Errorf("repetition-operator operand invalid")
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

// isEndAnchor reports whether the '$' just consumed is an end anchor: at the
// end of the pattern, or right before a group's \) (Spencer's p_bre treats a
// trailing $ of each subexpression as the anchor).
func (p *parser) isEndAnchor() bool {
	// p.pos points just past '$'.
	if p.pos >= len(p.src) {
		return true
	}
	return p.src[p.pos] == '\\' && p.pos+1 < len(p.src) && p.src[p.pos+1] == ')'
}

// parseEscape parses a backslash escape. Error texts are Spencer's regerror
// strings; the cases and their outcomes mirror p_simp_re's BACKSL switch.
func (p *parser) parseEscape(first bool) (node, error) {
	p.next() // consume backslash
	if p.eof() {
		return nil, fmt.Errorf(`trailing backslash (\)`)
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
			return nil, fmt.Errorf("parentheses not balanced")
		}
		p.next()
		p.next()
		if p.closed == nil {
			p.closed = map[int]bool{}
		}
		p.closed[idx] = true // the group is now a valid backref target
		return &groupNode{idx: idx, sub: sub}, nil
	// \< \> (word boundaries) are the one vi search-layer rewrite folded into
	// this self-contained port (nvi re_conv); Spencer's core spells word
	// boundaries only as [[:<:]] / [[:>:]] (also accepted, see parseClass).
	// There are deliberately no \n or \t escapes: POSIX regex (and so nvi)
	// treats an escaped ordinary character as that literal character, so \t
	// matches the letter t.  Tab/newline atoms are vim regex.
	case '<':
		return wordStartNode{}, nil
	case '>':
		return wordEndNode{}, nil
	case '{':
		// A \{ not attached to an atom (parsePiece consumes the attached
		// ones): Spencer BACKSL|'{' is REG_BADRPT, even pattern-first.
		return nil, fmt.Errorf("repetition-operator operand invalid")
	case '}':
		// A stray \} is REG_EPAREN in Spencer (BACKSL|'}').
		return nil, fmt.Errorf("parentheses not balanced")
	case '1', '2', '3', '4', '5', '6', '7', '8', '9':
		// A backreference is valid only to a group that has already been closed
		// (matching Spencer/nvi, which error on \1 before \(...\), self-reference
		// inside a group, or a reference to a nonexistent group).
		idx := int(e - '0')
		if !p.closed[idx] {
			return nil, fmt.Errorf("invalid backreference number")
		}
		return &backrefNode{idx: idx}, nil
	}
	// In nomagic mode, \. \[ \* take on their special meaning.
	if !p.magic {
		switch e {
		case '.':
			return anyNode{}, nil
		case '[':
			return p.parseClass()
		case '*':
			if !first {
				// Same as a magic "a**": a repetition with no operand.
				return nil, fmt.Errorf("repetition-operator operand invalid")
			}
			return &litNode{r: '*'}, nil
		}
	}
	// Otherwise an escaped character is that literal character.
	return &litNode{r: e}, nil
}
