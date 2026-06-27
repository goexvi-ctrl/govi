package engine

import "testing"

func TestSetMode(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if e.scr.opts.Str("mode") != "contextual" {
		t.Fatalf("mode default = %q, want contextual", e.scr.opts.Str("mode"))
	}
	if err := e.exExecute("set mode=terminal"); err != nil {
		t.Fatal(err)
	}
	if e.scr.opts.Str("mode") != "terminal" {
		t.Fatalf("mode = %q, want terminal", e.scr.opts.Str("mode"))
	}
	if err := e.exExecute("set mode=p"); err != nil {
		t.Fatal(err)
	}
	if e.scr.opts.Str("mode") != "terminal" {
		t.Fatalf("mode = %q, want terminal after prefix p", e.scr.opts.Str("mode"))
	}
	if err := e.exExecute("set mode=hybrid"); err != nil {
		t.Fatal(err)
	}
	if e.scr.opts.Str("mode") != "contextual" {
		t.Fatalf("mode = %q, want contextual after alias hybrid", e.scr.opts.Str("mode"))
	}
	if err := e.exExecute("set mode=bogus"); err == nil {
		t.Fatal("set mode=bogus should be rejected")
	}
	if e.scr.opts.Str("mode") != "contextual" {
		t.Fatalf("mode changed to %q after a rejected set", e.scr.opts.Str("mode"))
	}
}
