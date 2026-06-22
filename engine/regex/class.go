package regex

import (
	"fmt"
	"unicode"
)

// parseClass parses a [...] bracket expression. The opening '[' is at the
// current position.
func (p *parser) parseClass() (node, error) {
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
			return nil, fmt.Errorf("regex: unterminated [")
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

		c := p.next()
		// Range a-z (but a trailing '-' before ']' is literal).
		if p.peek() == '-' && p.peekAt(1) != ']' && p.peekAt(1) != 0 {
			p.next() // '-'
			hi := p.next()
			lo := c
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

func (p *parser) parsePosixName() (string, error) {
	p.next() // '['
	p.next() // ':'
	var name []rune
	for !p.eof() && p.peek() != ':' {
		name = append(name, p.next())
	}
	if !(p.peek() == ':' && p.peekAt(1) == ']') {
		return "", fmt.Errorf("regex: malformed [: :]")
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
	return nil, fmt.Errorf("regex: unknown class [:%s:]", name)
}
