package engine

import "testing"

func TestRuneWidthWide(t *testing.T) {
	cases := []struct {
		r    rune
		want int
	}{
		{'a', 1},
		{'A', 1},
		{'日', 2}, // CJK ideograph
		{'本', 2},
		{'Ａ', 2},  // fullwidth Latin A
		{'é', 1},  // accented Latin, narrow
		{'\t', 8}, // tab at column 0
	}
	for _, tc := range cases {
		if got := runeWidth(tc.r, 0, 8); got != tc.want {
			t.Errorf("runeWidth(%q) = %d, want %d", tc.r, got, tc.want)
		}
	}
}

func TestDisplayWidthWide(t *testing.T) {
	e, _, _ := newTestEngine(t, "a日本z\n") // 1 + 2 + 2 + 1 = 6 columns
	if got := e.scr.displayWidth(1); got != 6 {
		t.Fatalf("displayWidth = %d, want 6", got)
	}
}

func TestDisplayCellsWide(t *testing.T) {
	dl := makeDisplayLine([]rune("a日b"), 8)
	cells := DisplayCells(dl)
	// a (1) + 日 (2: glyph + continuation) + b (1) = 4 cells.
	if len(cells) != 4 {
		t.Fatalf("len(cells) = %d, want 4", len(cells))
	}
	if cells[0].Rune != 'a' || cells[1].Rune != '日' || cells[2].Rune != 0 || cells[3].Rune != 'b' {
		t.Fatalf("cells = %+v", cells)
	}
}

func TestDisplayColumnWide(t *testing.T) {
	dl := makeDisplayLine([]rune("a日本z"), 8)
	// Column (rune index) -> display column.
	checks := map[int]int{0: 0, 1: 1, 2: 3, 3: 5}
	for col, want := range checks {
		if got := DisplayColumn(dl, col); got != want {
			t.Errorf("DisplayColumn(rune %d) = %d, want %d", col, got, want)
		}
	}
}

func TestWrapWithWideChars(t *testing.T) {
	// 6 wide chars = 12 columns; at width 10 that wraps to 2 rows.
	e, _, _ := newTestEngine(t, "日本語日本語\n")
	e.Resize(10, 10)
	if got := e.scr.displayWidth(1); got != 12 {
		t.Fatalf("displayWidth = %d, want 12", got)
	}
	if got := e.scr.screenLines(1); got != 2 {
		t.Fatalf("screenLines = %d, want 2", got)
	}
}
