package engine

import "testing"

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
