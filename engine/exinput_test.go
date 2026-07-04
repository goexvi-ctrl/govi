package engine

import "testing"

func TestExAppend(t *testing.T) {
	e, _, _ := newTestEngine(t, "a\nc\n")
	drive(e, "Q")      // ex mode
	drive(e, "1a\r")   // append after line 1
	drive(e, "X\rY\r") // two input lines
	drive(e, ".\r")    // terminator
	if got := bufText(e); got != "a\nX\nY\nc" {
		t.Fatalf("ex append: %q", got)
	}
}

func TestExInsert(t *testing.T) {
	e, _, _ := newTestEngine(t, "a\nc\n")
	drive(e, "Q2i\rB\r.\r")
	if got := bufText(e); got != "a\nB\nc" {
		t.Fatalf("ex insert: %q", got)
	}
}

func TestExChange(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\ntwo\nthree\n")
	drive(e, "Q2c\rNEW\r.\r")
	if got := bufText(e); got != "one\nNEW\nthree" {
		t.Fatalf("ex change: %q", got)
	}
}

func TestExChangeRange(t *testing.T) {
	e, _, _ := newTestEngine(t, "a\nb\nc\nd\n")
	drive(e, "Q2,3c\rX\r.\r")
	if got := bufText(e); got != "a\nX\nd" {
		t.Fatalf("ex change range: %q", got)
	}
}

func TestExAppendFromColon(t *testing.T) {
	// :a from vi command mode also collects input.
	e, _, _ := newTestEngine(t, "a\nc\n")
	drive(e, ":1a\r")
	drive(e, "Z\r.\r")
	if got := bufText(e); got != "a\nZ\nc" {
		t.Fatalf("colon append: %q", got)
	}
}

// TestExAutoprint covers CORNERS A-3: in ex (Q) mode, the E_AUTOPRINT commands
// (delete, move, copy/t, join, put, <, >, undo) echo the new current line when
// the autoprint option is set. Verified against nvi -e (run under a pty).
func TestExAutoprint(t *testing.T) {
	last := func(out []string) string {
		if len(out) == 0 {
			return ""
		}
		return out[len(out)-1]
	}
	enterEx := func(content string) *Engine {
		e, _, _ := newTestEngine(t, content)
		e.EnterEx()
		e.TakeMessage() // drain the startup file-load message before feeding
		return e
	}
	cases := []struct {
		name string
		feed []string // ex command lines; the last one's output is checked
		want string   // "" means: only require that autoprint fired (non-empty)
	}{
		{"delete", []string{"2d"}, "ccc"},
		{"move", []string{"4m1"}, "ddd"},
		{"copy", []string{"1t3"}, "aaa"},
		{"join", []string{"2,3j"}, "bbb ccc"},
		{"put", []string{"2y", "4put"}, "bbb"},
		{"shiftr", []string{"set sw=8 ts=8", "2>"}, "\tbbb"}, // one tab, shown as 8 spaces
		{"undo", []string{"2d", "u"}, ""},                    // undo is E_AUTOPRINT; line depends on its cursor
	}
	for _, c := range cases {
		e := enterEx("aaa\nbbb\nccc\nddd\neee\n")
		var out []string
		for _, line := range c.feed {
			out = e.ExFeedLine(line)
		}
		got := last(out)
		if c.want == "" {
			if len(out) == 0 {
				t.Errorf("%s: expected autoprint output, got none", c.name)
			}
		} else if got != c.want {
			t.Errorf("%s: autoprint = %q, want %q", c.name, got, c.want)
		}
	}
	// noautoprint suppresses it.
	e := enterEx("aaa\nbbb\nccc\n")
	e.ExFeedLine("set noautoprint")
	if out := e.ExFeedLine("2d"); len(out) != 0 {
		t.Errorf("noautoprint: got output %q, want none", out)
	}
	// A :global body command must not autoprint each line.
	e2 := enterEx("x1\ny\nx2\nz\n")
	if out := e2.ExFeedLine("g/x/d"); len(out) != 0 {
		t.Errorf("global delete: got autoprint %q, want none", out)
	}
}

