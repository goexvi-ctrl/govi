package engine

import "testing"

func TestMapCommand(t *testing.T) {
	e, _, _ := newTestEngine(t, "alpha\nbeta\ngamma\n")
	e.exExecute("map X dd") // X deletes a line
	drive(e, "X")
	if got := bufText(e); got != "beta\ngamma" {
		t.Fatalf("map X dd: got %q", got)
	}
}

func TestMapMultiChar(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\ntwo\n")
	e.exExecute("map ,d dd")
	drive(e, ",d")
	if got := bufText(e); got != "two" {
		t.Fatalf("map ,d dd: got %q", got)
	}
}

func TestMapPrefixAmbiguity(t *testing.T) {
	// "z" and "zz" both mapped; typing "z" then a non-z resolves the short map.
	e, _, _ := newTestEngine(t, "a\nb\nc\n")
	e.exExecute("map zz G")  // zz -> last line
	e.exExecute("map z 2G")  // z -> line 2
	// Type z then j: 'z' is a prefix of 'zz', so it waits; 'j' is not 'z', so
	// the short map 'z' fires (-> line 2), then 'j' moves down to line 3.
	drive(e, "zj")
	if e.scr.cursor.Line != 3 {
		t.Fatalf("ambiguous map: line %d, want 3", e.scr.cursor.Line)
	}
}

func TestMapTimeout(t *testing.T) {
	e, _, _ := newTestEngine(t, "a\nb\nc\nd\n")
	e.exExecute("map zz G")
	e.exExecute("map z 2G")
	drive(e, "z")                  // ambiguous: waits
	if e.scr.cursor.Line != 1 {
		t.Fatalf("should still be waiting, line %d", e.scr.cursor.Line)
	}
	e.Input(TimeoutEvent{})        // timeout resolves to short map z -> 2G
	if e.scr.cursor.Line != 2 {
		t.Fatalf("after timeout: line %d, want 2", e.scr.cursor.Line)
	}
}

func TestMapInsert(t *testing.T) {
	e, _, _ := newTestEngine(t, "\n")
	e.exExecute("map! jk \x1b") // jk -> ESC in insert mode (literal escape)
	drive(e, "ihello")
	drive(e, "jk") // exit insert via the map
	drive(e, "x")  // now in command mode, delete a char
	if got := bufText(e); got != "hell" {
		t.Fatalf("insert map jk->ESC: got %q", got)
	}
}

func TestUnmap(t *testing.T) {
	e, _, _ := newTestEngine(t, "a\nb\n")
	e.exExecute("map X dd")
	e.exExecute("unmap X")
	drive(e, "X") // no longer mapped; 'X' is delete-char-before (no-op at col 0)
	if got := bufText(e); got != "a\nb" {
		t.Fatalf("after unmap: got %q", got)
	}
}

func TestAbbreviate(t *testing.T) {
	e, _, _ := newTestEngine(t, "\n")
	e.exExecute("ab teh the")
	drive(e, "iteh \x1b") // typing space after 'teh' expands it
	if got := bufText(e); got != "the " {
		t.Fatalf("abbrev: got %q", got)
	}
}

func TestAbbreviateOnEsc(t *testing.T) {
	e, _, _ := newTestEngine(t, "\n")
	e.exExecute("ab fn function")
	drive(e, "ifn\x1b")
	if got := bufText(e); got != "function" {
		t.Fatalf("abbrev on esc: got %q", got)
	}
}
