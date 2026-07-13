package engine

import (
	"os"
	"path/filepath"
	"testing"
)

// exCase runs an ex command line (or several, separated by '\n') against initial
// content and checks the buffer result.
func exCase(t *testing.T, name, initial string, cmds []string, want string) {
	t.Helper()
	e, _, _ := newTestEngine(t, initial)
	for _, c := range cmds {
		if err := e.exExecute(c); err != nil {
			t.Fatalf("%s: %q: %v", name, c, err)
		}
	}
	if got := bufText(e); got != want {
		t.Errorf("%s: %v on %q\n got %q\nwant %q", name, cmds, initial, got, want)
	}
}

func TestExDelete(t *testing.T) {
	exCase(t, "delete-range", "a\nb\nc\nd\n", []string{"2,3d"}, "a\nd")
	exCase(t, "delete-current", "a\nb\nc\n", []string{"2", "d"}, "a\nc")
	exCase(t, "delete-count", "a\nb\nc\nd\n", []string{"2d 2"}, "a\nd")
	exCase(t, "delete-all", "a\nb\nc\n", []string{"%d"}, "")
	exCase(t, "delete-dollar", "a\nb\nc\n", []string{"$d"}, "a\nb")
}

// TestExBackwardRange covers CORNERS B-1: a reversed two-address range is a
// specific parse error and makes no change (nvi ex.c). Verified against nvi.
func TestExBackwardRange(t *testing.T) {
	const want = "The second address is smaller than the first"
	for _, cmd := range []string{"4,2d", "4,2s/./X/", "3,1j", "5;3d", "2,1"} {
		e, _, _ := newTestEngine(t, "a\nb\nc\nd\ne\n")
		err := e.exExecute(cmd)
		if err == nil || err.Error() != want {
			t.Errorf("%q: err = %v, want %q", cmd, err, want)
		}
		if got := bufText(e); got != "a\nb\nc\nd\ne" {
			t.Errorf("%q changed the buffer: %q", cmd, got)
		}
	}
}

// TestExPipeSeparator covers CORNERS B-3: '|' separates ex commands, except
// inside a substitute RE (literal), and for the whole-line commands (!, global,
// v). '\|' is a literal pipe. Verified against nvi.
func TestExPipeSeparator(t *testing.T) {
	cases := []struct{ name, content, cmd, want string }{
		{"delete-then-subst", "aaa\nbbb\nccc\n", "1d|2s/c/Z/", "bbb\nZcc"},
		{"subst-then-subst", "aaa\nbbb\n", "1s/a/X/|s/X/Y/", "Yaa\nbbb"},
		{"pipe-literal-in-pattern", "axb\n", `s/a|b/Z/`, "axb"},
		{"pipe-literal-in-repl", "aaa\n", `s/a/x|y/`, "x|yaa"},
		{"escaped-pipe", "a\n", `s/a/b\|c/`, "b|c"},
		{"set-then-set", "x\n", "set noai|set sw=9", "x"},
		{"global-body-pipe", "x1\ny\nx2\n", "g/x/s/x/Q/|s/Q/W/", "W1\ny\nW2"},
	}
	for _, c := range cases {
		e, _, _ := newTestEngine(t, c.content)
		e.exExecute(c.cmd) // a no-match :s reports an error but must not change the buffer
		if got := bufText(e); got != c.want {
			t.Errorf("%s: %q -> %q, want %q", c.name, c.cmd, got, c.want)
		}
	}
	// set-then-set really applied both options.
	e, _, _ := newTestEngine(t, "x\n")
	e.exExecute("set noautoindent|set shiftwidth=9")
	if e.scr.opts.Bool("autoindent") || e.scr.opts.Int("shiftwidth") != 9 {
		t.Errorf("compound :set did not apply both: ai=%v sw=%d",
			e.scr.opts.Bool("autoindent"), e.scr.opts.Int("shiftwidth"))
	}
}

