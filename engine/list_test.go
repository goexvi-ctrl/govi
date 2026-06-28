package engine

import (
	"strings"
	"testing"
)

func TestListModeTabsAndDollar(t *testing.T) {
	dl := makeDisplayLine([]rune("a\tb"), 8, true)
	if got := DisplayLineWidth(dl); got != 5 { // a + ^I + b + $
		t.Fatalf("DisplayLineWidth = %d, want 5", got)
	}
	cells := DisplayCells(dl)
	want := []rune{'a', '^', 'I', 'b', '$'}
	if len(cells) != len(want) {
		t.Fatalf("len(cells) = %d, want %d", len(cells), len(want))
	}
	for i, r := range want {
		if cells[i].Rune != r {
			t.Fatalf("cells[%d].Rune = %q, want %q", i, cells[i].Rune, r)
		}
	}
}

func TestListModeControlChars(t *testing.T) {
	dl := makeDisplayLine([]rune{'\x01', 0x7f}, 8, true)
	cells := DisplayCells(dl)
	want := []rune{'^', 'A', '^', '?', '$'}
	for i, r := range want {
		if cells[i].Rune != r {
			t.Fatalf("cells[%d].Rune = %q, want %q", i, cells[i].Rune, r)
		}
	}
}

func TestListModeEmptyLine(t *testing.T) {
	dl := makeDisplayLine(nil, 8, true)
	if got := DisplayLineWidth(dl); got != 1 {
		t.Fatalf("DisplayLineWidth(empty) = %d, want 1", got)
	}
	cells := DisplayCells(dl)
	if len(cells) != 1 || cells[0].Rune != '$' {
		t.Fatalf("cells = %+v, want [$]", cells)
	}
}

func TestFormatColonLineTabExpanded(t *testing.T) {
	got := FormatColonLine([]rune("a\tb"), 8, false)
	want := "a       b"
	if got != want {
		t.Fatalf("FormatColonLine = %q, want %q", got, want)
	}
}

func TestFormatColonLineTabList(t *testing.T) {
	got := FormatColonLine([]rune("a\tb"), 8, true)
	want := "a^Ib"
	if got != want {
		t.Fatalf("FormatColonLine(list) = %q, want %q", got, want)
	}
}

func TestFormatVisibleControls(t *testing.T) {
	got := FormatVisibleControls([]rune("a\t\x0d"))
	want := "a^I^M"
	if got != want {
		t.Fatalf("FormatVisibleControls = %q, want %q", got, want)
	}
}

func TestFormatListLine(t *testing.T) {
	got := FormatListLine([]rune("a\t\x01"))
	want := "a^I^A$"
	if got != want {
		t.Fatalf("FormatListLine = %q, want %q", got, want)
	}
	if got := FormatListLine(nil); got != "$" {
		t.Fatalf("FormatListLine(empty) = %q, want $", got)
	}
}

func TestDisplayWidthListMode(t *testing.T) {
	e, _, _ := newTestEngine(t, "\t\t\tx\n")
	e.Resize(10, 10)
	if got := e.scr.displayWidth(1); got != 25 {
		t.Fatalf("displayWidth without list = %d, want 25", got)
	}
	e.exExecute("set list")
	if got := e.scr.displayWidth(1); got != 8 { // ^I^I^Ix + $
		t.Fatalf("displayWidth with list = %d, want 8", got)
	}
}

func TestWrapWithListTabs(t *testing.T) {
	e, _, _ := newTestEngine(t, "\t\t\tx\n")
	e.Resize(10, 10)
	e.exExecute("set list")
	if got := e.scr.screenLines(1); got != 1 {
		t.Fatalf("screenLines with list = %d, want 1", got)
	}
}

func TestPrintRangeList(t *testing.T) {
	e, _, _ := newTestEngine(t, "a\tb\n")
	if err := e.exExecute("list"); err != nil {
		t.Fatal(err)
	}
	// Print output goes to the paged overlay (nvi vs_msg), not the status line.
	if got := e.scr.pendingOutput; len(got) != 1 || got[0] != "a^Ib$" {
		t.Fatalf("pendingOutput = %q, want [a^Ib$]", got)
	}
}

func TestPrintRangeListOption(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello\n")
	if err := e.exExecute("set list"); err != nil {
		t.Fatal(err)
	}
	if err := e.exExecute("1print"); err != nil {
		t.Fatal(err)
	}
	if got := e.scr.pendingOutput; len(got) != 1 || got[0] != "hello$" {
		t.Fatalf("pendingOutput = %q, want [hello$]", got)
	}
}

func TestViewListAccessor(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.WithView(func(v View) {
		if v.List() {
			t.Fatal("list should be off by default")
		}
	})
	e.exExecute("set list")
	e.WithView(func(v View) {
		if !v.List() {
			t.Fatal("list should be on")
		}
		dl := v.Line(1)
		if !dl.List {
			t.Fatal("DisplayLine.List should be true")
		}
	})
}

func TestListModeWhitespaceLine(t *testing.T) {
	e, _, _ := newTestEngine(t, "   \n")
	e.exExecute("set list")
	e.WithView(func(v View) {
		cells := DisplayCells(v.Line(1))
		// Three spaces plus trailing $.
		if len(cells) != 4 || cells[3].Rune != '$' {
			t.Fatalf("cells = %+v", cells)
		}
	})
}

func TestListRuneWidthTab(t *testing.T) {
	if got := runeWidth('\t', 0, 8, true); got != 2 {
		t.Fatalf("runeWidth(tab, list) = %d, want 2", got)
	}
	if got := runeWidth('\t', 0, 8, false); got != 8 {
		t.Fatalf("runeWidth(tab, !list) = %d, want 8", got)
	}
}

func TestListWrapRowsOf(t *testing.T) {
	dl := makeDisplayLine([]rune(strings.Repeat("x", 10)), 8, true)
	// 10 chars + $ = 11 cols; at width 10 that's 2 rows.
	if got := wrapRows(DisplayLineWidth(dl), 10); got != 2 {
		t.Fatalf("wrapRows = %d, want 2", got)
	}
}
