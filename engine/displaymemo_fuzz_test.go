package engine

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

func dlEqual(a, b DisplayLine) bool {
	if a.List != b.List || len(a.Text) != len(b.Text) || len(a.Widths) != len(b.Widths) {
		return false
	}
	for i := range a.Text {
		if a.Text[i] != b.Text[i] {
			return false
		}
	}
	for i := range a.Widths {
		if a.Widths[i] != b.Widths[i] {
			return false
		}
	}
	return true
}

// fuzzDisplayLine builds a line that exercises every runeWidth branch: tabs,
// control chars, DEL, plain ASCII, an accented (narrow non-ASCII) rune, and a
// wide CJK rune -- so a stale memo would show up as a width as well as a text
// mismatch.
func fuzzDisplayLine(rng *rand.Rand) []rune {
	palette := []rune{'a', 'b', ' ', '\t', '\x01', '\x1f', '\x7f', '世', 'é', 'Z'}
	n := rng.IntN(14)
	out := make([]rune, n)
	for i := range out {
		out[i] = palette[rng.IntN(len(palette))]
	}
	return out
}

// TestDisplayMemoMatchesFresh drives the engine through random edits, undo, and
// tabstop/list option changes, and after every mutation checks that the
// memoized displayLine equals a freshly computed makeDisplayLine for every line.
// The memo is populated before each mutation, so any missed invalidation (stale
// text or widths after an edit, or after a tabstop/list change) is caught.
func TestDisplayMemoMatchesFresh(t *testing.T) {
	steps := 4000
	if testing.Short() {
		steps = 800
	}
	var seed strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&seed, "seed line %d\twith\ttabs and 世 wide\n", i)
	}
	e, _, _ := newTestEngine(t, seed.String())
	s := e.scr
	rng := rand.New(rand.NewPCG(99, 2024))

	verify := func(stepDesc string) {
		t.Helper()
		tab := s.opts.Int("tabstop")
		list := s.opts.Bool("list")
		n := s.store.Lines()
		for lno := int64(1); lno <= n; lno++ {
			got := s.displayLine(lno)
			fresh := makeDisplayLine(cloneRunesEngine(s.lineRunes(lno)), tab, list)
			if !dlEqual(got, fresh) {
				t.Fatalf("memo stale after %s at line %d:\n memo  text=%q widths=%v list=%v\n fresh text=%q widths=%v list=%v",
					stepDesc, lno, string(got.Text), got.Widths, got.List,
					string(fresh.Text), fresh.Widths, fresh.List)
			}
		}
	}

	verify("init")
	for step := 0; step < steps; step++ {
		// Populate the memo for every line so a later staleness shows up.
		n := s.store.Lines()
		for lno := int64(1); lno <= n; lno++ {
			s.displayLine(lno)
		}

		switch rng.IntN(8) {
		case 0, 1, 2: // edit a line's content
			if n > 0 {
				lno := int64(rng.IntN(int(n))) + 1
				e.beginChange()
				s.setLine(lno, fuzzDisplayLine(rng))
				e.endChange()
			}
		case 3: // insert a line
			lno := int64(rng.IntN(int(n)+1)) + 1
			e.beginChange()
			s.insertLine(min64(lno, n+1), fuzzDisplayLine(rng))
			e.endChange()
		case 4: // delete a line
			if n > 1 {
				lno := int64(rng.IntN(int(n))) + 1
				e.beginChange()
				s.deleteLine(lno)
				e.endChange()
			}
		case 5: // toggle list (caught via dlList, not the gen)
			if s.opts.Bool("list") {
				e.RunEx("set nolist")
			} else {
				e.RunEx("set list")
			}
		case 6: // change tabstop (caught via dlTab, not the gen)
			e.RunEx(fmt.Sprintf("set tabstop=%d", 1+rng.IntN(8)))
		case 7: // undo (a content change that must bump the gen)
			e.RunEx("undo")
		}

		verify(fmt.Sprintf("step %d", step))
	}
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// cloneRunesEngine copies a rune slice so the fresh DisplayLine does not alias
// the live buffer line (makeDisplayLine keeps the slice in Text).
func cloneRunesEngine(r []rune) []rune {
	out := make([]rune, len(r))
	copy(out, r)
	return out
}
