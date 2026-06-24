package engine

import "testing"

func TestLiteralRuneCtrlA(t *testing.T) {
	r, ok := literalRune(KeyEvent{Rune: 'a', Mods: ModCtrl})
	if !ok || r != 1 {
		t.Fatalf("literalRune(^A) = %q, %v want 1, true", r, ok)
	}
}

func TestLiteralRuneCtrlAt(t *testing.T) {
	r, ok := literalRune(KeyEvent{Rune: '@', Mods: ModCtrl})
	if !ok || r != 0 {
		t.Fatalf("literalRune(^@) = %q, %v want NUL, true", r, ok)
	}
}

func TestLiteralRuneCtrlNulRune(t *testing.T) {
	// macOS sends the control code directly (NUL) with ModCtrl still set.
	r, ok := literalRune(KeyEvent{Rune: 0, Mods: ModCtrl})
	if !ok || r != 0 {
		t.Fatalf("literalRune(^@ raw) = %q, %v want NUL, true", r, ok)
	}
}

func TestLiteralRuneCtrlRawSOH(t *testing.T) {
	// macOS sends SOH (1) with ModCtrl for ^A, not letter 'a'.
	r, ok := literalRune(KeyEvent{Rune: 1, Mods: ModCtrl})
	if !ok || r != 1 {
		t.Fatalf("literalRune(^A raw) = %q, %v want 1, true", r, ok)
	}
}

func TestInsertLiteralCtrlA(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	drive(e, "i")
	e.Input(KeyEvent{Rune: 'v', Mods: ModCtrl})
	e.Input(KeyEvent{Rune: 'a', Mods: ModCtrl})
	drive(e, "\x1b")
	runes := e.scr.lineRunes(1)
	if len(runes) != 2 || runes[0] != 1 || runes[1] != 'x' {
		t.Fatalf("buffer = %v, want SOH + x", runes)
	}
	if got := FormatVisibleControls(runes); got != "^Ax" {
		t.Fatalf("display = %q, want ^Ax", got)
	}
}

func TestInsertLiteralNul(t *testing.T) {
	e, _, _ := newTestEngine(t, "hi\n")
	drive(e, "i")
	e.Input(KeyEvent{Rune: 'v', Mods: ModCtrl})
	e.Input(KeyEvent{Rune: '@', Mods: ModCtrl})
	drive(e, "\x1b")
	runes := e.scr.lineRunes(1)
	if len(runes) != 3 || runes[0] != 0 {
		t.Fatalf("buffer = %v, want NUL first", runes)
	}
	if got := FormatVisibleControls(runes); got != "^@hi" {
		t.Fatalf("display = %q, want ^@hi", got)
	}
}

func TestColonLiteralCtrlA(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	drive(e, ":")
	e.Input(KeyEvent{Rune: 'v', Mods: ModCtrl})
	e.Input(KeyEvent{Rune: 'a', Mods: ModCtrl})
	if len(e.scr.colon) != 1 || e.scr.colon[0] != 1 {
		t.Fatalf("colon = %v, want [1]", e.scr.colon)
	}
	msg, _ := (view{e.scr}).Message()
	if msg != ":^A" {
		t.Fatalf("msg = %q, want :^A", msg)
	}
}

func TestColonLiteralNul(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	drive(e, ":")
	e.Input(KeyEvent{Rune: 'v', Mods: ModCtrl})
	e.Input(KeyEvent{Rune: '@', Mods: ModCtrl})
	if len(e.scr.colon) != 1 || e.scr.colon[0] != 0 {
		t.Fatalf("colon = %v, want [NUL]", e.scr.colon)
	}
	msg, _ := (view{e.scr}).Message()
	if msg != ":^@" {
		t.Fatalf("msg = %q, want :^@", msg)
	}
}