// TestMessageWordingParity covers CORNERS B-10: a sample of common status
// messages aligned to nvi's exact wording (verified against nvi 1.81.6).
func TestMessageWordingParity(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := New(&captureFrontend{}, Options{})
	if err := e.OpenArgs([]string{p}); err != nil {
		t.Fatal(err)
	}
	e.Resize(10, 60)

	// Write to a new name: "<name>: new file: N lines, M characters".
	if err := e.exExecute("w " + filepath.Join(dir, "g.txt")); err != nil {
		t.Fatal(err)
	}
	if want := "g.txt: new file: 3 lines, 17 characters"; e.scr.msg != want {
		t.Errorf("write-new msg = %q, want %q", e.scr.msg, want)
	}
	// Write the current (now-existing) file: no "new file:" prefix, "characters".
	if err := e.exExecute("w"); err != nil {
		t.Fatal(err)
	}
	if want := "f.txt: 3 lines, 17 characters"; e.scr.msg != want {
		t.Errorf("write-existing msg = %q, want %q", e.scr.msg, want)
	}
	// Failed search reports just "Pattern not found" (no echoed pattern).
	if err := e.startSearch("zzzz", searchFwd); err == nil || err.Error() != "Pattern not found" {
		t.Errorf("search miss = %v, want \"Pattern not found\"", err)
	}
	// Unknown option carries nvi's "'set all'" hint.
	err := e.exExecute("set nosuchopt")
	if want := "set: no suchopt option: 'set all' gives all option values"; err == nil || err.Error() != want {
		t.Errorf("bad option = %v, want %q", err, want)
	}
}

func TestExMove(t *testing.T) {
	exCase(t, "move-end", "1\n2\n3\n", []string{"1m$"}, "2\n3\n1")
	exCase(t, "move-top", "1\n2\n3\n", []string{"3m0"}, "3\n1\n2")
	exCase(t, "move-range", "a\nb\nc\nd\n", []string{"1,2m4"}, "c\nd\na\nb")
}

func TestExCopy(t *testing.T) {
	exCase(t, "copy-end", "1\n2\n3\n", []string{"1t$"}, "1\n2\n3\n1")
	exCase(t, "copy-co", "1\n2\n", []string{"1co0"}, "1\n1\n2")
	exCase(t, "copy-range", "a\nb\nc\n", []string{"1,2t$"}, "a\nb\nc\na\nb")
}

func TestExJoin(t *testing.T) {
	exCase(t, "join-range", "a\nb\nc\nd\n", []string{"1,3j"}, "a b c\nd")
	exCase(t, "join-current", "a\nb\nc\n", []string{"1j"}, "a b\nc")
}

func TestExYankPut(t *testing.T) {
	exCase(t, "yank-put", "a\nb\nc\n", []string{"1y", "$pu"}, "a\nb\nc\na")
	exCase(t, "move-via-named", "x\ny\n", []string{`1d a`, `$pu a`}, "y\nx")
}

func TestExShift(t *testing.T) {
	exCase(t, "shift-right", "a\nb\n", []string{"1>"}, "\ta\nb")
	exCase(t, "shift-left", "\ta\nb\n", []string{"1<"}, "a\nb")
}

func TestExAddressOnlyGoto(t *testing.T) {
	e, _, _ := newTestEngine(t, "a\nb\nc\nd\n")
	if err := e.exExecute("3"); err != nil {
		t.Fatal(err)
	}
	if e.scr.cursor.Line != 3 {
		t.Fatalf("goto: line %d, want 3", e.scr.cursor.Line)
	}
}

func TestExSubstitute(t *testing.T) {
	exCase(t, "subst-first", "one one\n", []string{"s/one/1/"}, "1 one")
	exCase(t, "subst-global", "one one\n", []string{"s/one/1/g"}, "1 1")
	exCase(t, "subst-range", "a\na\na\n", []string{"1,2s/a/b/"}, "b\nb\na")
	exCase(t, "subst-whole", "foo\nfoo\n", []string{"%s/o/0/g"}, "f00\nf00")
	exCase(t, "subst-amp", "cat\n", []string{"s/cat/[&]/"}, "[cat]")
	exCase(t, "subst-backref", "John Smith\n", []string{`s/\([A-Za-z]*\) \([A-Za-z]*\)/\2 \1/`}, "Smith John")
	exCase(t, "subst-upper", "hello\n", []string{`s/.*/\U&/`}, "HELLO")
	exCase(t, "subst-delete", "axbxc\n", []string{"s/x//g"}, "abc")
	// \n is the letter n in nvi; the line is split by a literal CR (see
	// TestSubstReplacementEscapes).
	exCase(t, "subst-newline", "a,b,c\n", []string{"s/,/\r/g"}, "a\nb\nc")
	// A replacement with no closing delimiter keeps its trailing blanks: they
	// ARE the replacement text (nvi), so ":s/b/<tab>" yields "a<tab>c", not "ac".
	exCase(t, "subst-open-tab", "abc\n", []string{"s/b/\t"}, "a\tc")
	exCase(t, "subst-open-space", "abc\n", []string{"s/b/ "}, "a c")
	exCase(t, "subst-open-tab-x", "abc\n", []string{"s/b/\tX"}, "a\tXc")
}

