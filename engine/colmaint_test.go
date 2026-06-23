package engine

import "testing"

func TestColumnMaintenanceTab(t *testing.T) {
	// Line 1 "a\tb": 'b' is rune index 2 but display column 8. j should land on
	// the char at display column 8 of line 2.
	e, _, _ := newTestEngine(t, "a\tb\n0123456789\n")
	drive(e, "0ll") // onto 'b' (display col 8)
	drive(e, "j")
	if e.scr.cursor.Col != 8 {
		t.Fatalf("after j over tab: col %d, want 8 (display-col maintained)", e.scr.cursor.Col)
	}
	drive(e, "k")
	if e.scr.cursor.Col != 2 {
		t.Fatalf("after k back: col %d, want 2 (rune index of 'b')", e.scr.cursor.Col)
	}
}

func TestColumnMaintenanceWide(t *testing.T) {
	// Line 1 has wide chars; the cursor's display column must map to the right
	// rune on line 2.
	e, _, _ := newTestEngine(t, "日本x\nabcdef\n") // x at display col 4, rune idx 2
	drive(e, "0ll")                              // onto 'x' (display col 4)
	drive(e, "j")
	if e.scr.cursor.Col != 4 { // 'e' at display col 4 on line 2
		t.Fatalf("after j: col %d, want 4", e.scr.cursor.Col)
	}
}

func TestColumnMaintenanceShortLine(t *testing.T) {
	// Moving through a short line should not lose the desired column.
	e, _, _ := newTestEngine(t, "abcdefgh\nxy\nabcdefgh\n")
	drive(e, "5l") // col 5
	drive(e, "j")  // line 2 "xy": clamps to last col (1)
	if e.scr.cursor.Col != 1 {
		t.Fatalf("j onto short line: col %d, want 1", e.scr.cursor.Col)
	}
	drive(e, "j") // line 3: desired col 5 restored
	if e.scr.cursor.Col != 5 {
		t.Fatalf("j back to long line: col %d, want 5 (desired column kept)", e.scr.cursor.Col)
	}
}

func TestColumnMaintenanceStickyEOL(t *testing.T) {
	e, _, _ := newTestEngine(t, "abcdef\nxy\nlongword\n")
	drive(e, "$") // end of line 1
	drive(e, "j") // end of line 2
	if e.scr.cursor.Col != 1 {
		t.Fatalf("$ then j: col %d, want 1 (EOL of 'xy')", e.scr.cursor.Col)
	}
	drive(e, "j") // end of line 3
	if e.scr.cursor.Col != 7 {
		t.Fatalf("$ then jj: col %d, want 7 (EOL of 'longword')", e.scr.cursor.Col)
	}
}

func TestColumnMaintenanceResetByHorizontal(t *testing.T) {
	// A horizontal motion resets the desired column.
	e, _, _ := newTestEngine(t, "abcdefgh\nabcdefgh\n")
	drive(e, "$") // sticky EOL
	drive(e, "0") // reset to column 0
	drive(e, "j") // should be column 0, not EOL
	if e.scr.cursor.Col != 0 {
		t.Fatalf("0 then j after $: col %d, want 0", e.scr.cursor.Col)
	}
}
