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
	exCase(t, "subst-newline", "a,b,c\n", []string{`s/,/\n/g`}, "a\nb\nc")
}

func TestExGlobal(t *testing.T) {
	exCase(t, "global-delete", "keep\ndrop x\nkeep\ndrop y\n", []string{"g/drop/d"}, "keep\nkeep")
	exCase(t, "global-subst", "a1\nb\na2\n", []string{"g/a/s/a/X/"}, "X1\nb\nX2")
	exCase(t, "vglobal-delete", "a\nb\na\nc\n", []string{"v/a/d"}, "a\na")
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
