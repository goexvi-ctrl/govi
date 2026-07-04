package regex

import (
	"fmt"
	"unicode"
)

// parseClass parses a [...] bracket expression. The opening '[' is at the
// current position.
func (p *parser) parseClass() (node, error) {
	// Spencer's "Dept of Truly Sickening Special-Case Kludges": [[:<:]] and
	// [[:>:]] are not character classes at all but word-boundary anchors,
	// equivalent to \< and \>. nvi (the regex it uses) supports these, so we do
	// too.
	if p.peek() == '[' && p.peekAt(1) == '[' && p.peekAt(2) == ':' &&
		p.peekAt(4) == ':' && p.peekAt(5) == ']' && p.peekAt(6) == ']' {
		switch p.peekAt(3) {
		case '<':
			p.skip(7)
			return wordStartNode{}, nil
		case '>':
			p.skip(7)
			return wordEndNode{}, nil
		}
	}

	p.next() // '['
	neg := false
	if p.peek() == '^' {
		neg = true
		p.next()
	}
	var preds []func(rune) bool
	i := 0
	for {
		if p.eof() {
			return nil, fmt.Errorf("brackets ([ ]) not balanced")
		}
		r := p.peek()
		if r == ']' && i > 0 {
			p.next()
			break
		}
		i++

		// POSIX class [:name:].
		if r == '[' && p.peekAt(1) == ':' {
			name, err := p.parsePosixName()
			if err != nil {
				return nil, err
			}
			f, err := posixClass(name)
			if err != nil {
				return nil, err
			}
			preds = append(preds, f)
			continue
		}

		c, err := p.parseClassElement()
		if err != nil {
			return nil, err
		}
		// Range a-z (but a trailing '-' before ']' is literal).
		if p.peek() == '-' && p.peekAt(1) != ']' && p.peekAt(1) != 0 {
			p.next() // '-'
			hi, err := p.parseClassElement()
			if err != nil {
				return nil, err
			}
			lo := c
			if lo > hi {
				// Spencer REG_ERANGE, e.g. [z-a].
				return nil, fmt.Errorf("invalid character range")
			}
			preds = append(preds, func(ch rune) bool { return ch >= lo && ch <= hi })
		} else {
			lit := c
			preds = append(preds, func(ch rune) bool { return ch == lit })
		}
	}
	pred := func(ch rune) bool {
		for _, f := range preds {
			if f(ch) {
				return true
			}
		}
		return false
	}
	return &classNode{neg: neg, pred: pred}, nil
}

// parseClassElement returns the next single-character element of a bracket
// expression: a plain rune, a [[.c.]] collating element, or a [[=c=]]
// equivalence element (Spencer p_b_symbol/p_b_coll_elem). In the C locale an
// equivalence class is just its own character. Multi-character collating
// names are not supported: REG_ECOLLATE, as Spencer reports for names not in
// its table.
func (p *parser) parseClassElement() (rune, error) {
	if p.peek() == '[' && (p.peekAt(1) == '.' || p.peekAt(1) == '=') {
		delim := p.peekAt(1)
		p.next() // '['
		p.next() // '.' or '='
		var elem []rune
		for !p.eof() && !(p.peek() == delim && p.peekAt(1) == ']') {
			elem = append(elem, p.next())
		}
		if p.eof() {
			return 0, fmt.Errorf("brackets ([ ]) not balanced")
		}
		p.next() // '.' or '='
		p.next() // ']'
		if len(elem) != 1 {
			return 0, fmt.Errorf("invalid collating element")
		}
		return elem[0], nil
	}
	return p.next(), nil
}

func (p *parser) parsePosixName() (string, error) {
	p.next() // '['
	p.next() // ':'
	var name []rune
	for !p.eof() && p.peek() != ':' {
		name = append(name, p.next())
	}
	if !(p.peek() == ':' && p.peekAt(1) == ']') {
		return "", fmt.Errorf("invalid character class")
	}
	p.next() // ':'
	p.next() // ']'
	return string(name), nil
}

func posixClass(name string) (func(rune) bool, error) {
	switch name {
	case "alpha":
		return unicode.IsLetter, nil
	case "digit":
		return unicode.IsDigit, nil
	case "alnum":
		return func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }, nil
	case "upper":
		return unicode.IsUpper, nil
	case "lower":
		return unicode.IsLower, nil
	case "space":
		return unicode.IsSpace, nil
	case "blank":
		return func(r rune) bool { return r == ' ' || r == '\t' }, nil
	case "punct":
		return unicode.IsPunct, nil
	case "cntrl":
		return unicode.IsControl, nil
	case "xdigit":
		return func(r rune) bool {
			return r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F'
		}, nil
	case "print":
		return unicode.IsPrint, nil
	case "graph":
		return unicode.IsGraphic, nil
	}
	return nil, fmt.Errorf("invalid character class")
}
