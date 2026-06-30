package engine

import (
	"os"
	"path/filepath"
	"testing"

	"govi/engine/register"
)

// twoFileSplit opens file a, sets a 24-row terminal, and splits in a new screen
// editing file b. It returns the engine and the two paths.
func twoFileSplit(t *testing.T) (*Engine, string, string) {
	t.Helper()
	dir := t.TempDir()
	a := filepath.Join(dir, "aaa.txt")
	b := filepath.Join(dir, "bbb.txt")
	if err := os.WriteFile(a, []byte("a1\na2\na3\na4\na5\na6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b1\nb2\nb3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fe := &captureFrontend{}
	e := New(fe, Options{})
	if err := e.Open(a); err != nil {
		t.Fatal(err)
	}
	e.Resize(23, 80) // 24-row terminal (23 text rows + 1 status)
	if err := e.editNewScreen(b); err != nil {
		t.Fatal(err)
	}
	return e, a, b
}

func TestSplitHorizGeometry(t *testing.T) {
	e, _, _ := twoFileSplit(t)

	if len(e.screens) != 2 {
		t.Fatalf("screens = %d, want 2", len(e.screens))
	}
	// Cursor was at the top of aaa (line 1), so old keeps the top half and the
	// new screen takes the bottom; focus moves to the new screen.
	top, bot := e.screens[0], e.screens[1]
	if filepath.Base(top.name) != "aaa.txt" || filepath.Base(bot.name) != "bbb.txt" {
		t.Fatalf("order: top=%s bot=%s", top.name, bot.name)
	}
	if e.scr != bot {
		t.Fatalf("focus should be on the new (bottom) screen")
	}
	// Display height 24 -> half=12. Old keeps disp-half=12 (11 text rows), new
	// gets 12 (11 text rows); roff 0 and 12.
	if top.roff != 0 || top.rows != 11 {
		t.Fatalf("top geometry: roff=%d rows=%d, want 0/11", top.roff, top.rows)
	}
	if bot.roff != 12 || bot.rows != 11 {
		t.Fatalf("bot geometry: roff=%d rows=%d, want 12/11", bot.roff, bot.rows)
	}
	if top.roff+top.rows+1 != bot.roff {
		t.Fatalf("screens not contiguous: %d vs %d", top.roff+top.rows+1, bot.roff)
	}
}

func TestSplitTooSmall(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	os.WriteFile(a, []byte("x\n"), 0o644)
	e := New(&captureFrontend{}, Options{})
	if err := e.Open(a); err != nil {
		t.Fatal(err)
	}
	e.Resize(2, 80) // 3-row terminal: display height 3 < 4, cannot split
	if err := e.editNewScreen(a); err == nil {
		t.Fatalf("expected split-too-small error")
	}
	if len(e.screens) != 1 {
		t.Fatalf("screens = %d after failed split, want 1", len(e.screens))
	}
}

func TestSwitchScreenCycles(t *testing.T) {
	e, _, _ := twoFileSplit(t)
	bot := e.screens[1]
	top := e.screens[0]
	if e.scr != bot {
		t.Fatalf("start focus should be bottom")
	}
	e.switchScreen() // bottom (idx1) -> wrap to top (idx0)
	if e.scr != top {
		t.Fatalf("^W should move to the top screen")
	}
	e.switchScreen() // top (idx0) -> bottom (idx1)
	if e.scr != bot {
		t.Fatalf("^W should cycle back to the bottom screen")
	}
}

func TestSwitchScreenSingleErrors(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\ntwo\n")
	e.switchScreen()
	if e.scr.msg == "" {
		t.Fatalf("^W with one screen should report an error message")
	}
}

func TestCloseScreenJoinBottomIntoTop(t *testing.T) {
	e, _, _ := twoFileSplit(t)
	// Focus is on the bottom screen; closing it folds its space into the top
	// screen (HORIZ_PRECEDE), which becomes active and full height.
	top := e.screens[0]
	e.closeCurrentScreen()
	if len(e.screens) != 1 || e.scr != top {
		t.Fatalf("after closing bottom: screens=%d active==top=%v", len(e.screens), e.scr == top)
	}
	if top.roff != 0 || top.rows != 23 {
		t.Fatalf("top should fill 24-row terminal: roff=%d rows=%d, want 0/23", top.roff, top.rows)
	}
}

func TestCloseScreenJoinTopIntoBottom(t *testing.T) {
	e, _, _ := twoFileSplit(t)
	e.switchScreen() // focus the top screen
	bot := e.screens[1]
	top := e.screens[0]
	if e.scr != top {
		t.Fatalf("expected focus on top")
	}
	e.closeCurrentScreen() // close top -> bottom moves up and grows (HORIZ_FOLLOW)
	if len(e.screens) != 1 || e.scr != bot {
		t.Fatalf("after closing top: screens=%d active==bot=%v", len(e.screens), e.scr == bot)
	}
	if bot.roff != 0 || bot.rows != 23 {
		t.Fatalf("bottom should fill terminal: roff=%d rows=%d, want 0/23", bot.roff, bot.rows)
	}
}

func TestSplitSharesRegisters(t *testing.T) {
	e, _, _ := twoFileSplit(t)
	// Yank a line in the bottom screen, switch to the top, and confirm the same
	// register content is visible (cut buffers are shared across split screens).
	e.scr.regs.StoreYank('a', register.Text{Lines: [][]rune{[]rune("shared")}})
	got := e.screens[0].regs.Get('a')
	if got.Empty() || string(got.Lines[0]) != "shared" {
		t.Fatalf("register not shared across screens: %+v", got)
	}
}

func TestSplitCopiesOptions(t *testing.T) {
	e, _, _ := twoFileSplit(t)
	// Options are per-screen: changing one screen's option must not affect the
	// other's.
	e.screens[0].opts.b["number"] = true
	if e.screens[1].opts.Bool("number") {
		t.Fatalf("options should be independent per screen")
	}
}
