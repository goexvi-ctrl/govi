package regex

import "unicode"

// machine holds the input and capture state for one match attempt.
type machine struct {
	in   []rune
	ic   bool  // ignore case
	caps []int // 2*(ngroups+1) slots; -1 when unset
}

func (m *machine) eq(a, b rune) bool {
	if m.ic {
		return unicode.ToLower(a) == unicode.ToLower(b)
	}
	return a == b
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// node is one compiled regex element. match attempts to match starting at pos;
// on success it calls k with the position after this element and returns its
// result, enabling backtracking through the continuation k.
type node interface {
	match(m *machine, pos int, k func(int) bool) bool
}

type litNode struct{ r rune }

func (n *litNode) match(m *machine, pos int, k func(int) bool) bool {
	if pos < len(m.in) && m.eq(m.in[pos], n.r) {
		return k(pos + 1)
	}
	return false
}

type anyNode struct{}

func (anyNode) match(m *machine, pos int, k func(int) bool) bool {
	if pos < len(m.in) {
		return k(pos + 1)
	}
	return false
}

type classNode struct {
	neg  bool
	pred func(rune) bool
}

func (n *classNode) match(m *machine, pos int, k func(int) bool) bool {
	if pos >= len(m.in) {
		return false
	}
	ch := m.in[pos]
	matched := n.pred(ch)
	if m.ic && !matched {
		matched = n.pred(unicode.ToLower(ch)) || n.pred(unicode.ToUpper(ch))
	}
	if matched != n.neg {
		return k(pos + 1)
	}
	return false
}

type concatNode struct{ nodes []node }

func (n *concatNode) match(m *machine, pos int, k func(int) bool) bool {
	var rec func(i, pos int) bool
	rec = func(i, pos int) bool {
		if i == len(n.nodes) {
			return k(pos)
		}
		return n.nodes[i].match(m, pos, func(p int) bool { return rec(i+1, p) })
	}
	return rec(0, pos)
}

type altNode struct{ alts []node }

func (n *altNode) match(m *machine, pos int, k func(int) bool) bool {
	for _, a := range n.alts {
		if a.match(m, pos, k) {
			return true
		}
	}
	return false
}

type starNode struct{ sub node }

func (n *starNode) match(m *machine, pos int, k func(int) bool) bool {
	var rec func(pos int) bool
	rec = func(pos int) bool {
		if n.sub.match(m, pos, func(p int) bool {
			if p == pos { // no progress: stop expanding to avoid infinite loop
				return false
			}
			return rec(p)
		}) {
			return true
		}
		return k(pos) // greedy: fall back to matching fewer
	}
	return rec(pos)
}

type intervalNode struct {
	sub    node
	lo, hi int // hi == -1 means unbounded
}

func (n *intervalNode) match(m *machine, pos int, k func(int) bool) bool {
	var rec func(count, pos int) bool
	rec = func(count, pos int) bool {
		if n.hi == -1 || count < n.hi {
			if n.sub.match(m, pos, func(p int) bool {
				if p == pos && count >= n.lo {
					return false
				}
				return rec(count+1, p)
			}) {
				return true
			}
		}
		if count >= n.lo {
			return k(pos)
		}
		return false
	}
	return rec(0, pos)
}

type groupNode struct {
	idx int
	sub node
}

func (n *groupNode) match(m *machine, pos int, k func(int) bool) bool {
	o1, o2 := m.caps[2*n.idx], m.caps[2*n.idx+1]
	m.caps[2*n.idx] = pos
	if n.sub.match(m, pos, func(p int) bool {
		save := m.caps[2*n.idx+1]
		m.caps[2*n.idx+1] = p
		if k(p) {
			return true
		}
		m.caps[2*n.idx+1] = save
		return false
	}) {
		return true
	}
	m.caps[2*n.idx], m.caps[2*n.idx+1] = o1, o2
	return false
}

type backrefNode struct{ idx int }

func (n *backrefNode) match(m *machine, pos int, k func(int) bool) bool {
	if 2*n.idx+1 >= len(m.caps) {
		return false
	}
	s, e := m.caps[2*n.idx], m.caps[2*n.idx+1]
	if s < 0 || e < 0 {
		return false
	}
	length := e - s
	if pos+length > len(m.in) {
		return false
	}
	for i := 0; i < length; i++ {
		if !m.eq(m.in[pos+i], m.in[s+i]) {
			return false
		}
	}
	return k(pos + length)
}

type bolNode struct{}

func (bolNode) match(m *machine, pos int, k func(int) bool) bool {
	if pos == 0 {
		return k(pos)
	}
	return false
}

type eolNode struct{}

func (eolNode) match(m *machine, pos int, k func(int) bool) bool {
	if pos == len(m.in) {
		return k(pos)
	}
	return false
}

type wordStartNode struct{}

func (wordStartNode) match(m *machine, pos int, k func(int) bool) bool {
	cur := pos < len(m.in) && isWordRune(m.in[pos])
	prev := pos > 0 && isWordRune(m.in[pos-1])
	if cur && !prev {
		return k(pos)
	}
	return false
}

type wordEndNode struct{}

func (wordEndNode) match(m *machine, pos int, k func(int) bool) bool {
	prev := pos > 0 && isWordRune(m.in[pos-1])
	cur := pos < len(m.in) && isWordRune(m.in[pos])
	if prev && !cur {
		return k(pos)
	}
	return false
}
