package engine

import (
	"errors"
	"testing"
)

// Stage 2: the CPU-bound loops (search, :s, :g/:v) must observe the interrupt
// flag and abort with errInterrupted, leaving any work already done in place.
// These tests set the flag before invoking the operation via exExecute (which,
// unlike Input, does not clear it), so the loop aborts on its first check.

func TestInterruptAbortsSubstitute(t *testing.T) {
	e, _, _ := newTestEngine(t, "aaa\naaa\naaa\n")
	// Control: without an interrupt the substitution runs.
	if err := e.exExecute("%s/a/b/g"); err != nil {
		t.Fatalf("control substitute: %v", err)
	}
	if got := string(e.scr.lineRunes(1)); got != "bbb" {
		t.Fatalf("control substitute did not run: %q", got)
	}

	e2, _, _ := newTestEngine(t, "aaa\naaa\naaa\n")
	e2.Interrupt()
	err := e2.exExecute("%s/a/b/g")
	if !errors.Is(err, errInterrupted) {
		t.Fatalf("interrupted substitute err = %v, want errInterrupted", err)
	}
	if got := string(e2.scr.lineRunes(1)); got != "aaa" {
		t.Fatalf("substitute ran despite interrupt: %q", got)
	}
}

func TestInterruptAbortsGlobal(t *testing.T) {
	e, _, _ := newTestEngine(t, "x1\nx2\nx3\n")
	e.Interrupt()
	err := e.exExecute("g/x/d")
	if !errors.Is(err, errInterrupted) {
		t.Fatalf("interrupted global err = %v, want errInterrupted", err)
	}
	if e.scr.lineCount() != 3 {
		t.Fatalf("global deleted lines despite interrupt: count=%d", e.scr.lineCount())
	}
}

// TestGlobalIsOneUndoUnit covers CORNERS B-9: a :g runs as a single undo unit,
// so one u reverts every line it changed. The interrupt path shares this change
// group (it returns errInterrupted inside the same beginChange/endChange), so a
// ^C-interrupted :g keeps its partial changes and they undo as one unit too --
// matching nvi ex_global.c.
func TestGlobalIsOneUndoUnit(t *testing.T) {
	e, _, _ := newTestEngine(t, "x1\ny\nx2\nz\nx3\n")
	if err := e.exExecute("g/x/d"); err != nil {
		t.Fatal(err)
	}
	if got := bufText(e); got != "y\nz" {
		t.Fatalf("after g/x/d: %q", got)
	}
	drive(e, "u")
	if got := bufText(e); got != "x1\ny\nx2\nz\nx3" {
		t.Fatalf("one u after :g restored %q, want all lines back", got)
	}
}

func TestInterruptAbortsSearch(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\ntwo\nthree\n")
	e.Interrupt()
	err := e.startSearch("three", searchFwd)
	if !errors.Is(err, errInterrupted) {
		t.Fatalf("interrupted search err = %v, want errInterrupted", err)
	}
	// A genuine miss (no interrupt) still reports not-found, not "Interrupted".
	e2, _, _ := newTestEngine(t, "one\ntwo\n")
	err = e2.startSearch("zzz", searchFwd)
	if err == nil || errors.Is(err, errInterrupted) {
		t.Fatalf("miss err = %v, want a not-found error", err)
	}
}
