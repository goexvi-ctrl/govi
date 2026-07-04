package engine

import (
	"testing"
	"time"
)

// TestSetEncodingOptions covers CORNERS B-8: the fileencoding and inputencoding
// options (with fe/ie abbreviations) are recognized and settable, so :set no
// longer errors on them (govi is UTF-8 internally, so the value is cosmetic).
func TestSetEncodingOptions(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if got := e.scr.opts.Str("fileencoding"); got == "" {
		t.Error("fileencoding should have a non-empty locale default")
	}
	for _, set := range []string{"set fileencoding=utf-8", "set inputencoding=iso-8859-1", "set fe=latin1", "set ie=utf-8"} {
		if err := e.exExecute(set); err != nil {
			t.Errorf("%q: %v", set, err)
		}
	}
	if got := e.scr.opts.Str("fileencoding"); got != "latin1" {
		t.Errorf("fileencoding = %q, want latin1", got)
	}
	if got := e.scr.opts.Str("inputencoding"); got != "utf-8" {
		t.Errorf("inputencoding = %q, want utf-8", got)
	}
}

func TestSetBool(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if e.scr.opts.Bool("autoindent") {
		t.Fatal("autoindent should default off")
	}
	if err := e.exExecute("set ai"); err != nil {
		t.Fatal(err)
	}
	if !e.scr.opts.Bool("autoindent") {
		t.Fatal("set ai did not enable autoindent")
	}
	e.exExecute("set noai")
	if e.scr.opts.Bool("autoindent") {
		t.Fatal("set noai did not disable autoindent")
	}
	e.exExecute("set ai!")
	if !e.scr.opts.Bool("autoindent") {
		t.Fatal("set ai! did not toggle on")
	}
}

func TestSetNumeric(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if err := e.exExecute("set ts=4 sw=2"); err != nil {
		t.Fatal(err)
	}
	if e.scr.opts.Int("tabstop") != 4 {
		t.Fatalf("tabstop = %d, want 4", e.scr.opts.Int("tabstop"))
	}
	if e.scr.opts.Int("shiftwidth") != 2 {
		t.Fatalf("shiftwidth = %d, want 2", e.scr.opts.Int("shiftwidth"))
	}
}

func TestSetTabsPrefix(t *testing.T) {
	e, _, _ := newTestEngine(t, "a\tb\n")
	if err := e.exExecute("set tabs=4"); err != nil {
		t.Fatal(err)
	}
	if e.scr.opts.Int("tabstop") != 4 {
		t.Fatalf("tabstop = %d, want 4", e.scr.opts.Int("tabstop"))
	}
	if got := e.scr.displayWidth(1); got != 5 { // 'a' + 3 to next tab @ 4
		t.Fatalf("display width with ts=4 = %d, want 5", got)
	}
}

func TestSetAmbiguousPrefix(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if err := e.exExecute("set ta=4"); err == nil {
		t.Fatal("ta should be ambiguous (tabstop, taglength, tags, ...)")
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

func TestSetForegroundBackground(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if err := e.exExecute("set foreground=#ff0000 background=wheat"); err != nil {
		t.Fatal(err)
	}
	if got := e.scr.opts.Str("foreground"); got != "#ff0000" {
		t.Fatalf("foreground = %q, want #ff0000", got)
	}
	if got := e.scr.opts.Str("background"); got != "wheat" {
		t.Fatalf("background = %q, want wheat", got)
	}
	if err := e.exExecute("set fg=blue"); err != nil {
		t.Fatal(err)
	}
	if got := e.scr.opts.Str("foreground"); got != "blue" {
		t.Fatalf("fg = %q, want blue", got)
	}
}

func TestWrapscanOff(t *testing.T) {
	e, _, _ := newTestEngine(t, "foo\nbar\nfoo\n")
	e.exExecute("set nows")
	drive(e, "G")      // last line (a foo)
	drive(e, "/foo\r") // forward, no wrap -> no match below, stays
	if e.scr.cursor.Line != 3 {
		t.Fatalf("nows search wrapped or moved: line %d, want 3", e.scr.cursor.Line)
	}
}

func TestSetRefresh(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if got := e.scr.opts.Str("refresh"); got != "20ms" {
		t.Fatalf("default refresh = %q, want 20ms", got)
	}
	if err := e.exExecute("set refresh=100ms"); err != nil {
		t.Fatal(err)
	}
	if got := e.scr.opts.Str("refresh"); got != "100ms" {
		t.Fatalf("refresh = %q, want 100ms", got)
	}
	if got := e.RefreshInterval(); got != 100*time.Millisecond {
		t.Fatalf("RefreshInterval = %v, want 100ms", got)
	}
	if err := e.exExecute("set refresh=0"); err != nil {
		t.Fatal(err)
	}
	if got := e.RefreshInterval(); got != 0 {
		t.Fatalf("RefreshInterval = %v, want 0", got)
	}
	if err := e.exExecute("set refresh=nope"); err == nil {
		t.Fatal("invalid refresh should fail")
	}
}
