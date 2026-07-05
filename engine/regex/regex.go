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
//
// Design notes (what "bug-for-bug with nvi" does and does not mean here):
//
//   - Backtracking, not RE2. Like Spencer's engine it is a recursive
//     backtracker (continuation-passing), which is what backreferences require
//     and can be exponential on pathological patterns -- the same trade nvi
//     makes. For human-authored single-line search this is the right choice.
//   - One vi-/search-layer construct is folded into this package rather than
//     living above Spencer's regcomp: \< \> word boundaries (Spencer's C only
//     spells these [[:<:]] / [[:>:]], which are also accepted; nvi's re_conv
//     rewrites \< \> into them). Everything else is Spencer BRE: no
//     alternation (\| is a literal '|'), and an escaped ordinary character is
//     that literal character.
//   - Runes, not bytes. Historic nvi is byte/locale oriented; operating on
//     []rune is a deliberate modernization (correct UTF-8 handling) rather than
//     an attempt at ASCII byte-for-byte reproduction.
//   - An empty pattern is a valid compile here; "repeat the last RE" for an
//     empty / or ? is handled in the search/substitute layer, as in nvi.
package regex

import "fmt"

// Options controls compilation.
type Options struct {
	IgnoreCase bool // the 'ignorecase' option; folds case, including in [classes]
	// Magic is the 'magic' option (vi default true). When false it is nvi
	// nomagic -- '.', '*' and '[' are literal unless backslash-escaped (\. \* \[
	// take on their special meaning) -- NOT Spencer's REG_NOSPEC ("all literal").
	Magic bool
	// Alt enables \| alternation. User patterns never get this: POSIX BRE has
	// no alternation and Spencer/nvi match \| as a literal '|'. It exists for
	// the internally generated cscope patterns, whose blank-run expression
	// needs alternation -- nvi compiles those with REG_EXTENDED for the same
	// reason (re_compile SEARCH_CSCOPE).
	Alt bool
	// Interrupt, when set, is polled during matching (every pollEvery
	// quantifier iterations); returning true aborts the attempt, which then
	// reports no match. It exists because the backtracker is exponential on
	// pathological nested quantifiers (\(a*\)*b) -- without it such a match
	// would hang the editor beyond even ^C (qa/CORNERS.md Part C #12). The
	// engine passes its ^C flag; nil means never interrupted.
	Interrupt func() bool
}

// Regex is a compiled pattern.
type Regex struct {
	root    node
	ngroups int
	ic      bool
	intr    func() bool
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
	p := &parser{src: []rune(pattern), magic: opts.Magic, alt: opts.Alt}
	root, err := p.parseAlternation(true)
	if err != nil {
		return nil, err
	}
	if !p.eof() {
		// The only way the parser stops early is an unmatched \) (Spencer
		// REG_EPAREN).
		return nil, fmt.Errorf("parentheses not balanced")
	}
	return &Regex{root: root, ngroups: p.ngroups, ic: opts.IgnoreCase, intr: opts.Interrupt}, nil
}

// MatchAt returns the leftmost-longest match beginning at or after start, or
// ok=false. POSIX semantics, like Spencer's engine: among matches at the
// leftmost matching position, the longest wins. The backtracker explores every
// way to match at that position (the continuation always declines) and keeps
// the captures from the first walk that reached the longest end; exploration
// stops early once a match runs to the end of the input, since nothing can be
// longer.
func (re *Regex) MatchAt(in []rune, start int) (mm Match, ok bool) {
	if start < 0 {
		start = 0
	}
	// An interrupt during the match unwinds as a matchInterrupted panic from
	// machine.poll; report it as no match -- the search/substitute layers see
	// the failure with the interrupt flag set and message "Interrupted".
	defer func() {
		if r := recover(); r != nil {
			if _, isIntr := r.(matchInterrupted); !isIntr {
				panic(r)
			}
			mm, ok = Match{}, false
		}
	}()
	m := &machine{in: in, ic: re.ic, caps: make([]int, 2*(re.ngroups+1)), intr: re.intr}
	bestCaps := make([]int, len(m.caps))
	for s := start; s <= len(in); s++ {
		for i := range m.caps {
			m.caps[i] = -1
		}
		m.caps[0] = s
		best := -1
		re.root.match(m, s, func(p int) bool {
			if p > best {
				best = p
				copy(bestCaps, m.caps)
			}
			return p == len(in) // longest possible: stop exploring
		})
		if best >= 0 {
			groups := make([][2]int, re.ngroups+1)
			groups[0] = [2]int{s, best}
			for g := 1; g <= re.ngroups; g++ {
				groups[g] = [2]int{bestCaps[2*g], bestCaps[2*g+1]}
			}
			return Match{Start: s, End: best, Groups: groups}, true
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
		if re.intr != nil && re.intr() {
			break // interrupted: stop rescanning earlier positions
		}
		if mm, ok := re.MatchAt(in, s); ok && mm.Start == s {
			return mm, true
		}
	}
	return Match{}, false
}

// NumGroups returns the number of capturing groups.
func (re *Regex) NumGroups() int { return re.ngroups }
