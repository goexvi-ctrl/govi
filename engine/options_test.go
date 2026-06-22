package engine

import "testing"

func TestSetBool(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if e.scr.opts.autoindent {
		t.Fatal("autoindent should default off")
	}
	if err := e.exExecute("set ai"); err != nil {
		t.Fatal(err)
	}
	if !e.scr.opts.autoindent {
		t.Fatal("set ai did not enable autoindent")
	}
	e.exExecute("set noai")
	if e.scr.opts.autoindent {
		t.Fatal("set noai did not disable autoindent")
	}
	e.exExecute("set ai!")
	if !e.scr.opts.autoindent {
		t.Fatal("set ai! did not toggle on")
	}
}

func TestSetNumeric(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if err := e.exExecute("set ts=4 sw=2"); err != nil {
		t.Fatal(err)
	}
	if e.scr.opts.tabstop != 4 {
		t.Fatalf("tabstop = %d, want 4", e.scr.opts.tabstop)
	}
	if e.scr.opts.shiftwidth != 2 {
		t.Fatalf("shiftwidth = %d, want 2", e.scr.opts.shiftwidth)
	}
}

func TestSetIgnorecaseAffectsSearch(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello WORLD\n")
	e.exExecute("set ic")
	drive(e, "/world\r")
	if e.scr.cursor.Col != 6 {
		t.Fatalf("ic search -> col %d, want 6", e.scr.cursor.Col)
	}
}

func TestSetShiftwidth(t *testing.T) {
	exCase(t, "sw=4-shift", "x\n", []string{"set sw=4 ts=8", "1>"}, "    x")
}

func TestAutoindent(t *testing.T) {
	e, _, _ := newTestEngine(t, "    foo\n")
	e.exExecute("set ai")
	drive(e, "obar\x1b") // open below: should inherit 4-space indent
	if got := bufText(e); got != "    foo\n    bar" {
		t.Fatalf("autoindent o: got %q", got)
	}
}

func TestAutoindentNewline(t *testing.T) {
	e, _, _ := newTestEngine(t, "\n")
	e.exExecute("set ai")
	drive(e, "i    abc\rdef\x1b") // after Enter, def should be indented 4
	if got := bufText(e); got != "    abc\n    def" {
		t.Fatalf("autoindent newline: got %q", got)
	}
}

func TestWrapscanOff(t *testing.T) {
	e, _, _ := newTestEngine(t, "foo\nbar\nfoo\n")
	e.exExecute("set nows")
	drive(e, "G")       // last line (a foo)
	drive(e, "/foo\r")  // forward, no wrap -> no match below, stays
	if e.scr.cursor.Line != 3 {
		t.Fatalf("nows search wrapped or moved: line %d, want 3", e.scr.cursor.Line)
	}
}
