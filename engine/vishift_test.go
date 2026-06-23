package engine

import "testing"

func TestViShiftDoubled(t *testing.T) {
	viCase(t, ">>", "x\n", ">>", "\tx") // sw=ts=8 -> one tab
	viCase(t, "<<", "\tx\n", "<<", "x") // dedent removes the tab
	viCase(t, ">>-already-indented", "\tx\n", ">>", "\t\tx")
	viCase(t, "2>>", "a\nb\nc\n", "2>>", "\ta\n\tb\nc")
}

func TestViShiftMotion(t *testing.T) {
	viCase(t, ">j", "a\nb\nc\n", ">j", "\ta\n\tb\nc")
	viCase(t, ">G", "a\nb\n", ">G", "\ta\n\tb")
	viCase(t, "<j-dedent", "\ta\n\tb\nc\n", "<j", "a\nb\nc")
}

func TestViShiftWidth(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.exExecute("set sw=4 ts=8")
	drive(e, ">>")
	if got := bufText(e); got != "    x" { // 4 spaces (less than a tabstop)
		t.Fatalf(">> with sw=4: got %q", got)
	}
}

func TestViShiftCursor(t *testing.T) {
	e, _, _ := newTestEngine(t, "  foo\n")
	drive(e, "$") // move off column 0
	drive(e, ">>")
	// Cursor lands on the first non-blank of the shifted line.
	if e.scr.cursor.Col != e.scr.firstNonBlank(1) {
		t.Fatalf(">> cursor col = %d, want first non-blank %d", e.scr.cursor.Col, e.scr.firstNonBlank(1))
	}
}
