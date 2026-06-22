package engine

import "testing"

func TestRangeText(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello\nworld\n")
	if got := e.RangeText(Pos{1, 1}, Pos{1, 4}); got != "ell" {
		t.Errorf("intra-line RangeText = %q, want %q", got, "ell")
	}
	if got := e.RangeText(Pos{1, 3}, Pos{2, 2}); got != "lo\nwo" {
		t.Errorf("multi-line RangeText = %q, want %q", got, "lo\nwo")
	}
	// Reversed args are ordered.
	if got := e.RangeText(Pos{1, 4}, Pos{1, 1}); got != "ell" {
		t.Errorf("reversed RangeText = %q, want %q", got, "ell")
	}
}

func TestDeleteRange(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello\n")
	e.DeleteRange(Pos{1, 1}, Pos{1, 3}) // remove "el"
	if got := bufText(e); got != "hlo" {
		t.Errorf("DeleteRange = %q, want %q", got, "hlo")
	}
	if e.scr.cursor != (Pos{1, 1}) {
		t.Errorf("cursor after DeleteRange = %+v, want {1,1}", e.scr.cursor)
	}
}

func TestDeleteRangeMultiline(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello\nworld\n")
	e.DeleteRange(Pos{1, 3}, Pos{2, 2}) // remove "lo\nwo" -> "helrld"
	if got := bufText(e); got != "helrld" {
		t.Errorf("multiline DeleteRange = %q, want %q", got, "helrld")
	}
}

func TestReplaceSelectionType(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello\n")
	// Replace "he" with "X" and keep typing "I": result "XIllo", insert mode.
	e.ReplaceSelectionType(Pos{1, 0}, Pos{1, 2}, "X")
	if e.scr.mode != ModeInsert {
		t.Fatalf("mode after ReplaceSelectionType = %v, want Insert", e.scr.mode)
	}
	drive(e, "I\x1b") // continue typing, then ESC closes the change
	if got := bufText(e); got != "XIllo" {
		t.Errorf("ReplaceSelectionType = %q, want %q", got, "XIllo")
	}
	// The whole thing is one undo unit.
	drive(e, "u")
	if got := bufText(e); got != "hello" {
		t.Errorf("undo after ReplaceSelectionType = %q, want %q", got, "hello")
	}
}

func TestReplaceSelectionText(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello\n")
	e.ReplaceSelectionText(Pos{1, 1}, Pos{1, 3}, "XY") // "h" + "XY" + "lo"
	if got := bufText(e); got != "hXYlo" {
		t.Errorf("ReplaceSelectionText = %q, want %q", got, "hXYlo")
	}
	if e.scr.mode != ModeCommand {
		t.Errorf("mode after ReplaceSelectionText = %v, want Command", e.scr.mode)
	}
	drive(e, "u")
	if got := bufText(e); got != "hello" {
		t.Errorf("undo after ReplaceSelectionText = %q, want %q", got, "hello")
	}
}

func TestInsertTextAndMultiline(t *testing.T) {
	e, _, _ := newTestEngine(t, "ac\n")
	e.MoveCursorTo(1, 1) // caret before 'c'
	e.InsertText("b")
	if got := bufText(e); got != "abc" {
		t.Errorf("InsertText = %q, want %q", got, "abc")
	}
	e.InsertText("X\nY") // splits the line
	if got := bufText(e); got != "abX\nYc" {
		t.Errorf("multiline InsertText = %q, want %q", got, "abX\nYc")
	}
}

func TestMoveCursorClamps(t *testing.T) {
	e, _, _ := newTestEngine(t, "ab\ncd\n")
	e.MoveCursorTo(99, 99)
	if e.scr.cursor.Line != 2 {
		t.Errorf("clamped line = %d, want 2", e.scr.cursor.Line)
	}
	// Command mode clamps the column to the last rune.
	if e.scr.cursor.Col != 1 {
		t.Errorf("clamped col = %d, want 1", e.scr.cursor.Col)
	}
}
