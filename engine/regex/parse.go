package regex

import "fmt"

type parser struct {
	src      []rune
	pos      int
	magic    bool
	extended bool // POSIX ERE (nvi :set extended / REG_EXTENDED)
	alt      bool // BRE \| alternation (internal cscope only; not user BRE)
	ngroups  int
	closed   map[int]bool // groups whose close has been parsed (valid backref targets)
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

// atGroupClose reports whether the parser is at a group terminator:
// ERE ")", BRE "\)".
func (p *parser) atGroupClose() bool {
	if p.extended {
		return p.peek() == ')'
	}
	return p.peek() == '\\' && p.peekAt(1) == ')'
}

// atAlt reports whether the parser is at an alternation separator.
// ERE: unescaped "|". BRE: "\|" only when Options.Alt is set (cscope);
// otherwise "\|" is a literal '|' (Spencer/nvi user BRE).
func (p *parser) atAlt() bool {
	if p.extended {
		return p.peek() == '|'
	}
	return p.alt && p.peek() == '\\' && p.peekAt(1) == '|'
}

// consumeAlt advances past the alternation separator.
func (p *parser) consumeAlt() {
	if p.extended {
		p.next() // |
		return
	}
	p.next() // backslash
	p.next() // |
}

// omitNode is a parse-only marker: Spencer's repeat() DROP for {0}/{0,0}
// (ERE) or \{0\}/\{0,0\} (BRE) removes the operand from the strip. It must
// not appear in a compiled Regex.
type omitNode struct{}

func (omitNode) match(m *machine, pos int, k func(int) bool) bool {
	panic("regex: omitNode reached the matcher")
}

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
		p.consumeAlt()
		n, err := p.parseConcat(true)
		if err != nil {
			return nil, err
		}
		alts = append(alts, n)
	}
	// Spencer p_ere: REQUIRE nonempty each branch (REG_EMPTY).
	for _, a := range alts {
		if _, ok := a.(omitNode); ok {
			return nil, fmt.Errorf("empty (sub)expression")
		}
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
		// {0}/{0,0}: Spencer DROP -- skip the operand entirely.
		if _, ok := n.(omitNode); ok {
			first = false
			continue
		}
		seq = append(seq, n)
		// A leading ^ does not use up the "first simple RE" position: Spencer's
		// p_bre consumes the anchor before its loop, so what follows is still
		// first (a * there is ordinary). Same for ERE: ^ is not a repeatable atom.
		if _, isBol := n.(bolNode); !isBol {
			first = false
		}
	}
	if len(seq) == 0 {
		return omitNode{}, nil
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
	// A ^ anchor takes no repetition (Spencer: wascaret blocks *+?{).
	if _, isBol := atom.(bolNode); isBol {
		return atom, nil
	}
	// At most one repetition per atom; a second is REG_BADRPT.
	switch {
	case p.starOp():
		p.consumeStar()
		return &starNode{sub: atom}, nil
	case p.extended && p.peek() == '+':
		p.next()
		// a+ == a\{1,\} 
		return &intervalNode{sub: atom, lo: 1, hi: -1}, nil
	case p.extended && p.peek() == '?':
		p.next()
		// a? == a\{0,1\}
		return &intervalNode{sub: atom, lo: 0, hi: 1}, nil
	case p.intervalOp():
		lo, hi, err := p.parseInterval()
		if err != nil {
			return nil, err
		}
		// Spencer repeat REP(0,0): drop the operand.
		if lo == 0 && hi == 0 {
			return omitNode{}, nil
		}
		return &intervalNode{sub: atom, lo: lo, hi: hi}, nil
	}
	return atom, nil
}

// starOp reports a kleene-star operator at the cursor.
// Magic BRE/ERE: "*". Nomagic BRE/ERE: "\*" (nvi re_conv flips the sense of
// .*[ before regcomp; govi folds that into Magic).
func (p *parser) starOp() bool {
	if p.magic {
		return p.peek() == '*'
	}
	return p.peek() == '\\' && p.peekAt(1) == '*'
}

