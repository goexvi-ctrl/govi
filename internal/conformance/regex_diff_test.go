package conformance

import "testing"

// regexCases is a broad battery of substitute/global commands that exercise the
// BRE features (and nvi extensions) the regex engine must reproduce. Each is run
// against both the nvi oracle (Henry Spencer regex) and govi; the resulting file
// must match exactly.
var regexCases = []struct {
	name  string
	input string
	cmds  []string
}{
	// literals and the dot metacharacter
	{"dot", "abc\nadc\na.c\n", []string{`%s/a.c/X/`}},
	{"dot-escaped", "abc\na.c\n", []string{`%s/a\.c/X/`}},
	{"star", "aaa\nbaab\n", []string{`%s/a*/X/g`}},
	{"star-literal-at-start", "*a*b\n", []string{`%s/*/X/g`}},
	{"dot-star", "axxxb\nab\n", []string{`%s/a.*b/X/`}},
	{"star-greedy", "<a><b>\n", []string{`%s/<.*>/X/`}},

	// anchors
	{"caret", "abcabc\n", []string{`%s/^abc/X/`}},
	{"dollar", "abcabc\n", []string{`%s/abc$/X/`}},
	{"caret-dollar-empty", "\nx\n\n", []string{`%s/^$/EMPTY/`}},
	{"caret-mid-literal", "a^b\n", []string{`%s/a^b/X/`}},
	{"dollar-mid-literal", "a$b\n", []string{`%s/a$b/X/`}},
	{"anchor-insert-bol", "one\ntwo\n", []string{`%s/^/> /`}},
	{"anchor-insert-eol", "one\ntwo\n", []string{`%s/$/;/`}},

	// character classes
	{"class-simple", "a1b2c3\n", []string{`%s/[0-9]//g`}},
	{"class-range", "Hello World\n", []string{`%s/[a-z]/./g`}},
	{"class-negate", "a1b2!c\n", []string{`%s/[^0-9]//g`}},
	{"class-rbracket-first", "a]b[c\n", []string{`%s/[]]/X/g`}},
	{"class-caret-not-first", "a^b\n", []string{`%s/[a^]/X/g`}},
	{"class-dash-last", "a-b+c\n", []string{`%s/[+-]/X/g`}},
	{"class-dash-first", "a-b\n", []string{`%s/[-a]/X/g`}},
	{"class-metachar-literal", "a.b*c\n", []string{`%s/[.*]/X/g`}},
	// NOTE: nvi's build has broken POSIX-class support ([[:alpha:]], [[:digit:]],
	// [[:upper:]] all match the same alnum-ish set, ignoring the class name).
	// govi implements them per spec, a deliberate divergence; not tested here.

	// BRE metacharacters that are LITERAL in nvi (no ERE without :set extended),
	// and the escaped forms nvi does not treat as operators -- govi must agree.
	{"pipe-literal", "a|b\n", []string{`%s/a|b/X/`}},
	{"plus-literal", "a+b\n", []string{`%s/a+b/X/`}},
	{"question-literal", "a?b\n", []string{`%s/a?b/X/`}},
	{"paren-literal", "(x)y\n", []string{`%s/(x)/Z/`}},
	{"brace-literal", "a{b}\n", []string{`%s/a{b}/X/`}},
	{"backslash-plus-literal", "a+b\n", []string{`%s/a\+/X/`}},
	{"backslash-question-literal", "colou?r\n", []string{`%s/colou\?r/X/`}},
	{"backslash-w-literal", "a w b\n", []string{`%s/\w/X/g`}},
	{"class-backslash-plus-literal", "12+3\n", []string{`%s/[0-9]\+/N/`}},

	// grouping, backreferences, repetition
	{"group-backref-swap", "John Smith\n", []string{`%s/\([A-Za-z]*\) \([A-Za-z]*\)/\2 \1/`}},
	{"backref-in-pattern", "hello hello world\n", []string{`%s/\([a-z]*\) \1/DUP/`}},
	{"backref-valid", "abcabc x\n", []string{`%s/\(abc\)\1/Y/`}},
	{"backref-nested", "abab\n", []string{`%s/\(\(ab\)\2\)/Z/`}},
	{"interval-exact", "aaaaa\n", []string{`%s/a\{3\}/X/`}},
	{"interval-min", "aaaaa\n", []string{`%s/a\{2,\}/X/`}},
	{"interval-range", "aaaaa\n", []string{`%s/a\{2,3\}/X/`}},
	// word boundaries (nvi BRE supports \< and \>)
	{"word-start", "the theme\n", []string{`%s/\<the/X/g`}},
	{"word-end", "the theme bathe\n", []string{`%s/the\>/X/g`}},
	{"word-both", "a cat category\n", []string{`%s/\<cat\>/X/g`}},
	// Spencer's [[:<:]] / [[:>:]] word-boundary kludge (not POSIX classes).
	{"bracket-word-start", "cat scatter cat\n", []string{`%s/[[:<:]]cat/X/g`}},
	{"bracket-word-end", "cat scatter cat\n", []string{`%s/cat[[:>:]]/X/g`}},
	{"bracket-word-both", "cat category cat\n", []string{`%s/[[:<:]]cat[[:>:]]/X/g`}},
	// named collating elements [[.name.]] resolve through Spencer's cname.h
	// (nvi regcomp.c p_b_coll_elem, fixed 2026-07-06): [[.tab.]] is a tab,
	// [[.comma.]] a ',', and a named endpoint anchors a range.
	{"coll-tab", "a\tb\tc\n", []string{`%s/[[.tab.]]/X/g`}},
	{"coll-comma", "a,b,c\n", []string{`%s/[[.comma.]]/X/g`}},
	{"coll-period", "a.b.c\n", []string{`%s/[[.period.]]/X/g`}},
	{"coll-single-char", "a-b\n", []string{`%s/[[.-.]]/X/`}},
	{"coll-range", "abcXYZ\n", []string{`%s/[[.a.]-[.c.]]/_/g`}},
	{"coll-class-tab", "a\tb c\n", []string{`%s/[[.tab.] ]/X/g`}},

	// empty matches and global behavior
	{"empty-global", "abc\n", []string{`%s/x*/-/g`}},
	{"empty-between", "abc\n", []string{`%s/b*/-/g`}},
	{"star-empty-anchored", "aaa\n", []string{`%s/a*$/X/`}},

	// replacement metacharacters
	{"repl-amp", "cat\n", []string{`%s/cat/[&]/`}},
	{"repl-amp-escaped", "cat\n", []string{`%s/cat/\&/`}},
	{"repl-upper-one", "john\n", []string{`%s/\(j\)/\u\1/`}},
	{"repl-upper-all", "hello world\n", []string{`%s/.*/\U&/`}},
	{"repl-lower-all", "HELLO\n", []string{`%s/.*/\L&/`}},
	{"repl-upper-until-E", "hello world\n", []string{`%s/\([a-z]*\) \([a-z]*\)/\U\1\E \2/`}},
	{"repl-newline", "a,b,c\n", []string{`%s/,/\r/g`}},

	// case folding
	{"ignorecase", "Foo foo FOO\n", []string{`set ic`, `%s/foo/x/g`}},
	{"noignorecase", "Foo foo FOO\n", []string{`%s/foo/x/g`}},

	// combinations / trickier
	{"alternation-ish-class", "cat bat rat\n", []string{`%s/[cbr]at/X/g`}},
	{"nested-group-repeat", "abab abab\n", []string{`%s/\(ab\)\{2\}/X/g`}},
	{"backref-repeat", "abcabc xy\n", []string{`%s/\(abc\)\1/Y/`}},
	{"leading-star-after-group", "a)b\n", []string{`%s/)/]/`}},
	{"whole-line", "anything here\n", []string{`%s/.*/REPLACED/`}},
	{"multi-substitute", "aaa\nbbb\nccc\n", []string{`%s/a/1/g`, `%s/b/2/g`, `%s/c/3/g`}},

	// ERE (:set extended) -- nvi REG_EXTENDED / Spencer p_ere via the editor surface.
	{"ere-alt", "xcdy\nacbd\n", []string{`set extended`, `%s/ab|cd/X/g`}},
	{"ere-plus", "aaab\nb\naab\n", []string{`set extended`, `%s/a+b/X/g`}},
	{"ere-quest", "ac\nabc\nabbc\n", []string{`set extended`, `%s/ab?c/X/g`}},
	{"ere-group-alt", "abd\nacd\nad\n", []string{`set extended`, `%s/a(b|c)d/X/g`}},
	{"ere-group-plus", "ababc\n", []string{`set extended`, `%s/(ab)+c/X/`}},
	{"ere-brace", "aaaa\n", []string{`set extended`, `%s/a{2,3}/X/g`}},
	{"ere-brace-exact", "aaaa\n", []string{`set extended`, `%s/a{2}/X/g`}},
	{"ere-leftmost-longest", "xabcy\n", []string{`set extended`, `%s/ab|abc/X/`}},
	// Spencer ERE has no backrefs: \1 is literal '1' (matches a1, not aa).
	{"ere-no-backref", "aa\na1\n", []string{`set extended`, `%s/(a)\1/X/g`}},
	{"ere-bre-plus-literal", "xa+y\n", []string{`%s/a+/X/`}},    // BRE: + literal
	{"ere-off-paren-literal", "(ab)\n", []string{`%s/(ab)/X/`}}, // BRE: parens literal
	{"ere-word-boundary", "cat scatter cat\n", []string{`set extended`, `%s/\<cat\>/X/g`}},
	{"ere-nomagic-dot", "axc\na.c\n", []string{`set extended`, `set nomagic`, `%s/a.c/X/g`}},
	{"ere-drop-caret", "abc\n", []string{`set extended`, `%s/.{0}^/X/g`}},
}

