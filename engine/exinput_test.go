package engine

import "testing"

func TestExAppend(t *testing.T) {
	e, _, _ := newTestEngine(t, "a\nc\n")
	drive(e, "Q")        // ex mode
	drive(e, "1a\r")     // append after line 1
	drive(e, "X\rY\r")   // two input lines
	drive(e, ".\r")      // terminator
	if got := bufText(e); got != "a\nX\nY\nc" {
		t.Fatalf("ex append: %q", got)
	}
}

func TestExInsert(t *testing.T) {
	e, _, _ := newTestEngine(t, "a\nc\n")
	drive(e, "Q2i\rB\r.\r")
	if got := bufText(e); got != "a\nB\nc" {
		t.Fatalf("ex insert: %q", got)
	}
}

func TestExChange(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\ntwo\nthree\n")
	drive(e, "Q2c\rNEW\r.\r")
	if got := bufText(e); got != "one\nNEW\nthree" {
		t.Fatalf("ex change: %q", got)
	}
}

func TestExChangeRange(t *testing.T) {
	e, _, _ := newTestEngine(t, "a\nb\nc\nd\n")
	drive(e, "Q2,3c\rX\r.\r")
	if got := bufText(e); got != "a\nX\nd" {
		t.Fatalf("ex change range: %q", got)
	}
}

func TestExAppendFromColon(t *testing.T) {
	// :a from vi command mode also collects input.
	e, _, _ := newTestEngine(t, "a\nc\n")
	drive(e, ":1a\r")
	drive(e, "Z\r.\r")
	if got := bufText(e); got != "a\nZ\nc" {
		t.Fatalf("colon append: %q", got)
	}
}
