// Package regex implements a vi-compatible regular expression engine. vi uses
// POSIX basic regular expressions (BRE) by default, with an optional extended
// (ERE) mode gated on nvi's :set extended. Neither mode is Go's RE2-based
// regexp package (RE2 forbids backreferences). This is a backtracking matcher
// over runes, mirroring nvi's bundled Spencer regex (regex/) plus the vi
// search layer (common/search.c, ex/ex_subst.c re_conv).
//
// The engine works on a single line of runes at a time, matching vi's
// line-oriented search and substitute semantics.
//
// Design notes:
//
//   - One matcher, two parse modes. Options.Extended selects ERE syntax
//     (( ) | + ? {m,n} unescaped) vs BRE (\( \) \{m,n\}, no alternation).
//     Both share the same AST nodes and backtracker.
//   - Backtracking, not RE2. Like Spencer it is a recursive continuation-
//     passing matcher -- required for BRE backreferences, exponential on
//     pathological nested quantifiers. The Interrupt hook lets ^C abort a
//     blown-up match.
//   - Vi-layer constructs folded in: \< \> word boundaries (nvi re_conv
//     rewrites these to [[:<:]] / [[:>:]] before regcomp; we accept both
//     spellings) and magic/nomagic (nvi re_conv flips .*[ ; we do it in the
//     parser via Options.Magic).
//   - Spencer ERE has no backreferences: in Extended mode \1 is a literal
//     '1', matching nvi. BRE \(...\)\1 still works when Extended is false.
//   - Runes, not bytes: deliberate UTF-8 modernization, not ASCII byte
//     reproduction.
//   - Empty pattern compile is allowed; "repeat the last RE" is handled above
//     this package in the search/substitute layer.
package regex

import "fmt"

// Options controls compilation.
type Options struct {
	IgnoreCase bool // the 'ignorecase' option; folds case, including in [classes]
	// Magic is the 'magic' option (vi default true). When false it is nvi
	// nomagic -- '.', '*' and '[' are literal unless backslash-escaped (\. \* \[
	// take on their special meaning) -- NOT Spencer's REG_NOSPEC ("all literal").
	// Applies under both BRE and ERE (nvi re_conv runs before regcomp either way).
	Magic bool
	// Extended selects POSIX ERE syntax (nvi :set extended / REG_EXTENDED).
	// Default false is BRE, matching nvi's default noextended.
	Extended bool
	// Alt enables BRE-style \| alternation for internally generated cscope
	// patterns only. User BREs never set this ( \| is a literal '|' ). When
	// Extended is true, alternation is the unescaped ERE | and Alt is ignored.
	Alt bool
	// Interrupt, when set, is polled during matching (every pollEvery
	// quantifier iterations); returning true aborts the attempt, which then
	// reports no match. The engine passes its ^C flag; nil means never
	// interrupted.
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
	p := &parser{
		src:      []rune(pattern),
		magic:    opts.Magic,
		extended: opts.Extended,
		alt:      opts.Alt && !opts.Extended, // ERE uses unescaped |; ignore Alt
	}
	root, err := p.parseAlternation(true)
	if err != nil {
		return nil, err
	}
	if !p.eof() {
		// Unmatched group close left unconsumed, or other early stop.
		return nil, fmt.Errorf("parentheses not balanced")
	}
	// Spencer: REQUIRE nonempty (REG_EMPTY) after {0}-drops leave nothing.
	if _, ok := root.(omitNode); ok {
		return nil, fmt.Errorf("empty (sub)expression")
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
