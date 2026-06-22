package conformance

import "testing"

// viConformanceCases are vi-mode keystroke scripts whose observable result
// (final buffer contents) must match the C nvi oracle exactly.
var viConformanceCases = []struct {
	name  string
	input string
	keys  string
}{
	{"delete-char", "hello world\n", "x"},
	{"delete-count", "hello world\n", "3x"},
	{"delete-word", "hello world\n", "dw"},
	{"delete-word-eol", "foo\nbar\n", "dw"},
	{"delete-to-eol", "hello world\n", "wd$"},
	{"delete-line", "a\nb\nc\n", "dd"},
	{"delete-2-lines", "a\nb\nc\nd\n", "2dd"},
	{"delete-down", "a\nb\nc\n", "dj"},
	{"change-word", "hello world\n", "cwbye\x1b"},
	{"change-to-eol", "hello world\n", "wCthere\x1b"},
	{"change-line", "old\nkeep\n", "ccnew\x1b"},
	{"insert", "bc\n", "iA\x1b"},
	{"append", "ac\n", "ab\x1b"},
	{"append-eol", "ab\n", "Ac\x1b"},
	{"open-below", "a\nc\n", "ob\x1b"},
	{"open-above", "b\nc\n", "Oa\x1b"},
	{"yank-put", "one\ntwo\n", "yyp"},
	{"delete-put-swap", "ab\n", "xp"},
	{"join", "foo\nbar\n", "J"},
	{"replace-char", "abc\n", "rX"},
	{"replace-count", "abcd\n", "3rX"},
	{"replace-mode", "abcdef\n", "RXYZ\x1b"},
	{"tilde", "abc\n", "~"},
	{"find-delete", "a,b,c\n", "df,"},
	{"till-delete", "a,b,c\n", "dt,"},
	{"goto-line", "a\nb\nc\nd\n", "3Gdd"},
	{"dot-repeat-x", "hello\n", "x.."},
	{"dot-repeat-dw", "a b c d\n", "dw."},
	{"undo", "hello\n", "xxu"},
	{"undo-toggle", "hello\n", "xxuu"},
	{"undo-toggle-3", "hello\n", "xxuuu"},
	{"undo-dot-walk", "hello\n", "xxxu.."},
	{"undo-dot-count", "hello\n", "xxxxu2."},
	{"undo-redo-direction", "abcd\n", "xxxuuu..u.."},
	{"undo-redo-partial", "abcde\n", "xxxxu..u."},
	{"insert-newline", "helloworld\n", "5lifoo\rbar\x1b"},
	{"search-delete", "alpha\nbeta\ngamma\n", "/gamma\rdd"},
	{"search-next-delete", "x\nfoo\ny\nfoo\n", "/foo\rnD"},
	{"search-back-change", "foo\nbar\nfoo\n", "G?foo\rcwBAZ\x1b"},
	{"search-regex-delete", "abc123\n", "/[0-9]\rD"},
	{"map-command", "a\nb\nc\nd\n", ":map X dd\rXX"},
	{"autoindent-open", "    foo\n", ":set ai\robar\x1b"},
	{"abbrev-insert", "\n", ":ab teh the\riteh \x1b"},
	{"append-last-char", "abc\n", "$aX\x1b"},
	{"x-at-end", "abc\n", "$x"},
	{"r-at-end", "abc\n", "$rZ"},
	{"change-last-char", "abc\n", "$cXdef\x1b"},
}

// TestViConformance pins govi's vi-mode behavior to nvi. It needs both an nvi
// oracle and a working PTY; it skips otherwise.
func TestViConformance(t *testing.T) {
	oracle := FindOracle()
	if oracle == "" {
		t.Skip("no nvi oracle found")
	}

	for _, tc := range viConformanceCases {
		t.Run(tc.name, func(t *testing.T) {
			sess := ViSession{Input: tc.input, Keys: tc.keys}

			want, err := RunOracleVi(oracle, sess)
			if err != nil {
				t.Skipf("oracle run failed (PTY unavailable?): %v", err)
			}
			got, err := RunGoviVi(sess)
			if err != nil {
				t.Fatalf("govi run: %v", err)
			}
			if normalizeBuf(got) != normalizeBuf(want) {
				t.Errorf("keys %q on %q:\n govi %q\n nvi  %q",
					tc.keys, tc.input, normalizeBuf(got), normalizeBuf(want))
			}
		})
	}
}
