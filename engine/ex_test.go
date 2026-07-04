package engine

import "testing"

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