func TestRegexConformance(t *testing.T) {
	oracle := FindOracle()
	if oracle == "" {
		t.Skip("no nvi oracle found")
	}
	govi, err := GoviBinary()
	if err != nil {
		t.Fatalf("GoviBinary: %v", err)
	}
	// Binary-to-binary so RE errors / "No match found" (non-zero exit) still
	// compare buffer content, not just successful compiles.
	for _, tc := range regexCases {
		t.Run(tc.name, func(t *testing.T) {
			sess := ExSession{Input: tc.input, Commands: tc.cmds}
			want := RunBatchBinaryFull(oracle, sess, 0)
			got := RunBatchBinaryFull(govi, sess, 0)
			if isInfraErr(want.ExitErr) {
				t.Fatalf("oracle infra: %v", want.ExitErr)
			}
			if isInfraErr(got.ExitErr) {
				t.Fatalf("govi infra: %v", got.ExitErr)
			}
			wantOK := want.ExitErr == nil
			gotOK := got.ExitErr == nil
			if wantOK != gotOK {
				t.Errorf("exit nvi_ok=%v govi_ok=%v nvi_err=%v govi_err=%v",
					wantOK, gotOK, want.ExitErr, got.ExitErr)
			}
			if normalizeBuf(got.Content) != normalizeBuf(want.Content) {
				t.Errorf("cmds %v on %q\n govi %q\n nvi  %q",
					tc.cmds, tc.input, normalizeBuf(got.Content), normalizeBuf(want.Content))
			}
		})
	}
}
