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

func twoFileVsplit(t *testing.T) (*Engine, string, string) {
	t.Helper()
	dir := t.TempDir()
	a := filepath.Join(dir, "aaa.txt")
	b := filepath.Join(dir, "bbb.txt")
	os.WriteFile(a, []byte("a1\na2\na3\n"), 0o644)
	os.WriteFile(b, []byte("b1\nb2\n"), 0o644)
	e := New(&captureFrontend{}, Options{})
	if err := e.Open(a); err != nil {
		t.Fatal(err)
	}
	e.Resize(23, 80)
	if err := e.vsplitNewScreen(b); err != nil {
		t.Fatal(err)
	}
	return e, a, b
}

func TestVsplitGeometry(t *testing.T) {
	e, _, _ := twoFileVsplit(t)
	if len(e.screens) != 2 {
		t.Fatalf("screens = %d, want 2", len(e.screens))
	}
	left, right := e.screens[0], e.screens[1]
	if filepath.Base(left.name) != "aaa.txt" || filepath.Base(right.name) != "bbb.txt" {
		t.Fatalf("order: left=%s right=%s", left.name, right.name)
	}
	if e.scr != right {
		t.Fatalf("focus should be on the new (right) screen")
	}
	// 80 cols -> left=40, divider col 40, right=39 at coff 41. Rows unchanged.
	if left.coff != 0 || left.cols != 40 {
		t.Fatalf("left: coff=%d cols=%d, want 0/40", left.coff, left.cols)
	}
	if right.coff != 41 || right.cols != 39 {
		t.Fatalf("right: coff=%d cols=%d, want 41/39", right.coff, right.cols)
	}
	if left.rows != right.rows || left.roff != 0 || right.roff != 0 {
		t.Fatalf("rows/roff: left %d/%d right %d/%d", left.rows, left.roff, right.rows, right.roff)
	}
}

func TestVsplitTooNarrow(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	os.WriteFile(a, []byte("x\n"), 0o644)
	e := New(&captureFrontend{}, Options{})
	if err := e.Open(a); err != nil {
		t.Fatal(err)
	}
	e.Resize(23, 40) // 40 cols: cols/2 == 20 == minimum, cannot vsplit
	if err := e.vsplitNewScreen(a); err == nil {
		t.Fatalf("expected vsplit-too-narrow error")
	}
	if len(e.screens) != 1 {
		t.Fatalf("screens = %d after failed vsplit, want 1", len(e.screens))
	}
}

func TestVsplitCloseJoinsHorizontally(t *testing.T) {
	e, _, _ := twoFileVsplit(t)
	// Focus is the right screen; closing it gives its columns (and the divider)
	// to the left screen (VERT_PRECEDE), which becomes active and full width.
	left := e.screens[0]
	e.closeCurrentScreen()
	if len(e.screens) != 1 || e.scr != left {
		t.Fatalf("after closing right: screens=%d active==left=%v", len(e.screens), e.scr == left)
	}
	if left.coff != 0 || left.cols != 80 {
		t.Fatalf("left should fill width: coff=%d cols=%d, want 0/80", left.coff, left.cols)
	}
}

func TestBgBackgroundsAndFgSwaps(t *testing.T) {
	e, _, _ := twoFileSplit(t) // aaa top, bbb bottom (active)
	bbb := e.scr
	aaa := e.screens[0]
	// :bg backgrounds the active (bbb) and folds space into aaa.
	if err := e.exBg(&exCmd{}); err != nil {
		t.Fatal(err)
	}
	if len(e.screens) != 1 || e.scr != aaa || len(e.bg) != 1 || e.bg[0] != bbb {
		t.Fatalf("after :bg screens=%d active==aaa=%v bg=%d", len(e.screens), e.scr == aaa, len(e.bg))
	}
	if aaa.rows != 23 {
		t.Fatalf("aaa should fill the terminal after :bg, rows=%d", aaa.rows)
	}
	// :fg swaps aaa out and brings bbb back at aaa's geometry.
	if err := e.exFg(&exCmd{}); err != nil {
		t.Fatal(err)
	}
	if len(e.screens) != 1 || e.scr != bbb || len(e.bg) != 1 || e.bg[0] != aaa {
		t.Fatalf("after :fg active==bbb=%v bg has aaa=%v", e.scr == bbb, len(e.bg) == 1 && e.bg[0] == aaa)
	}
	if bbb.rows != 23 || bbb.roff != 0 {
		t.Fatalf("bbb should take aaa's geometry: rows=%d roff=%d", bbb.rows, bbb.roff)
	}
}

func TestBgOnlyScreenErrors(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\n")
	if err := e.exBg(&exCmd{}); err == nil {
		t.Fatalf(":bg with one screen should error")
	}
}

func TestFgNewScreenSplits(t *testing.T) {
	e, _, _ := twoFileSplit(t)
	bbb := e.scr
	_ = e.exBg(&exCmd{}) // background bbb; one screen (aaa) shown
	if len(e.screens) != 1 {
		t.Fatalf("setup: screens=%d", len(e.screens))
	}
	// :Fg brings bbb back as a new split.
	if err := e.exFg(&exCmd{newScreen: true}); err != nil {
		t.Fatal(err)
	}
	if len(e.screens) != 2 || len(e.bg) != 0 || e.scr != bbb {
		t.Fatalf("after :Fg screens=%d bg=%d active==bbb=%v", len(e.screens), len(e.bg), e.scr == bbb)
	}
}

func TestResizeGrowShrink(t *testing.T) {
	e, _, _ := twoFileSplit(t) // top rows 11, bottom rows 11
	e.switchScreen()           // focus the top screen
	top := e.scr
	bottom := e.screens[1]
	if err := e.resizeScreen(3, aIncrease); err != nil {
		t.Fatal(err)
	}
	if top.rows != 14 || bottom.rows != 8 || bottom.roff != 15 {
		t.Fatalf("grow: top.rows=%d bottom.rows=%d bottom.roff=%d, want 14/8/15", top.rows, bottom.rows, bottom.roff)
	}
	if err := e.resizeScreen(3, aDecrease); err != nil {
		t.Fatal(err)
	}
	if top.rows != 11 || bottom.rows != 11 || bottom.roff != 12 {
		t.Fatalf("shrink back: top.rows=%d bottom.rows=%d bottom.roff=%d, want 11/11/12", top.rows, bottom.rows, bottom.roff)
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
