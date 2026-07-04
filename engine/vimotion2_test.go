package engine

import (
	"fmt"
	"strings"
	"testing"
)

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
	if row := e.scr.screenRowOf(5, e.scr.top); row != 2 {
		t.Fatalf("z.: line 5 at screen row %d, want 2 (middle of 5)", row)
	}
}

func TestZScreenPositionWrap(t *testing.T) {
	// One long wrapped line above a short target line: logical-line z. used to
	// leave the target ~80% down the screen.
	long := strings.Repeat("x", 25)
	e, _, _ := newTestEngine(t, long+"\nshort\n")
	e.Resize(4, 10) // 4 text rows, 10 cols -> long line uses 3 rows
	drive(e, "2G")
	drive(e, "z.")

	if e.scr.top != 2 {
		t.Fatalf("z. wrap: top = %d, want 2", e.scr.top)
	}
	if row := e.scr.screenRowOf(2, e.scr.top); row != 0 {
		t.Fatalf("z. wrap: line 2 at screen row %d, want 0", row)
	}

	// Ten single-line rows: target should land near the vertical center.
	var b strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&b, "line%d\n", i)
	}
	e, _, _ = newTestEngine(t, b.String())
	e.Resize(10, 40)
	drive(e, "5G")
	drive(e, "z.")
	if row := e.scr.screenRowOf(5, e.scr.top); row < 4 || row > 5 {
		t.Fatalf("z. center: line 5 at screen row %d, want 4 or 5", row)
	}
}

func TestZScreenPositionLineAndWindow(t *testing.T) {
	e, _, _ := newTestEngine(t, "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n")
	e.Resize(10, 40)

	// 12z.: line 12 clamps to 10, centered in the full window.
	drive(e, "1G")
	drive(e, "12z.")
	if e.scr.cursor.Line != 10 {
		t.Fatalf("12z.: cursor line = %d, want 10", e.scr.cursor.Line)
	}
	if row := e.scr.screenRowOf(10, e.scr.top); row < 4 || row > 5 {
		t.Fatalf("12z.: line 10 at screen row %d, want near center", row)
	}

	// 8z<CR>: explicit line at top.
	drive(e, "8z\r")
	if e.scr.top != 8 || e.scr.cursor.Line != 8 {
		t.Fatalf("8z<CR>: top=%d cursor=%d, want 8/8", e.scr.top, e.scr.cursor.Line)
	}

	// 8z3.: line 8 at top in a 3-row map (vs_crel); type is ignored.
	drive(e, "8z3.")
	if e.scr.rows != 10 {
		t.Fatalf("8z3.: full rows = %d, want 10", e.scr.rows)
	}
	if e.scr.mapRows != 3 {
		t.Fatalf("8z3.: mapRows = %d, want 3", e.scr.mapRows)
	}
	if e.scr.top != 8 || e.scr.cursor.Line != 8 {
		t.Fatalf("8z3.: top=%d cursor=%d, want 8/8", e.scr.top, e.scr.cursor.Line)
	}
	if row := e.scr.screenRowOf(8, e.scr.top); row != 0 {
		t.Fatalf("8z3.: line 8 at screen row %d, want 0 (top)", row)
	}

	// z3-, z3<enter> match z3.
	drive(e, "5G")
	drive(e, "z3-")
	if e.scr.mapRows != 3 || e.scr.top != 5 {
		t.Fatalf("z3-: mapRows=%d top=%d, want 3/5", e.scr.mapRows, e.scr.top)
	}
	drive(e, "6G")
	drive(e, "z3\r")
	if e.scr.mapRows != 3 || e.scr.top != 6 {
		t.Fatalf("z3<CR>: mapRows=%d top=%d, want 3/6", e.scr.mapRows, e.scr.top)
	}

	// j expands the map when the cursor passes the reduced window (nvi IS_SMALL).
	drive(e, "1G")
	drive(e, "z3.")
	if e.scr.mapRows != 3 {
		t.Fatalf("z3.: mapRows = %d, want 3", e.scr.mapRows)
	}
	drive(e, "jjj") // line 4 is below the 3-row map
	if e.scr.mapRows != 4 {
		t.Fatalf("after jjj: mapRows = %d, want 4", e.scr.mapRows)
	}
	if e.scr.cursor.Line != 4 {
		t.Fatalf("after jjj: cursor line = %d, want 4", e.scr.cursor.Line)
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

// qaWrapFixture is the QA wrap fixture at its 12x40 geometry (11 text rows,
// 40 columns): lines 1 and 4 are 57 characters and wrap onto two rows.
func qaWrapFixture(t *testing.T) *Engine {
	long := "alpha beta gamma delta epsilon zeta eta theta iota kappa"
	lines := []string{long, "second short", "third line here", long, "fifth",
		"sixth", "seventh", "eighth", "ninth", "tenth", "eleventh", "twelfth"}
	e, _, _ := newTestEngine(t, strings.Join(lines, "\n")+"\n")
	e.Resize(11, 40)
	return e
}

// TestPageOverWraps checks that ^F/^B page in physical screen rows when
// wrapped lines are on screen (nvi vs_sm_up/vs_sm_down): the cursor keeps its
// screen-row index while the view scrolls under it and then walks the
// remaining rows, so it lands on nvi's line, not offset-many logical lines
// away (QA-11).
func TestPageOverWraps(t *testing.T) {
	e := qaWrapFixture(t)

	// 14 content rows in an 11-row window: ^F wants 9 rows but only 3 exist
	// below the bottom; the cursor still moves the full 9 rows (0+9 = row 6
	// of the new screen = line 8).
	e.Input(KeyEvent{Rune: 'f', Mods: ModCtrl})
	if e.scr.top != 3 {
		t.Fatalf("^F: top %d, want 3", e.scr.top)
	}
	curAt(t, e, 8, 0, "^F over wraps")

	// From the bottom (top=3, cursor line 12 = row 10), ^B scrolls back the 3
	// available rows and moves the cursor up 9 rows total, from (12,0) to
	// (4,0): screen row 4 of the new screen, below the four rows of lines 1-3.
	drive(e, "G")
	e.Input(KeyEvent{Rune: 'b', Mods: ModCtrl})
	if e.scr.top != 1 {
		t.Fatalf("G^B: top %d, want 1", e.scr.top)
	}
	curAt(t, e, 4, 0, "^B over wraps")
}
