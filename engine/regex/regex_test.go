package regex

import (
	"testing"
	"time"
)

func mustCompile(t *testing.T, pat string, ic bool) *Regex {
	t.Helper()
	re, err := Compile(pat, Options{Magic: true, IgnoreCase: ic})
	if err != nil {
		t.Fatalf("compile %q: %v", pat, err)
	}
	return re
}

// Compile errors and their texts follow Spencer's regcomp/regerror (what nvi
// bundles and displays after an "RE error: " prefix).
func TestCompileErrors(t *testing.T) {
	cases := []struct{ pat, want string }{
		{`a**`, "repetition-operator operand invalid"},
		{`a*\{2\}`, "repetition-operator operand invalid"},
		{`a\{2\}\{2\}`, "repetition-operator operand invalid"},
		{`\{2\}`, "repetition-operator operand invalid"},
		{`\}`, "parentheses not balanced"},
		{`a\)`, "parentheses not balanced"},
		{`\(a`, "parentheses not balanced"},
		{`a\`, `trailing backslash (\)`},
		{`a\{3,1\}`, "invalid repetition count(s)"},
		{`a\{256\}`, "invalid repetition count(s)"},
		{`a\{2x\}`, "invalid repetition count(s)"},
		{`a\{2`, "braces not balanced"},
		// Spencer repeat REP(0,0) drops the operand; a pattern left empty is
		// REG_EMPTY. \{0,\} / \{0,n>0\} stay valid.
		{`a\{0\}`, "empty (sub)expression"},
		{`a\{0,0\}`, "empty (sub)expression"},
		{`\(a\{0\}\)`, "empty (sub)expression"},
		{`a\{0\}b\{0\}`, "empty (sub)expression"},
		{`[z-a]`, "invalid character range"},
		{`[abc`, "brackets ([ ]) not balanced"},
		{`[[:bogus:]]`, "invalid character class"},
		{`\1`, "invalid backreference number"},
	}
	for _, tc := range cases {
		_, err := Compile(tc.pat, Options{Magic: true})
		if err == nil {
			t.Errorf("compile %q: no error, want %q", tc.pat, tc.want)
			continue
		}
		if err.Error() != tc.want {
			t.Errorf("compile %q: error %q, want %q", tc.pat, err.Error(), tc.want)
		}
	}
	// The same rules in nomagic: \* not in first position and not after an
	// atom is a bad repetition; a first \* is a literal star.
	if _, err := Compile(`a\*\*`, Options{Magic: false}); err == nil || err.Error() != "repetition-operator operand invalid" {
		t.Errorf(`nomagic a\*\*: err %v`, err)
	}
	re := mustCompileOpts(t, `\*x`, Options{Magic: false})
	if m, ok := re.MatchAt([]rune("z*x"), 0); !ok || m.Start != 1 || m.End != 3 {
		t.Errorf(`nomagic \*x: got %+v ok=%v, want 1-3`, m, ok)
	}
	// Still-valid forms.
	for _, ok := range []string{`*a`, `^*a`, `a\{2,\}`, `a\{0,255\}`, `a\{0,1\}`, `a\{0,\}`, `x\(\)y`, `[]a]`, `[a-]`} {
		if _, err := Compile(ok, Options{Magic: true}); err != nil {
			t.Errorf("compile %q: unexpected error %v", ok, err)
		}
	}
	// \{0\} drops the atom: a\{0\}b is just b (Spencer DROP).
	reDrop := mustCompileOpts(t, `a\{0\}b`, Options{Magic: true})
	if m, ok := reDrop.MatchAt([]rune("abc"), 0); !ok || m.Start != 1 || m.End != 2 {
		t.Errorf(`a\{0\}b on "abc": got %+v ok=%v, want 1-2`, m, ok)
	}
}

func mustCompileOpts(t *testing.T, pat string, o Options) *Regex {
	t.Helper()
	re, err := Compile(pat, o)
	if err != nil {
		t.Fatalf("compile %q: %v", pat, err)
	}
	return re
}

func TestMatchBasic(t *testing.T) {
	cases := []struct {
		pat, in    string
		start, end int // expected whole-match offsets, or -1 if no match
	}{
		{"abc", "xabcx", 1, 4},
		{"a.c", "xazcx", 1, 4},
		{"a*", "aaab", 0, 3},
		{"ab*c", "ac", 0, 2},
		{"ab*c", "abbbc", 0, 5},
		{"^abc", "abcdef", 0, 3},
		{"abc$", "xxabc", 2, 5},
		{"^$", "", 0, 0},
		{"[a-c]x", "zbx", 1, 3},
		{"[^0-9]", "12a3", 2, 3},
		{"[[:digit:]]*", "42abc", 0, 2},
		// Collating [[.c.]] and equivalence [[=c=]] elements: in the C locale
		// each is just its own character, usable as a range endpoint.
		{"[[=a=]]", "z a", 2, 3},
		{"[[.-.]]x", "a-x", 1, 3},
		{"[[.a.]-[.c.]]", "zb", 1, 2},
		// Named collating elements resolve through Spencer's cname.h table:
		// [[.tab.]] matches a tab, [[.comma.]] a ',', and a named endpoint
		// works in a range (nvi regcomp.c p_b_coll_elem, fixed 2026-07-06).
		{"[[.tab.]]", "a\tb", 1, 2},
		{"[[.comma.]]x", "a,x", 1, 3},
		{"[[.newline.]]", "a\nb", 1, 2},
		{"[[.space.]-[.tilde.]]", "\t!", 1, 2},
		{`\(ab\)\1`, "abab", 0, 4},
		{`a\{2,3\}`, "aaaa", 0, 3},
		{`a\{2\}`, "aaaa", 0, 2},
		{`\<word\>`, "a word here", 2, 6},
		// No BRE alternation: \| is an escaped ordinary '|' (Spencer/nvi).
		{`foo\|bar`, "a foo|bar b", 2, 9},
		{`foo\|bar`, "xbar", -1, -1},
		// POSIX leftmost-longest (Spencer): the greedy-prefix parse is not
		// enough when a later optional part could extend the match.
		{`a*\(ab\)\{0,1\}`, "aab", 0, 3},
		{`ab*\(bc\)\{0,1\}`, "abbc", 0, 4},
		{`x*\(xy\)\{0,1\}z*`, "xxy", 0, 3},
		// * after ^ is an ordinary character (Spencer p_simp_re starordinary):
		// the anchor is not a repeatable atom.
		{`^*a`, "*ab", 0, 2},
		{`^*a`, "x*a", -1, -1},
		// A leading * is ordinary too, at the top level and in a group.
		{`*a`, "z*a", 1, 3},
		{`\(*a\)`, "z*a", 1, 3},
		{`\.`, "a.b", 1, 2},
		{"xyz", "abc", -1, -1},
		// POSIX/nvi: an escaped ordinary character is that literal character.
		// \t is the letter t and \n the letter n, never tab/newline (vim).
		{`a\tb`, "xatbx", 1, 4},
		{`a\tb`, "a\tb", -1, -1},
		{`a\nb`, "xanbx", 1, 4},
	}
	for _, tc := range cases {
		re := mustCompile(t, tc.pat, false)
		m, ok := re.MatchAt([]rune(tc.in), 0)
		if tc.start < 0 {
			if ok {
				t.Errorf("%q on %q: expected no match, got %+v", tc.pat, tc.in, m)
			}
			continue
		}
		if !ok {
			t.Errorf("%q on %q: expected match, got none", tc.pat, tc.in)
			continue
		}
		if m.Start != tc.start || m.End != tc.end {
			t.Errorf("%q on %q: got [%d,%d), want [%d,%d)", tc.pat, tc.in, m.Start, m.End, tc.start, tc.end)
		}
	}
}

func TestIgnoreCase(t *testing.T) {
	re := mustCompile(t, "abc", true)
	if _, ok := re.MatchAt([]rune("xABCy"), 0); !ok {
		t.Fatal("ignorecase match failed")
	}
}

// TestEREMatch pins POSIX ERE syntax (nvi :set extended / REG_EXTENDED) against
// Spencer's p_ere rules: unescaped ( ) | + ? {m,n}, no backreferences (\1 is
// literal '1'), empty branches REG_EMPTY, {0} drops like BRE \{0\}.
func TestEREMatch(t *testing.T) {
	ere := Options{Magic: true, Extended: true}
	cases := []struct {
		pat, in    string
		start, end int
	}{
		{"ab|cd", "xcdy", 1, 3},
		{"ab|cd", "acbd", -1, -1},
		{"a+b", "aaab", 0, 4},
		{"a+b", "b", -1, -1},
		{"ab?c", "ac", 0, 2},
		{"ab?c", "abc", 0, 3},
		{"(ab)+c", "ababc", 0, 5},
		{"a(b|c)d", "acd", 0, 3},
		{"ab|abc", "xabcy", 1, 4}, // leftmost-longest
		{"a{2,3}", "aaaa", 0, 3},
		{"a{2}", "aaaa", 0, 2},
		{"(ab)", "xaby", 1, 3},
		{`foo\|bar`, "a foo|bar b", 2, 9}, // \| still literal |
		{`\<word\>`, "a word here", 2, 6}, // vi-layer word boundary
		{`(a)\1`, "a1", 0, 2},             // ERE: \1 is literal '1'
		{`(a)\1`, "aa", -1, -1},
		{"()", "x", 0, 0},
		{"a+", "xa+y", 1, 2}, // one-or-more a
	}
	for _, tc := range cases {
		re := mustCompileOpts(t, tc.pat, ere)
		m, ok := re.MatchAt([]rune(tc.in), 0)
		if tc.start < 0 {
			if ok {
				t.Errorf("ERE %q on %q: expected no match, got %+v", tc.pat, tc.in, m)
			}
			continue
		}
		if !ok {
			t.Errorf("ERE %q on %q: expected match, got none", tc.pat, tc.in)
			continue
		}
		if m.Start != tc.start || m.End != tc.end {
			t.Errorf("ERE %q on %q: got [%d,%d), want [%d,%d)", tc.pat, tc.in, m.Start, m.End, tc.start, tc.end)
		}
	}
}

func TestERECompileErrors(t *testing.T) {
	ere := Options{Magic: true, Extended: true}
	cases := []struct{ pat, want string }{
		{"*", "repetition-operator operand invalid"},
		{"+", "repetition-operator operand invalid"},
		{"?", "repetition-operator operand invalid"},
		{"a|", "empty (sub)expression"},
		{"|a", "empty (sub)expression"},
		{"a{0}", "empty (sub)expression"},
		{"a{0,0}", "empty (sub)expression"},
		{"(a{0})", "empty (sub)expression"},
		{"a)", "parentheses not balanced"},
		{"(a", "parentheses not balanced"},
		{"a{3,1}", "invalid repetition count(s)"},
		{"a{2", "braces not balanced"},
	}
	for _, tc := range cases {
		_, err := Compile(tc.pat, ere)
		if err == nil {
			t.Errorf("ERE compile %q: no error, want %q", tc.pat, tc.want)
			continue
		}
		if err.Error() != tc.want {
			t.Errorf("ERE compile %q: error %q, want %q", tc.pat, err.Error(), tc.want)
		}
	}
	// {0,} and {0,1} stay valid (not REP(0,0)).
	for _, ok := range []string{"a{0,}", "a{0,1}", "a{0,255}", "a+", "a?", "a|b", "(a|b)+"} {
		if _, err := Compile(ok, ere); err != nil {
			t.Errorf("ERE compile %q: unexpected error %v", ok, err)
		}
	}
	// Drop: a{0}b is just b.
	re := mustCompileOpts(t, "a{0}b", ere)
	if m, ok := re.MatchAt([]rune("abc"), 0); !ok || m.Start != 1 || m.End != 2 {
		t.Errorf("ERE a{0}b on abc: got %+v ok=%v, want 1-2", m, ok)
	}
}

// TestERENomagic checks nvi re_conv's magic flip still applies under extended:
// bare . * [ are literal; \. \* \[ regain special meaning.
func TestERENomagic(t *testing.T) {
	o := Options{Magic: false, Extended: true}
	// bare a.c is three literals
	re := mustCompileOpts(t, "a.c", o)
	if m, ok := re.MatchAt([]rune("axc"), 0); ok {
		t.Fatalf("nomagic ERE a.c should not match axc, got %+v", m)
	}
	if m, ok := re.MatchAt([]rune("a.c"), 0); !ok || m.Start != 0 || m.End != 3 {
		t.Fatalf("nomagic ERE a.c on a.c: got %+v ok=%v", m, ok)
	}
	// \. is any
	re = mustCompileOpts(t, `a\.c`, o)
	if m, ok := re.MatchAt([]rune("axc"), 0); !ok || m.Start != 0 || m.End != 3 {
		t.Fatalf(`nomagic ERE a\.c on axc: got %+v ok=%v`, m, ok)
	}
	// ERE | still special under nomagic
	re = mustCompileOpts(t, "ab|cd", o)
	if m, ok := re.MatchAt([]rune("xcdy"), 0); !ok || m.Start != 1 || m.End != 3 {
		t.Fatalf("nomagic ERE ab|cd: got %+v ok=%v", m, ok)
	}
}

func TestGroups(t *testing.T) {
	re := mustCompile(t, `\(aa*\)\(bb*\)`, false)
	m, ok := re.MatchAt([]rune("aabbb"), 0)
	if !ok {
		t.Fatal("no match")
	}
	if m.Groups[1] != [2]int{0, 2} {
		t.Errorf("group1 = %v, want [0 2]", m.Groups[1])
	}
	if m.Groups[2] != [2]int{2, 5} {
		t.Errorf("group2 = %v, want [2 5]", m.Groups[2])
	}
}

func TestMatchLast(t *testing.T) {
	re := mustCompile(t, "a", false)
	m, ok := re.MatchLast([]rune("abaca"), 5)
	if !ok || m.Start != 4 {
		t.Fatalf("MatchLast = %+v ok=%v, want start 4", m, ok)
	}
}

// TestWordBoundaryBracketKludge covers Spencer's [[:<:]] / [[:>:]] forms, which
// are word-boundary anchors (equivalent to \< / \>), not character classes.
func TestWordBoundaryBracketKludge(t *testing.T) {
	cases := []struct {
		pat, in    string
		start, end int
	}{
		{`[[:<:]]cat`, "scatter cat", 8, 11},         // skips "cat" inside "scatter"
		{`cat[[:>:]]`, "category cat", 9, 12},        // skips "cat" starting "category"
		{`[[:<:]]cat[[:>:]]`, "category cat", 9, 12}, // whole word only
	}
	for _, c := range cases {
		re := mustCompile(t, c.pat, false)
		in := []rune(c.in)
		got := -1
		for i := 0; i <= len(in); i++ {
			if m, ok := re.MatchAt(in, i); ok {
				got = m.Start
				if m.Start != c.start || m.End != c.end {
					t.Errorf("%q on %q: [%d,%d), want [%d,%d)", c.pat, c.in, m.Start, m.End, c.start, c.end)
				}
				break
			}
		}
		if got == -1 {
			t.Errorf("%q on %q: no match", c.pat, c.in)
		}
	}
}

// TestBackrefValidation pins Spencer/nvi's rule that a backreference is valid
// only to a group already closed before it; otherwise compilation errors.
func TestBackrefValidation(t *testing.T) {
	invalid := []string{`\1`, `\2`, `\9`, `\(a\)\2`, `\(a\1\)`}
	for _, p := range invalid {
		if _, err := Compile(p, Options{Magic: true}); err == nil {
			t.Errorf("Compile(%q): want error (backref to unclosed/missing group), got nil", p)
		}
	}
	valid := []string{`\(a\)\1`, `\(a\)\(b\)\2\1`, `\(a\(b\)\2\)`}
	for _, p := range valid {
		if _, err := Compile(p, Options{Magic: true}); err != nil {
			t.Errorf("Compile(%q): unexpected error %v", p, err)
		}
	}
}

// TestInterruptAbortsPathologicalMatch pins the ^C escape hatch: a match that
// blows up exponentially (nested quantifiers, qa/CORNERS.md Part C #12) must
// abort promptly once the interrupt hook fires, instead of hanging the
// editor. Without the hook this pattern/input would run for years (2^64).
func TestInterruptAbortsPathologicalMatch(t *testing.T) {
	calls := 0
	re, err := Compile(`\(a*\)*b`, Options{Magic: true, Interrupt: func() bool { calls++; return true }})
	if err != nil {
		t.Fatal(err)
	}
	in := make([]rune, 64)
	for i := range in {
		in[i] = 'a'
	}
	start := time.Now()
	if _, ok := re.MatchAt(in, 0); ok {
		t.Fatal("must not match")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("interrupt did not abort the match (took %v)", d)
	}
	if calls == 0 {
		t.Fatal("interrupt hook was never polled")
	}
	if _, ok := re.MatchLast(in, len(in)); ok {
		t.Fatal("MatchLast must not match either")
	}
}

// TestInterruptFalseLeavesMatchingIntact: with a hook installed that never
// fires, results are identical to matching without one.
func TestInterruptFalseLeavesMatchingIntact(t *testing.T) {
	re, err := Compile(`\(ab\)*c`, Options{Magic: true, Interrupt: func() bool { return false }})
	if err != nil {
		t.Fatal(err)
	}
	m, ok := re.MatchAt([]rune("xababc"), 0)
	if !ok || m.Start != 1 || m.End != 6 {
		t.Fatalf("match = %+v ok=%v, want [1,6)", m, ok)
	}
	if g := m.Groups[1]; g != [2]int{3, 5} {
		t.Fatalf("group 1 = %v, want [3,5) (last iteration)", g)
	}
}
