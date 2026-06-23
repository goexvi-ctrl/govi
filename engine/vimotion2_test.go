package engine

import "testing"

func curAt(t *testing.T, e *Engine, line int64, col int, what string) {
	t.Helper()
	if e.scr.cursor.Line != line || e.scr.cursor.Col != col {
		t.Fatalf("%s: cursor at %+v, want line %d col %d", what, e.scr.cursor, line, col)
	}
}

func TestMatchMotion(t *testing.T) {
	e, _, _ := newTestEngine(t, "a(bcd)e\n")
	drive(e, "f(") // onto the '('
	drive(e, "%")
	curAt(t, e, 1, 5, "% to closing") // ')'
	drive(e, "%")
	curAt(t, e, 1, 1, "% back to opening")
}

func TestParagraphMotions(t *testing.T) {
	e, _, _ := newTestEngine(t, "a\nb\n\nc\nd\n\ne\n")
	drive(e, "}")
	curAt(t, e, 3, 0, "} to first blank")
	drive(e, "}")
	curAt(t, e, 6, 0, "} to second blank")
	drive(e, "{")
	curAt(t, e, 3, 0, "{ back to first blank")
}

func TestSectionMotions(t *testing.T) {
	e, _, _ := newTestEngine(t, "code\n{\nblock\n}\nmore\n")
	drive(e, "]]")
	curAt(t, e, 2, 0, "]] to section")
	drive(e, "G")
	drive(e, "[[")
	curAt(t, e, 2, 0, "[[ back to section")
}

func TestUnderscoreMotion(t *testing.T) {
	e, _, _ := newTestEngine(t, "  one\n  two\n  three\n")
	drive(e, "2_") // count-1 = 1 line down, first non-blank
	curAt(t, e, 2, 2, "2_")
}

func TestZScreenPosition(t *testing.T) {
	e, _, _ := newTestEngine(t, "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n")
	e.Resize(5, 40)
	drive(e, "5G")  // cursor to line 5
	drive(e, "z\r") // line 5 to top
	if e.scr.top != 5 {
		t.Fatalf("z<CR>: top = %d, want 5", e.scr.top)
	}
	drive(e, "z.") // center line 5 (rows=5 -> top = 5 - 2 = 3)
	if e.scr.top != 3 {
		t.Fatalf("z.: top = %d, want 3", e.scr.top)
	}
}

func TestExecBuffer(t *testing.T) {
	e, _, _ := newTestEngine(t, "dd\nfoo\nbar\n")
	drive(e, `"ayy`) // register a = "dd"
	drive(e, "j")    // line 2
	drive(e, "@a")   // run "dd" -> delete line 2
	if bufText(e) != "dd\nbar" {
		t.Fatalf("@a: got %q", bufText(e))
	}
}

func TestInsertCtrlW(t *testing.T) {
	e, _, _ := newTestEngine(t, "\n")
	drive(e, "ifoo bar")
	e.Input(KeyEvent{Rune: 'w', Mods: ModCtrl}) // erase "bar"
	drive(e, "baz\x1b")
	if bufText(e) != "foo baz" {
		t.Fatalf("^W: got %q", bufText(e))
	}
}

func TestFileInfoMessage(t *testing.T) {
	e, _, _ := newTestEngine(t, "a\nb\nc\n")
	drive(e, "j")
	e.Input(KeyEvent{Rune: 'g', Mods: ModCtrl})
	msg, kind := (view{e.scr}).Message()
	if kind != MsgInfo || msg == "" {
		t.Fatalf("^G message = %q kind %v", msg, kind)
	}
}