func TestExGlobal(t *testing.T) {
	exCase(t, "global-delete", "keep\ndrop x\nkeep\ndrop y\n", []string{"g/drop/d"}, "keep\nkeep")
	exCase(t, "global-subst", "a1\nb\na2\n", []string{"g/a/s/a/X/"}, "X1\nb\nX2")
	exCase(t, "vglobal-delete", "a\nb\na\nc\n", []string{"v/a/d"}, "a\na")
}

// nvi regsub: only \digit, \&, and the case controls are special in a
// replacement; \n and \t are the literal letters (newline-as-\n is sed/vim).
// A literal (^V-quoted) CR or NL character is what breaks the line.
func TestSubstReplacementEscapes(t *testing.T) {
	exCase(t, "repl-backslash-n", "one two three\n", []string{`s/two/X\nY/`}, "one XnY three")
	exCase(t, "repl-backslash-t", "one two three\n", []string{`s/two/X\tY/`}, "one XtY three")
	exCase(t, "repl-literal-cr", "one two three\n", []string{"s/two/X\rY/"}, "one X\nY three")
	exCase(t, "repl-literal-cr-escaped", "one two three\n", []string{"s/two/X\\\rY/"}, "one X\nY three")
}

// nvi's :s cursor rule (ex_subst.c slno/scno): after a substitution the
// cursor goes to the first nonblank of the last substituted line, UNLESS the
// last replaced match started exactly at the pre-command cursor position, in
// which case the cursor stays put.
func TestSubstCursor(t *testing.T) {
	// /two leaves the cursor at (1,4); s/two/T/ replaces the match starting
	// there, so the cursor stays at column 4.
	e, _, _ := newTestEngine(t, "one two three\n")
	drive(e, "/two\r")
	if err := e.exExecute("s/two/T/"); err != nil {
		t.Fatal(err)
	}
	if e.scr.cursor != (Pos{Line: 1, Col: 4}) {
		t.Fatalf("kept: cursor %+v, want 1,4", e.scr.cursor)
	}
	// Cursor at end of line; the match starts elsewhere: first nonblank.
	e2, _, _ := newTestEngine(t, "  one two\n")
	drive(e2, "$")
	if err := e2.exExecute("s/two/T/"); err != nil {
		t.Fatal(err)
	}
	if e2.scr.cursor != (Pos{Line: 1, Col: 2}) {
		t.Fatalf("moved: cursor %+v, want 1,2", e2.scr.cursor)
	}
}

// nvi regsub checks the magic option per special: under nomagic & is a
// literal ampersand and \& is the whole match; ~ / \~ flip the same way.
func TestSubstReplacementNomagic(t *testing.T) {
	exCase(t, "nomagic-amp", "abc\n", []string{"set nomagic", "s/b/{&}/"}, "a{&}c")
	exCase(t, "nomagic-esc-amp", "abc\n", []string{"set nomagic", "s/b/[\\&]/"}, "a[b]c")
	exCase(t, "nomagic-tilde", "one two\n", []string{"s/one/X/", "set nomagic", "s/two/~/"}, "X ~")
	exCase(t, "nomagic-esc-tilde", "one two\n", []string{"s/one/X/", "set nomagic", "s/two/[\\~]/"}, "X [X]")
	// Magic-side controls stay as they were.
	exCase(t, "magic-amp", "abc\n", []string{"s/b/{&}/"}, "a{b}c")
	exCase(t, "magic-esc-amp", "abc\n", []string{`s/b/[\&]/`}, "a[&]c")
}