func TestExLineMode(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\ntwo\nthree\n")
	drive(e, "Q")
	if !e.ExActive() {
		t.Fatal("Q should activate ex mode")
	}
	if e.ExPrompt() != ":" {
		t.Fatalf("prompt = %q, want :", e.ExPrompt())
	}
	// Print lines 1,2: output returned, not stored in a screen transcript.
	out := e.ExFeedLine("1,2p")
	if len(out) != 2 || out[0] != "one" || out[1] != "two" {
		t.Fatalf("print output = %v", out)
	}
	// a/i/c input: prompt disappears while collecting.
	e.ExFeedLine("2a")
	if e.ExPrompt() != "" {
		t.Fatalf("prompt during input = %q, want empty", e.ExPrompt())
	}
	e.ExFeedLine("INS")
	e.ExFeedLine(".")
	if e.ExPrompt() != ":" {
		t.Fatal("prompt should return after .")
	}
	if bufText(e) != "one\ntwo\nINS\nthree" {
		t.Fatalf("after ex append: %q", bufText(e))
	}
	// visual leaves ex mode.
	e.ExFeedLine("visual")
	if e.ExActive() {
		t.Fatal("visual should leave ex mode")
	}
}

func TestExBareAddressPrints(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\ntwo\nthree\n")
	drive(e, "Q")
	// A bare address prints that line and makes it current.
	out := e.ExFeedLine("1")
	if len(out) != 1 || out[0] != "one" {
		t.Fatalf("bare addr print = %v", out)
	}
	if e.scr.cursor.Line != 1 {
		t.Fatalf("current line = %d, want 1", e.scr.cursor.Line)
	}
	// A bare <enter> steps to the next line and prints it.
	if out := e.ExFeedLine(""); len(out) != 1 || out[0] != "two" {
		t.Fatalf("enter step 1 = %v", out)
	}
	if out := e.ExFeedLine(""); len(out) != 1 || out[0] != "three" {
		t.Fatalf("enter step 2 = %v", out)
	}
	// At end-of-file, another <enter> errors instead of advancing.
	if out := e.ExFeedLine(""); len(out) != 1 || out[0] != "at end-of-file" {
		t.Fatalf("enter at EOF = %v", out)
	}
}

func TestExColonBareAddressMoves(t *testing.T) {
	// From the vi colon line (not ex mode), a bare address still just moves.
	e, _, _ := newTestEngine(t, "one\ntwo\nthree\n")
	drive(e, ":3\r")
	if e.scr.cursor.Line != 3 {
		t.Fatalf("colon :3 moved to %d, want 3", e.scr.cursor.Line)
	}
}

func TestExStepTranscriptNoColons(t *testing.T) {
	// The GUI (event/transcript) path: stepping with <enter> must not insert ":"
	// lines between the printed lines (nvi overwrites the prompt with the line).
	e, _, _ := newTestEngine(t, "Line 1\nLine 2\nLine 3\n")
	drive(e, "Q1\r\r\r")
	tr := (view{e.scr}).ExTranscript()
	want := []string{":1", "Line 1", "Line 2", "Line 3"}
	if len(tr) != len(want) {
		t.Fatalf("transcript = %v, want %v", tr, want)
	}
	for i := range want {
		if tr[i] != want[i] {
			t.Fatalf("transcript[%d] = %q, want %q (full %v)", i, tr[i], want[i], tr)
		}
	}
	// One more <enter> is at EOF: the prompt stays, message below.
	drive(e, "\r")
	tr = (view{e.scr}).ExTranscript()
	if tr[len(tr)-2] != ":" || tr[len(tr)-1] != "at end-of-file" {
		t.Fatalf("EOF tail = %v", tr[len(tr)-2:])
	}
}
