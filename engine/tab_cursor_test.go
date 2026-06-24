package engine

import "testing"

func TestCursorDisplayColumnTab(t *testing.T) {
	dl := makeDisplayLine([]rune("\tx"), 8, false)
	if got := CursorDisplayColumn(dl, 0, ModeCommand); got != 7 {
		t.Fatalf("command mode on tab: col %d, want 7", got)
	}
	if got := CursorDisplayColumn(dl, 0, ModeInsert); got != 0 {
		t.Fatalf("insert mode on tab: col %d, want 0", got)
	}
}

func TestCursorDisplayColumnWide(t *testing.T) {
	dl := makeDisplayLine([]rune("a日b"), 8, false)
	if got := CursorDisplayColumn(dl, 1, ModeCommand); got != 2 {
		t.Fatalf("command mode on wide char: col %d, want 2", got)
	}
	if got := CursorDisplayColumn(dl, 1, ModeInsert); got != 1 {
		t.Fatalf("insert mode on wide char: col %d, want 1", got)
	}
}

func TestVerticalMotionPreservesTabEndColumn(t *testing.T) {
	e, _, _ := newTestEngine(t, "\tx\n\n")
	drive(e, "0") // onto tab; records desiredCol at end of tab (7)
	if e.scr.cursor.Col != 0 {
		t.Fatalf("cursor col = %d, want 0 (on tab)", e.scr.cursor.Col)
	}
	if e.scr.desiredCol != 7 {
		t.Fatalf("desiredCol = %d, want 7", e.scr.desiredCol)
	}
	drive(e, "j")
	if e.scr.cursor.Line != 2 {
		t.Fatalf("after j line = %d, want 2", e.scr.cursor.Line)
	}
	drive(e, "k")
	if e.scr.cursor.Col != 0 {
		t.Fatalf("after k back onto tab, col = %d, want 0", e.scr.cursor.Col)
	}
}
