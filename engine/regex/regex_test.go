package regex

import "testing"

func mustCompile(t *testing.T, pat string, ic bool) *Regex {
	t.Helper()
	re, err := Compile(pat, Options{Magic: true, IgnoreCase: ic})
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
		{`\(ab\)\1`, "abab", 0, 4},
		{`a\{2,3\}`, "aaaa", 0, 3},
		{`a\{2\}`, "aaaa", 0, 2},
		{`\<word\>`, "a word here", 2, 6},
		{`foo\|bar`, "xbar", 1, 4},
		{`\.`, "a.b", 1, 2},
		{"xyz", "abc", -1, -1},
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
		{`[[:<:]]cat`, "scatter cat", 8, 11},        // skips "cat" inside "scatter"
		{`cat[[:>:]]`, "category cat", 9, 12},       // skips "cat" starting "category"
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
