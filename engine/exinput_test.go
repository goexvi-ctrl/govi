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

func TestExLineMode(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\ntwo\nthree\n")
	drive(e, "Q")
	if !e.ExActive() {
		t.Fatal("Q should activate ex mode")
	}
	if e.ExPrompt() != ":" {
		t.Fatalf("prompt = %q, want :", e.ExPrompt())
	}
	// Print lines 1,2: output returned, not stored in a screen transcript.
	out := e.ExFeedLine("1,2p")
	if len(out) != 2 || out[0] != "one" || out[1] != "two" {
		t.Fatalf("print output = %v", out)
	}
	// a/i/c input: prompt disappears while collecting.
	e.ExFeedLine("2a")
	if e.ExPrompt() != "" {
		t.Fatalf("prompt during input = %q, want empty", e.ExPrompt())
	}
	e.ExFeedLine("INS")
	e.ExFeedLine(".")
	if e.ExPrompt() != ":" {
		t.Fatal("prompt should return after .")
	}
	if bufText(e) != "one\ntwo\nINS\nthree" {
		t.Fatalf("after ex append: %q", bufText(e))
	}
	// visual leaves ex mode.
	e.ExFeedLine("visual")
	if e.ExActive() {
		t.Fatal("visual should leave ex mode")
	}
}
