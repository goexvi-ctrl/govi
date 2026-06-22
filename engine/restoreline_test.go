package engine

import "testing"

func TestURestoresLine(t *testing.T) {
	viCase(t, "U", "hello\n", "xxxU", "hello")
	viCase(t, "UU-idempotent", "hello\n", "xxxUU", "hello")
	viCase(t, "U-then-edit", "abcdef\n", "xUx", "bcdef")
}

func TestUSnapshotRefresh(t *testing.T) {
	// U restores only the run of changes on the current line, stopping at the
	// inter-line change.
	viCase(t, "U-refresh", "hello\nworld\n", "xjxkxU", "ello\norld")
}

func TestUWithDirectionalUndo(t *testing.T) {
	// After U undoes all three deletions, a following u redoes the first one.
	viCase(t, "U-then-u", "hello\n", "xxxUu", "ello")
}

func TestUNoChanges(t *testing.T) {
	// U with nothing to restore on the line is a no-op.
	e, _, _ := newTestEngine(t, "hello\n")
	drive(e, "U")
	if bufText(e) != "hello" {
		t.Fatalf("U with no changes: %q", bufText(e))
	}
}

func TestUOnlyCurrentLine(t *testing.T) {
	// Editing line 2 must not let U on line 1 touch line 2.
	e, _, _ := newTestEngine(t, "aaa\nbbb\n")
	drive(e, "jx")  // line 2 -> "bb"
	drive(e, "kxU") // line 1 -> "aa", then U restores line 1 only
	if bufText(e) != "aaa\nbb" {
		t.Fatalf("U touched another line: %q", bufText(e))
	}
}
