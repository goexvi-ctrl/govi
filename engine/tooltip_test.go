package engine

import "testing"

func TestSetTooltip(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if e.scr.opts.Str("tooltip") != "hover" {
		t.Fatalf("tooltip default = %q, want hover", e.scr.opts.Str("tooltip"))
	}
	if e.scr.opts.Int("tooltipdelay") != 500 {
		t.Fatalf("tooltipdelay default = %d, want 500", e.scr.opts.Int("tooltipdelay"))
	}
	if e.scr.opts.Str("tooltipfile") != "" {
		t.Fatalf("tooltipfile default = %q, want empty", e.scr.opts.Str("tooltipfile"))
	}
	if err := e.exExecute("set tooltip=manual"); err != nil {
		t.Fatal(err)
	}
	if e.scr.opts.Str("tooltip") != "manual" {
		t.Fatalf("tooltip = %q, want manual", e.scr.opts.Str("tooltip"))
	}
	// Unique value prefixes resolve ("o" -> off), case-insensitively.
	if err := e.exExecute("set tooltip=O"); err != nil {
		t.Fatal(err)
	}
	if e.scr.opts.Str("tooltip") != "off" {
		t.Fatalf("tooltip = %q, want off after prefix O", e.scr.opts.Str("tooltip"))
	}
	if err := e.exExecute("set tooltip=bogus"); err == nil {
		t.Fatal("set tooltip=bogus should be rejected")
	}
	if e.scr.opts.Str("tooltip") != "off" {
		t.Fatalf("tooltip changed to %q after a rejected set", e.scr.opts.Str("tooltip"))
	}
	// The other two options: numeric delay and free-form file path. "tooltipd"
	// and "tooltipf" are the shortest unambiguous prefixes ("tooltip" itself is
	// an exact name).
	if err := e.exExecute("set tooltipd=1200 tooltipf=/tmp/x.tips"); err != nil {
		t.Fatal(err)
	}
	if e.scr.opts.Int("tooltipdelay") != 1200 {
		t.Fatalf("tooltipdelay = %d, want 1200", e.scr.opts.Int("tooltipdelay"))
	}
	if e.scr.opts.Str("tooltipfile") != "/tmp/x.tips" {
		t.Fatalf("tooltipfile = %q, want /tmp/x.tips", e.scr.opts.Str("tooltipfile"))
	}
	if err := e.exExecute("set toolt=hover"); err == nil {
		t.Fatal("set toolt= should be ambiguous across the three tooltip options")
	}
	// SetStrOption (the host path) canonicalizes and validates the same way.
	if err := e.SetStrOption("tooltip", "h"); err != nil {
		t.Fatal(err)
	}
	if e.scr.opts.Str("tooltip") != "hover" {
		t.Fatalf("tooltip = %q, want hover via SetStrOption", e.scr.opts.Str("tooltip"))
	}
	if err := e.SetStrOption("tooltip", "nope"); err == nil {
		t.Fatal("SetStrOption tooltip=nope should be rejected")
	}
	if e.IntOption("tooltipdelay") != 1200 {
		t.Fatalf("IntOption(tooltipdelay) = %d, want 1200", e.IntOption("tooltipdelay"))
	}
}
