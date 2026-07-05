package engine

import (
	"strings"
	"testing"
)

// The terminal special edit keys nvi seeds from terminfo as command-mode
// maps (cl/cl_term.c c_tklist): Insert (kich1) -> i, PageUp (kpp) -> ^B,
// PageDown (knp) -> ^F. qa/CORNERS.md Part C #10.

func TestInsertKeyEntersInsertMode(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\ntwo\n")
	e.Input(KeyEvent{Key: KeyInsert})
	if e.scr.mode != ModeInsert {
		t.Fatalf("mode = %v, want insert", e.scr.mode)
	}
	for _, r := range "zap" {
		e.Input(KeyEvent{Rune: r})
	}
	e.Input(KeyEvent{Key: KeyEscape})
	if got := string(e.scr.lineRunes(1)); got != "zapone" {
		t.Fatalf("line 1 = %q, want zapone", got)
	}
}

func TestPageKeysActAsCtrlFB(t *testing.T) {
	lines := make([]string, 60)
	for i := range lines {
		lines[i] = "l"
	}
	content := strings.Join(lines, "\n") + "\n"

	// PageDown must land exactly where a typed ^F does.
	e1, _, _ := newTestEngine(t, content)
	e1.Input(KeyEvent{Key: KeyPageDown})
	e2, _, _ := newTestEngine(t, content)
	e2.Input(KeyEvent{Rune: 'f', Mods: ModCtrl})
	if e1.scr.cursor != e2.scr.cursor || e1.scr.top != e2.scr.top {
		t.Fatalf("PageDown cursor/top %+v/%d, ^F %+v/%d",
			e1.scr.cursor, e1.scr.top, e2.scr.cursor, e2.scr.top)
	}
	if e1.scr.cursor.Line == 1 {
		t.Fatal("PageDown did not move")
	}

	// PageUp returns, matching ^B.
	e1.Input(KeyEvent{Key: KeyPageUp})
	e2.Input(KeyEvent{Rune: 'b', Mods: ModCtrl})
	if e1.scr.cursor != e2.scr.cursor || e1.scr.top != e2.scr.top {
		t.Fatalf("PageUp cursor/top %+v/%d, ^B %+v/%d",
			e1.scr.cursor, e1.scr.top, e2.scr.cursor, e2.scr.top)
	}

	// A count applies as it would to the typed control key (2^F pages two).
	e1.Input(KeyEvent{Rune: '2'})
	e1.Input(KeyEvent{Key: KeyPageDown})
	e2.Input(KeyEvent{Rune: '2'})
	e2.Input(KeyEvent{Rune: 'f', Mods: ModCtrl})
	if e1.scr.cursor != e2.scr.cursor {
		t.Fatalf("2<PageDown> %+v, 2^F %+v", e1.scr.cursor, e2.scr.cursor)
	}
}

func TestSpecialKeysIgnoredInInsertMode(t *testing.T) {
	// nvi's terminfo maps are SEQ_COMMAND only; in input mode the keys do
	// nothing in govi (raw nvi would see the bytes, but govi gets decoded
	// keys and drops them, like other unbound named keys).
	e, _, _ := newTestEngine(t, "one\n")
	drive(e, "i")
	e.Input(KeyEvent{Key: KeyInsert})
	e.Input(KeyEvent{Key: KeyPageDown})
	e.Input(KeyEvent{Key: KeyPageUp})
	if e.scr.mode != ModeInsert {
		t.Fatal("special keys must not leave insert mode")
	}
	e.Input(KeyEvent{Key: KeyEscape})
	if got := string(e.scr.lineRunes(1)); got != "one" {
		t.Fatalf("line 1 = %q, want unchanged", got)
	}
}