func (p *parser) consumeStar() {
	if p.magic {
		p.next()
		return
	}
	p.next()
	p.next()
}

// intervalOp reports a bound operator: BRE "\{", ERE "{" followed by a digit.
func (p *parser) intervalOp() bool {
	if p.extended {
		return p.peek() == '{' && p.peekAt(1) >= '0' && p.peekAt(1) <= '9'
	}
	return p.peek() == '\\' && p.peekAt(1) == '{'
}

// dupMax is Spencer's DUPMAX: the largest count allowed in a bound.
const dupMax = 255

// parseInterval parses \{m,n\} (BRE) or {m,n} (ERE). Error texts are Spencer's
// regerror strings (REG_BADBR, REG_EBRACE).
func (p *parser) parseInterval() (int, int, error) {
	// Consume the opener.
	if p.extended {
		p.next() // {
	} else {
		p.next() // \
		p.next() // {
	}
	badbr := func() (int, int, error) {
		if p.extended {
			for !p.eof() && p.peek() != '}' {
				p.next()
			}
			if p.eof() {
				return 0, 0, fmt.Errorf("braces not balanced")
			}
			return 0, 0, fmt.Errorf("invalid repetition count(s)")
		}
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
	if p.extended {
		if p.peek() != '}' {
			return badbr()
		}
		p.next()
	} else {
		if !(p.peek() == '\\' && p.peekAt(1) == '}') {
			return badbr()
		}
		p.next()
		p.next()
	}
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
	if p.eof() {
		return nil, fmt.Errorf("empty (sub)expression")
	}
	r := p.peek()
	switch {
	case r == '\\':
		return p.parseEscape(first)
	case p.extended && r == '(':
		return p.parseGroupERE()
	case p.extended && r == ')':
		// Unmatched ) -- Spencer REG_EPAREN (no POSIX_MISTAKE in nvi's build).
		return nil, fmt.Errorf("parentheses not balanced")
	case p.extended && (r == '+' || r == '?' || r == '|'):
		// Leading repetition / free | -- REG_BADRPT / handled by atAlt.
		if r == '|' {
			// Should be consumed by atAlt; a free | as atom is empty branch.
			return nil, fmt.Errorf("empty (sub)expression")
		}
		return nil, fmt.Errorf("repetition-operator operand invalid")
	case p.extended && r == '{':
		// "{" is ordinary unless a digit follows (then REG_BADRPT -- bound
		// with no atom).
		p.next()
		if !p.eof() && p.peek() >= '0' && p.peek() <= '9' {
			return nil, fmt.Errorf("repetition-operator operand invalid")
		}
		return &litNode{r: '{'}, nil
	case r == '*':
		// Magic: unescaped * is special. As a free atom it is REG_BADRPT
		// except as the first simple RE in BRE (historic ordinary *). ERE
		// leading * is always BADRPT. Nomagic: * is always ordinary (the
		// operator is \*; nvi re_conv flips the sense before regcomp).
		if p.magic {
			if p.extended || !first {
				return nil, fmt.Errorf("repetition-operator operand invalid")
			}
			p.next()
			return &litNode{r: '*'}, nil
		}
		p.next()
		return &litNode{r: '*'}, nil
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
		// BRE: anchor only as the first simple RE (else literal).
		// ERE: Spencer always emits OBOL for '^' (POSIX: also special after
		// '('; mid-pattern ^ is still an anchor and typically fails to match).
		if p.extended || first {
			return bolNode{}, nil
		}
		return &litNode{r: '^'}, nil
	case r == '$':
		p.next()
		// BRE: end-of-subexpression anchor (isEndAnchor).
		// ERE: Spencer always emits OEOL for '$'.
		if p.extended || p.isEndAnchor() {
			return eolNode{}, nil
		}
		return &litNode{r: '$'}, nil
	default:
		p.next()
		return &litNode{r: r}, nil
	}
}

// parseGroupERE parses an ERE "(...)" capturing group.
func (p *parser) parseGroupERE() (node, error) {
	p.next() // (
	idx := p.ngroups + 1
	p.ngroups++
	var sub node
	if p.atGroupClose() {
		// Immediately closed () is a legal empty group (Spencer skips p_ere).
		sub = emptyNode{}
	} else {
		var err error
		sub, err = p.parseAlternation(true)
		if err != nil {
			return nil, err
		}
		if _, ok := sub.(omitNode); ok {
			return nil, fmt.Errorf("empty (sub)expression")
		}
	}
	if !p.atGroupClose() {
		return nil, fmt.Errorf("parentheses not balanced")
	}
	p.next() // )
	if p.closed == nil {
		p.closed = map[int]bool{}
	}
	p.closed[idx] = true
	return &groupNode{idx: idx, sub: sub}, nil
}

// isEndAnchor reports whether the '$' just consumed is an end anchor.
func (p *parser) isEndAnchor() bool {
	if p.pos >= len(p.src) {
		return true
	}
	if p.extended {
		// End of branch: before "|" or ")".
		r := p.src[p.pos]
		return r == '|' || r == ')'
	}
	// BRE: before "\)".
	return p.src[p.pos] == '\\' && p.pos+1 < len(p.src) && p.src[p.pos+1] == ')'
}

// parseEscape parses a backslash escape.
//
// BRE: Spencer p_simp_re BACKSL switch -- groups, backrefs, \{ as BADRPT when
// unattached, vi-layer \< \>, nomagic \. \[ \*.
//
// ERE: Spencer p_ere_exp treats EVERY \X as ordinary X. The only exceptions
// we keep are the vi search-layer \< \> word boundaries (nvi re_conv rewrites
// those to [[:<:]] / [[:>:]] before regcomp, so they work under extended too).
func (p *parser) parseEscape(first bool) (node, error) {
	p.next() // consume backslash
	if p.eof() {
		return nil, fmt.Errorf(`trailing backslash (\)`)
	}
	e := p.next()

	if p.extended {
		// Vi search-layer word boundaries (nvi re_conv -> [[:<:]]/[[:>:]]).
		switch e {
		case '<':
			return wordStartNode{}, nil
		case '>':
			return wordEndNode{}, nil
		}
		// Nomagic flip (nvi re_conv): \. \[ \* regain special meaning.
		if !p.magic {
			switch e {
			case '.':
				return anyNode{}, nil
			case '[':
				return p.parseClass()
			case '*':
				// Leading \* is a repetition with no operand (ERE BADRPT).
				return nil, fmt.Errorf("repetition-operator operand invalid")
			}
		}
		// Spencer ERE: every other \X is ordinary X -- including digits
		// (ERE has no backreferences; \1 is the character '1').
		return &litNode{r: e}, nil
	}

	// ---- BRE escapes ----
	switch e {
	case '(':
		idx := p.ngroups + 1
		p.ngroups++
		var sub node
		if p.atGroupClose() {
			sub = emptyNode{}
		} else {
			var err error
			sub, err = p.parseAlternation(true)
			if err != nil {
				return nil, err
			}
			if _, ok := sub.(omitNode); ok {
				return nil, fmt.Errorf("empty (sub)expression")
			}
		}
		if !p.atGroupClose() {
			return nil, fmt.Errorf("parentheses not balanced")
		}
		p.next()
		p.next()
		if p.closed == nil {
			p.closed = map[int]bool{}
		}
		p.closed[idx] = true
		return &groupNode{idx: idx, sub: sub}, nil
	case '<':
		return wordStartNode{}, nil
	case '>':
		return wordEndNode{}, nil
	case '{':
		return nil, fmt.Errorf("repetition-operator operand invalid")
	case '}':
		return nil, fmt.Errorf("parentheses not balanced")
	case '1', '2', '3', '4', '5', '6', '7', '8', '9':
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
				return nil, fmt.Errorf("repetition-operator operand invalid")
			}
			return &litNode{r: '*'}, nil
		}
	}
	// Otherwise an escaped character is that literal character.
	return &litNode{r: e}, nil
}
