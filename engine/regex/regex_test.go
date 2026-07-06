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
	for _, ok := range []string{`*a`, `^*a`, `a\{2,\}`, `a\{0,255\}`, `x\(\)y`, `[]a]`, `[a-]`} {
		if _, err := Compile(ok, Options{Magic: true}); err != nil {
			t.Errorf("compile %q: unexpected error %v", ok, err)
		}
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
