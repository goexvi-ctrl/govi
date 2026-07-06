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
// equivalence element (Spencer p_b_symbol/p_b_coll_elem, via p_b_eclass for
// '='). A named element such as [[.tab.]] or [[.comma.]] resolves through the
// character-name table (Spencer cname.h); a single character stands for
// itself; anything else is REG_ECOLLATE, as Spencer reports for names not in
// its table. In the C locale an equivalence class is just its own character.
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
		if code, ok := collatingNames[string(elem)]; ok {
			return code, nil // known name
		}
		if len(elem) == 1 {
			return elem[0], nil // single character
		}
		return 0, fmt.Errorf("invalid collating element")
	}
	return p.next(), nil
}

// collatingNames maps a collating-element name to its character, mirroring
// Spencer's cname.h character-name table. It lets [[.name.]] and [[=name=]]
// name a character symbolically, e.g. [[.tab.]] for a tab or [[.comma.]] for
// ','. A single-character element bypasses this table (see parseClassElement).
var collatingNames = map[string]rune{
	"NUL": '\x00', "SOH": '\x01', "STX": '\x02', "ETX": '\x03',
	"EOT": '\x04', "ENQ": '\x05', "ACK": '\x06', "BEL": '\x07',
	"alert": '\x07', "BS": '\x08', "backspace": '\b', "HT": '\x09',
	"tab": '\t', "LF": '\x0a', "newline": '\n', "VT": '\x0b',
	"vertical-tab": '\v', "FF": '\x0c', "form-feed": '\f', "CR": '\x0d',
	"carriage-return": '\r', "SO": '\x0e', "SI": '\x0f', "DLE": '\x10',
	"DC1": '\x11', "DC2": '\x12', "DC3": '\x13', "DC4": '\x14',
	"NAK": '\x15', "SYN": '\x16', "ETB": '\x17', "CAN": '\x18',
	"EM": '\x19', "SUB": '\x1a', "ESC": '\x1b', "IS4": '\x1c',
	"FS": '\x1c', "IS3": '\x1d', "GS": '\x1d', "IS2": '\x1e',
	"RS": '\x1e', "IS1": '\x1f', "US": '\x1f',
	"space":                ' ',
	"exclamation-mark":     '!',
	"quotation-mark":       '"',
	"number-sign":          '#',
	"dollar-sign":          '$',
	"percent-sign":         '%',
	"ampersand":            '&',
	"apostrophe":           '\'',
	"left-parenthesis":     '(',
	"right-parenthesis":    ')',
	"asterisk":             '*',
	"plus-sign":            '+',
	"comma":                ',',
	"hyphen":               '-',
	"hyphen-minus":         '-',
	"period":               '.',
	"full-stop":            '.',
	"slash":                '/',
	"solidus":              '/',
	"zero":                 '0',
	"one":                  '1',
	"two":                  '2',
	"three":                '3',
	"four":                 '4',
	"five":                 '5',
	"six":                  '6',
	"seven":                '7',
	"eight":                '8',
	"nine":                 '9',
	"colon":                ':',
	"semicolon":            ';',
	"less-than-sign":       '<',
	"equals-sign":          '=',
	"greater-than-sign":    '>',
	"question-mark":        '?',
	"commercial-at":        '@',
	"left-square-bracket":  '[',
	"backslash":            '\\',
	"reverse-solidus":      '\\',
	"right-square-bracket": ']',
	"circumflex":           '^',
	"circumflex-accent":    '^',
	"underscore":           '_',
	"low-line":             '_',
	"grave-accent":         '`',
	"left-brace":           '{',
	"left-curly-bracket":   '{',
	"vertical-line":        '|',
	"right-brace":          '}',
	"right-curly-bracket":  '}',
	"tilde":                '~',
	"DEL":                  '\x7f',
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
