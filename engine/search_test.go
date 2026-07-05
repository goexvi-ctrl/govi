package engine

import (
	"strings"
	"testing"
	"time"
)

func TestSearchForward(t *testing.T) {
	e, _, _ := newTestEngine(t, "alpha\nbeta\ngamma\nbeta\n")
	drive(e, "/beta\r")
	if e.scr.cursor != (Pos{Line: 2, Col: 0}) {
		t.Fatalf("/beta -> %+v, want line2 col0", e.scr.cursor)
	}
	drive(e, "n") // next match wraps forward to line 4
	if e.scr.cursor.Line != 4 {
		t.Fatalf("n -> line %d, want 4", e.scr.cursor.Line)
	}
	drive(e, "n") // wraps back to line 2
	if e.scr.cursor.Line != 2 {
		t.Fatalf("n wrap -> line %d, want 2", e.scr.cursor.Line)
	}
	drive(e, "N") // opposite direction -> line 4
	if e.scr.cursor.Line != 4 {
		t.Fatalf("N -> line %d, want 4", e.scr.cursor.Line)
	}
}

// nvi re_conv: a ~ in a pattern is the last substitute replacement, spliced
// in verbatim; \~ is a literal tilde.  In nomagic the sense flips.
func TestSearchTildePattern(t *testing.T) {
	e, _, _ := newTestEngine(t, "one here\nx two y\na ~ b\n")
	if err := e.exExecute("1s/one/two/"); err != nil {
		t.Fatal(err)
	}
	drive(e, "1G0/~\r")
	if e.scr.cursor != (Pos{Line: 2, Col: 2}) {
		t.Fatalf("/~ -> %+v, want line2 col2 (the \"two\")", e.scr.cursor)
	}
	drive(e, `1G0/\~`+"\r")
	if e.scr.cursor != (Pos{Line: 3, Col: 2}) {
		t.Fatalf(`/\~ -> %+v, want line3 col2 (the literal ~)`, e.scr.cursor)
	}
	// nomagic flips the sense.
	if err := e.exExecute("set nomagic"); err != nil {
		t.Fatal(err)
	}
	drive(e, "1G0/~\r")
	if e.scr.cursor != (Pos{Line: 3, Col: 2}) {
		t.Fatalf("nomagic /~ -> %+v, want line3 col2", e.scr.cursor)
	}
	drive(e, `1G0/\~`+"\r")
	if e.scr.cursor != (Pos{Line: 2, Col: 2}) {
		t.Fatalf(`nomagic /\~ -> %+v, want line2 col2`, e.scr.cursor)
	}
}

// iclower (nvi): searches are case-insensitive as long as the pattern has no
// upper-case letter; one upper-case letter makes the search exact.
func TestSearchIclower(t *testing.T) {
	e, _, _ := newTestEngine(t, "xx ABC yy\nxx abc yy\n")
	if err := e.exExecute("set iclower"); err != nil {
		t.Fatal(err)
	}
	drive(e, "/abc\r")
	if e.scr.cursor != (Pos{Line: 1, Col: 3}) {
		t.Fatalf("iclower /abc -> %+v, want line1 col3 (ABC)", e.scr.cursor)
	}
	drive(e, "1G0/Abc\r") // upper-case present: exact, no match anywhere
	if e.scr.cursor != (Pos{Line: 1, Col: 0}) {
		t.Fatalf("iclower /Abc -> %+v, want unmoved (no match)", e.scr.cursor)
	}
}

func TestSearchBackward(t *testing.T) {
	e, _, _ := newTestEngine(t, "x foo\nbar\nfoo y\n")
	drive(e, "G")      // to last line
	drive(e, "?foo\r") // search backward
	if e.scr.cursor != (Pos{Line: 1, Col: 2}) {
		t.Fatalf("?foo -> %+v, want line1 col2", e.scr.cursor)
	}
}

func TestSearchColumn(t *testing.T) {
	e, _, _ := newTestEngine(t, "the quick brown\n")
	drive(e, "/quick\r")
	if e.scr.cursor != (Pos{Line: 1, Col: 4}) {
		t.Fatalf("/quick -> %+v, want line1 col4", e.scr.cursor)
	}
}

func TestSearchRegex(t *testing.T) {
	e, _, _ := newTestEngine(t, "abc123def\n")
	drive(e, "/[0-9]\r")
	if e.scr.cursor.Col != 3 {
		t.Fatalf("/[0-9] -> col %d, want 3", e.scr.cursor.Col)
	}
}

// TestSearchInterruptibleInsideOneLine pins the ^C escape hatch for a match
// that blows up exponentially within a single line (qa/CORNERS.md Part C
// #12): the regex Interrupt hook (compilePattern passes e.Interrupted) must
// break the match; the per-line poll in the search loop cannot see it. The
// search would otherwise run for ~2^64 steps.
func TestSearchInterruptibleInsideOneLine(t *testing.T) {
	e, _, _ := newTestEngine(t, strings.Repeat("a", 64)+"\n")
	done := make(chan struct{})
	go func() {
		defer close(done)
		drive(e, "/\\(a*\\)*b\r")
	}()
	time.Sleep(100 * time.Millisecond) // let the match start blowing up
	e.Interrupt()                      // the frontend-safe ^C entry point
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("search did not return after ^C; single-line match not interruptible")
	}
	if msg, k := e.scr.msg, e.scr.msgKind; k != MsgError || msg != "Interrupted" {
		t.Fatalf("msg = %q/%v, want Interrupted", msg, k)
	}
}
