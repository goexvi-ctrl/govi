package engine

import (
	"strings"
	"testing"
)

func TestScreenLines(t *testing.T) {
	e, _, _ := newTestEngine(t, strings.Repeat("x", 25)+"\nshort\n")
	e.Resize(10, 10) // 10 text columns, no number gutter
	if got := e.scr.screenLines(1); got != 3 {
		t.Fatalf("screenLines(25-col line @ w=10) = %d, want 3", got)
	}
	if got := e.scr.screenLines(2); got != 1 {
		t.Fatalf("screenLines(short) = %d, want 1", got)
	}
}

func TestScrollAccountsForWrap(t *testing.T) {
	// Five lines, each 25 cols, on a 10-col x 4-row screen: each line takes 3
	// screen rows, so only one full line fits at a time.
	var b strings.Builder
	for i := 0; i < 5; i++ {
		b.WriteString(strings.Repeat("abcde", 5)) // 25 cols
		b.WriteByte('\n')
	}
	e, _, _ := newTestEngine(t, b.String())
	e.Resize(4, 10)

	drive(e, "G") // jump to last line
	if e.scr.cursor.Line != 5 {
		t.Fatalf("G -> line %d", e.scr.cursor.Line)
	}
	// The cursor's line must be visible: top should be 5 (a single 3-row line
	// fits in 4 rows; line 4 would need 6 rows total).
	if e.scr.top != 5 {
		t.Fatalf("after G, top = %d, want 5 (wrap-aware scroll)", e.scr.top)
	}

	drive(e, "gg") // there is no gg; use 1G
	drive(e, "1G")
	if e.scr.top != 1 {
		t.Fatalf("after 1G, top = %d, want 1", e.scr.top)
	}
}

func TestWrapWithTabs(t *testing.T) {
	// A tab expands to the tabstop; ensure display width (not rune count) drives
	// wrapping.
	e, _, _ := newTestEngine(t, "\t\t\tx\n") // 3 tabs @ ts=8 = 24 cols + 1
	e.Resize(10, 10)
	if got := e.scr.displayWidth(1); got != 25 {
		t.Fatalf("displayWidth(3 tabs + x) = %d, want 25", got)
	}
	if got := e.scr.screenLines(1); got != 3 {
		t.Fatalf("screenLines = %d, want 3", got)
	}
}
