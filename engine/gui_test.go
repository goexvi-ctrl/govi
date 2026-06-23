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

func TestWordRange(t *testing.T) {
	e, _, _ := newTestEngine(t, "foo bar_baz, qux\n")
	// Click inside "bar_baz" (underscore is a word rune).
	a, b := e.WordRange(1, 5)
	if a != (Pos{1, 4}) || b != (Pos{1, 11}) {
		t.Errorf("WordRange in identifier = %+v..%+v, want {1,4}..{1,11}", a, b)
	}
	if got := e.RangeText(a, b); got != "bar_baz" {
		t.Errorf("word text = %q, want %q", got, "bar_baz")
	}
	// Click on the comma (punctuation) selects just it.
	a, b = e.WordRange(1, 11)
	if got := e.RangeText(a, b); got != "," {
		t.Errorf("punct word = %q, want %q", got, ",")
	}
	// Click on whitespace selects the run of blanks.
	a, b = e.WordRange(1, 3)
	if got := e.RangeText(a, b); got != " " {
		t.Errorf("blank word = %q, want a single space", got)
	}
}

func TestWordRangeCustom(t *testing.T) {
	e, _, _ := newTestEngine(t, "foo-bar baz\n")
	// Default: '-' is punctuation, so it breaks the word.
	a, b := e.WordRange(1, 0)
	if got := e.RangeText(a, b); got != "foo" {
		t.Errorf("default word = %q, want %q", got, "foo")
	}
	// Custom boundary treating '-' as a word rune joins "foo-bar".
	e.SetWordBoundary(func(line []rune, col int) (int, int) {
		isW := func(r rune) bool { return r == '-' || r == '_' || (r >= 'a' && r <= 'z') }
		n := len(line)
		if n == 0 {
			return 0, 0
		}
		if col >= n {
			col = n - 1
		}
		if !isW(line[col]) {
			return col, col + 1
		}
		s, en := col, col+1
		for s > 0 && isW(line[s-1]) {
			s--
		}
		for en < n && isW(line[en]) {
			en++
		}
		return s, en
	})
	a, b = e.WordRange(1, 0)
	if got := e.RangeText(a, b); got != "foo-bar" {
		t.Errorf("custom word = %q, want %q", got, "foo-bar")
	}
	// nil restores the default.
	e.SetWordBoundary(nil)
	a, b = e.WordRange(1, 0)
	if got := e.RangeText(a, b); got != "foo" {
		t.Errorf("restored default word = %q, want %q", got, "foo")
	}
}

func TestLineSelectRange(t *testing.T) {
	e, _, _ := newTestEngine(t, "alpha\nbeta\n")
	// A non-final line selects through the start of the next line (newline
	// included), so the copied text is a full line.
	a, b := e.LineSelectRange(1)
	if a != (Pos{1, 0}) || b != (Pos{2, 0}) {
		t.Errorf("LineSelectRange(1) = %+v..%+v, want {1,0}..{2,0}", a, b)
	}
	if got := e.RangeText(a, b); got != "alpha\n" {
		t.Errorf("line text = %q, want %q", got, "alpha\n")
	}
	// The last line has no following line; it ends at end-of-line.
	a, b = e.LineSelectRange(2)
	if a != (Pos{2, 0}) || b != (Pos{2, 4}) {
		t.Errorf("LineSelectRange(last) = %+v..%+v, want {2,0}..{2,4}", a, b)
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

func TestScrollLines(t *testing.T) {
	e, _, _ := newTestEngine(t, "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n")
	e.Resize(4, 20) // small viewport
	if e.scr.top != 1 {
		t.Fatalf("initial top = %d", e.scr.top)
	}
	e.ScrollLines(3) // scroll down 3 lines
	if e.scr.top != 4 {
		t.Errorf("after scroll +3, top = %d, want 4", e.scr.top)
	}
	// Cursor is not moved by scrolling.
	if e.scr.cursor.Line != 1 {
		t.Errorf("scroll moved cursor to %d, want 1", e.scr.cursor.Line)
	}
	e.ScrollLines(-2)
	if e.scr.top != 2 {
		t.Errorf("after scroll -2, top = %d, want 2", e.scr.top)
	}
	// Clamps at the top.
	e.ScrollLines(-100)
	if e.scr.top != 1 {
		t.Errorf("clamp top = %d, want 1", e.scr.top)
	}
	// Clamps at the last line.
	e.ScrollLines(1000)
	if e.scr.top != 10 {
		t.Errorf("clamp bottom = %d, want 10", e.scr.top)
	}
}
