package engine

import (
	"strings"
	"testing"
)

func TestSetAllShowsManyOptions(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.Resize(23, 80)
	drive(e, ":set all\r")
	out := e.scr.pendingOutput
	if out == nil {
		t.Fatal("set all produced no output overlay")
	}
	joined := strings.Join(out, "\n")
	for _, name := range []string{"autoindent", "ignorecase", "shiftwidth=8", "tabstop=8", "wrapscan", "shell=", "paragraphs="} {
		if !strings.Contains(joined, name) {
			t.Errorf("set all missing %q\n--- output ---\n%s", name, joined)
		}
	}
	// Should be a multi-line, multi-column grid, not one long line.
	if len(out) < 5 {
		t.Errorf("set all only %d lines; want a multi-line grid", len(out))
	}
	for i, line := range out {
		if len([]rune(line)) > 80 {
			t.Errorf("line %d exceeds 80 cols: %q", i, line)
		}
	}
	t.Logf("set all (%d lines):\n%s", len(out), joined)
}

func TestSetAllSortedByOptionName(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.Resize(23, 80)
	drive(e, ":set all\r")
	out := e.scr.pendingOutput

	// The "no" prefix is a display modifier only: options sort on the bare
	// name. "altwerase" sorts first, so "noaltwerase" leads the grid.
	if len(out) == 0 || !strings.HasPrefix(out[0], "noaltwerase") {
		t.Fatalf("first cell = %q, want noaltwerase (sorted by name, not display)", firstField(out))
	}

	// Reconstruct the first grid column and confirm it is sorted by bare name.
	var col1 []string
	for _, line := range out {
		f := strings.Fields(line)
		if len(f) < 2 { // skip the trailing single-entry long-value lines
			continue
		}
		col1 = append(col1, sortKey(f[0]))
	}
	for i := 1; i < len(col1); i++ {
		if col1[i] < col1[i-1] {
			t.Fatalf("first column not name-sorted: %q before %q", col1[i-1], col1[i])
		}
	}
}

func firstField(out []string) string {
	if len(out) == 0 {
		return ""
	}
	return out[0]
}

// sortKey reduces a displayed option to its sort name: drop "=value" and a
// leading "no".
func sortKey(disp string) string {
	if i := strings.IndexByte(disp, '='); i >= 0 {
		disp = disp[:i]
	}
	return strings.TrimPrefix(disp, "no")
}

func TestSetShowsOnlyChanged(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.Resize(23, 80)
	e.exExecute("set ai number sw=4")
	drive(e, ":set\r")
	joined := strings.Join(e.scr.pendingOutput, "\n")
	if !strings.Contains(joined, "autoindent") || !strings.Contains(joined, "number") || !strings.Contains(joined, "shiftwidth=4") {
		t.Errorf(":set (changed only) = %q", joined)
	}
	// Unchanged defaults should not appear.
	if strings.Contains(joined, "noautowrite") || strings.Contains(joined, "tabstop=8") {
		t.Errorf(":set should show only changed options, got %q", joined)
	}
}

func TestNewOptionsSettable(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	// Options that don't drive behavior yet must still be settable / queryable.
	for _, cmd := range []string{"set autowrite", "set noautowrite", "set report=10", "set shell=/bin/bash", "set wrapmargin=8", "set ruler"} {
		if err := e.exExecute(cmd); err != nil {
			t.Errorf("%q: %v", cmd, err)
		}
	}
	if e.scr.opts.Int("report") != 10 {
		t.Errorf("report = %d, want 10", e.scr.opts.Int("report"))
	}
	if e.scr.opts.Str("shell") != "/bin/bash" {
		t.Errorf("shell = %q", e.scr.opts.Str("shell"))
	}
	if !e.scr.opts.Bool("ruler") {
		t.Error("ruler not set")
	}
	if err := e.exExecute("set nosuchoption"); err == nil {
		t.Error("unknown option should error")
	}
}

func TestOutputOverlayDismissed(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello\n")
	e.Resize(23, 80)
	drive(e, ":set all\r")
	if e.scr.pendingOutput == nil {
		t.Fatal("expected overlay")
	}
	e.Input(KeyEvent{Rune: ' '}) // any key dismisses
	if e.scr.pendingOutput != nil {
		t.Fatal("overlay not dismissed")
	}
	// The dismiss key is consumed, not treated as a command.
	if bufText(e) != "hello" {
		t.Fatalf("dismiss key leaked into editor: %q", bufText(e))
	}
}
