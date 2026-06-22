// Package regex implements a vi-compatible regular expression engine. vi uses
// POSIX basic regular expressions (BRE) with extensions -- backreferences,
// \< and \> word boundaries, \{m,n\} intervals, and the magic/nomagic option --
// none of which Go's RE2-based regexp package supports (it forbids
// backreferences). This is a backtracking matcher over runes, mirroring the
// behavior of nvi's bundled regex (regex/) and search integration
// (common/search.c).
//
// The engine works on a single line of runes at a time, matching vi's
// line-oriented search and substitute semantics.
package regex

import "fmt"

// Options controls compilation.
type Options struct {
	IgnoreCase bool // the 'ignorecase' option
	Magic      bool // the 'magic' option (vi default true)
}

// Regex is a compiled pattern.
type Regex struct {
	root    node
	ngroups int
	ic      bool
}

// Match is the result of a successful match: rune offsets into the input.
// Groups[0] is the whole match; Groups[i] is the i-th \(...\) capture, with
// {-1,-1} for a group that did not participate.
type Match struct {
	Start, End int
	Groups     [][2]int
}

// Compile parses pattern under the given options.
func Compile(pattern string, opts Options) (*Regex, error) {
	p := &parser{src: []rune(pattern), magic: opts.Magic}
	root, err := p.parseAlternation(true)
	if err != nil {
		return nil, err
	}
	if !p.eof() {
		return nil, fmt.Errorf("regex: trailing characters in pattern")
	}
	return &Regex{root: root, ngroups: p.ngroups, ic: opts.IgnoreCase}, nil
}

// MatchAt returns the leftmost match beginning at or after start, or ok=false.
func (re *Regex) MatchAt(in []rune, start int) (Match, bool) {
	if start < 0 {
		start = 0
	}
	m := &machine{in: in, ic: re.ic, caps: make([]int, 2*(re.ngroups+1))}
	for s := start; s <= len(in); s++ {
		for i := range m.caps {
			m.caps[i] = -1
		}
		m.caps[0] = s
		end := -1
		if re.root.match(m, s, func(p int) bool { end = p; return true }) {
			groups := make([][2]int, re.ngroups+1)
			groups[0] = [2]int{s, end}
			for g := 1; g <= re.ngroups; g++ {
				groups[g] = [2]int{m.caps[2*g], m.caps[2*g+1]}
			}
			return Match{Start: s, End: end, Groups: groups}, true
		}
	}
	return Match{}, false
}

// MatchLast returns the rightmost match beginning at or before start (used for
// backward search). It scans starts downward.
func (re *Regex) MatchLast(in []rune, start int) (Match, bool) {
	if start > len(in) {
		start = len(in)
	}
	for s := start; s >= 0; s-- {
		if mm, ok := re.MatchAt(in, s); ok && mm.Start == s {
			return mm, true
		}
	}
	return Match{}, false
}

// NumGroups returns the number of capturing groups.
func (re *Regex) NumGroups() int { return re.ngroups }
