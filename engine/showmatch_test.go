package engine

import "testing"

func TestShowmatchFindsOpen(t *testing.T) {
	e, _, _ := newTestEngine(t, "\n")
	e.exExecute("set showmatch")
	drive(e, "i(abc")
	if e.scr.matchActive {
		t.Fatal("no match should be active before a close bracket")
	}
	e.Input(KeyEvent{Rune: ')'}) // type the closing paren
	if !e.scr.matchActive {
		t.Fatal("showmatch should activate on ')'")
	}
	if e.scr.matchPos != (Pos{Line: 1, Col: 0}) {
		t.Fatalf("matchPos = %+v, want line1 col0 (the '(')", e.scr.matchPos)
	}
	// The insertion point itself is unchanged (after the ')').
	if e.scr.cursor.Col != 5 {
		t.Fatalf("cursor col = %d, want 5", e.scr.cursor.Col)
	}
}

func TestShowmatchNested(t *testing.T) {
	e, _, _ := newTestEngine(t, "\n")
	e.exExecute("set sm")
	drive(e, "i(a{b")
	e.Input(KeyEvent{Rune: '}'})
	if !e.scr.matchActive || e.scr.matchPos.Col != 2 { // the '{'
		t.Fatalf("nested } match = %+v active=%v, want col2", e.scr.matchPos, e.scr.matchActive)
	}
}

// nvi flashes only for ')' and '}' (v_txt.c); ']' triggering showmatch is vim.
func TestShowmatchNotOnRightBracket(t *testing.T) {
	e, _, _ := newTestEngine(t, "\n")
	e.exExecute("set sm")
	drive(e, "i[a")
	e.Input(KeyEvent{Rune: ']'})
	if e.scr.matchActive {
		t.Fatal("']' must not trigger showmatch (nvi flashes only for ')' and '}')")
	}
	if bufText(e) != "[a]" {
		t.Fatalf("']' should still be inserted: %q", bufText(e))
	}
}

func TestShowmatchClearedByNextKey(t *testing.T) {
	e, _, _ := newTestEngine(t, "\n")
	e.exExecute("set sm")
	drive(e, "i()")
	if !e.scr.matchActive {
		t.Fatal("expected match active")
	}
	e.Input(KeyEvent{Rune: 'x'}) // next key clears the flash and is inserted
	if e.scr.matchActive {
		t.Fatal("match should clear on next key")
	}
	if bufText(e) != "()x" {
		t.Fatalf("next key not inserted normally: %q", bufText(e))
	}
}

func TestShowmatchClearedByTimeout(t *testing.T) {
	e, _, _ := newTestEngine(t, "\n")
	e.exExecute("set sm")
	drive(e, "i()")
	if !e.MatchPending() {
		t.Fatal("expected MatchPending")
	}
	e.Input(TimeoutEvent{})
	if e.scr.matchActive {
		t.Fatal("match should clear on timeout")
	}
}

func TestShowmatchOffByDefault(t *testing.T) {
	e, _, _ := newTestEngine(t, "\n")
	drive(e, "i()")
	if e.scr.matchActive {
		t.Fatal("showmatch off by default; no flash")
	}
}

func TestShowmatchNoMatch(t *testing.T) {
	e, _, _ := newTestEngine(t, "\n")
	e.exExecute("set sm")
	drive(e, "iabc")
	e.Input(KeyEvent{Rune: ')'}) // unmatched close
	if e.scr.matchActive {
		t.Fatal("unmatched ) should not activate showmatch")
	}
}