// TestSubstConfirm walks a :s///c substitution through its prompts: each
// candidate shows "Confirm change? [n]" with the buffer still unchanged and
// the cursor on the match; y substitutes, n declines, q stops the command.
func TestSubstConfirm(t *testing.T) {
	e, _, _ := newTestEngine(t, "foo bar foo baz\nsecond foo\nthird foo\n")
	if err := e.exExecute("%s/foo/X/gc"); err != nil {
		t.Fatal(err)
	}
	check := func(step string, line int64, col int, prompt bool) {
		t.Helper()
		if prompt && e.scr.msg != "Confirm change? [n]" {
			t.Fatalf("%s: msg = %q, want confirm prompt", step, e.scr.msg)
		}
		if !prompt && e.scr.msg == "Confirm change? [n]" {
			t.Fatalf("%s: confirm prompt still up", step)
		}
		if e.scr.cursor.Line != line || e.scr.cursor.Col != col {
			t.Fatalf("%s: cursor %d,%d, want %d,%d",
				step, e.scr.cursor.Line, e.scr.cursor.Col, line, col)
		}
	}
	check("start", 1, 0, true)
	if got := bufText(e); got != "foo bar foo baz\nsecond foo\nthird foo" {
		t.Fatalf("buffer changed before an answer: %q", got)
	}
	drive(e, "y") // accept the first: line 1 becomes "X bar foo baz"
	check("after y", 1, 6, true)
	if got := bufText(e); got != "X bar foo baz\nsecond foo\nthird foo" {
		t.Fatalf("after y: %q", got)
	}
	drive(e, "n") // decline the second foo on line 1; prompt moves to line 2
	check("after n", 2, 7, true)
	drive(e, "q") // stop: line 3 is never asked about
	if e.scr.subConfirm != nil {
		t.Fatal("confirm still pending after q")
	}
	check("after q", 2, 7, false)
	if got := bufText(e); got != "X bar foo baz\nsecond foo\nthird foo" {
		t.Fatalf("after q: %q", got)
	}
	// The accepted replacements undo as one unit.
	drive(e, "u")
	if got := bufText(e); got != "foo bar foo baz\nsecond foo\nthird foo" {
		t.Fatalf("after undo: %q", got)
	}
}

// TestSubstConfirmNoGlobal checks that without the g flag only the first
// match on each line is offered.
func TestSubstConfirmNoGlobal(t *testing.T) {
	e, _, _ := newTestEngine(t, "foo bar foo baz\nplain\nfoo again\n")
	if err := e.exExecute("%s/foo/X/c"); err != nil {
		t.Fatal(err)
	}
	drive(e, "y") // line 1 first foo; second foo is not offered
	if e.scr.cursor.Line != 3 || e.scr.cursor.Col != 0 {
		t.Fatalf("second prompt at %d,%d, want 3,0",
			e.scr.cursor.Line, e.scr.cursor.Col)
	}
	drive(e, "y")
	if got := bufText(e); got != "X bar foo baz\nplain\nX again" {
		t.Fatalf("buffer: %q", got)
	}
	if e.scr.subConfirm != nil {
		t.Fatal("confirm still pending after the last candidate")
	}
}

// TestExGlobalCursor checks the final cursor of a :g whose body edits lines:
// nvi leaves it on the line of the last insert/delete the body performed
// (ex.c range_lno), which for :m0 is the line after the last moved-from
// position, not the moved line's new home at the top (QA-18).
func TestExGlobalCursor(t *testing.T) {
	e, _, _ := newTestEngine(t, "foo bar foo baz\nsecond line here\nthird here\nfourth line\n")
	if err := e.exExecute("g/here/m0"); err != nil {
		t.Fatal(err)
	}
	if got := bufText(e); got != "third here\nsecond line here\nfoo bar foo baz\nfourth line" {
		t.Fatalf("buffer after :g/here/m0 = %q", got)
	}
	if e.scr.cursor.Line != 4 {
		t.Errorf(":g/here/m0 cursor line %d, want 4", e.scr.cursor.Line)
	}

	// A body command with no line inserts/deletes leaves the cursor on the
	// last visited match.
	e2, _, _ := newTestEngine(t, "a1\nb\na2\nc\n")
	if err := e2.exExecute("g/a/s/a/X/"); err != nil {
		t.Fatal(err)
	}
	if e2.scr.cursor.Line != 3 {
		t.Errorf(":g/a/s cursor line %d, want 3", e2.scr.cursor.Line)
	}

	// When the last touched line is gone, the cursor clamps to the last line.
	e3, _, _ := newTestEngine(t, "keep\ndrop\nkeep\ndrop\n")
	if err := e3.exExecute("g/drop/d"); err != nil {
		t.Fatal(err)
	}
	if e3.scr.cursor.Line != 2 {
		t.Errorf(":g/drop/d cursor line %d, want 2", e3.scr.cursor.Line)
	}
}
