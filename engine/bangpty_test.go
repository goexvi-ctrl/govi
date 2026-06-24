//go:build unix

package engine

import (
	"strings"
	"testing"
)

func TestRunBangPTY(t *testing.T) {
	out, err := runBangPTY("/bin/sh", "printf 'line1\\nline2\\n'", t.TempDir(), 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "line1") || !strings.Contains(out, "line2") {
		t.Fatalf("output = %q", out)
	}
}

func TestPresentBangOutputMultiline(t *testing.T) {
	fe := &captureFrontend{}
	e := New(fe, Options{})
	e.Resize(10, 40)
	e.presentBangOutput("one\ntwo\nthree")
	if len(e.scr.pendingOutput) != 3 {
		t.Fatalf("pendingOutput = %v", e.scr.pendingOutput)
	}
	if got := e.scr.pendingOutputPrompt(); got != promptLastPage {
		t.Fatalf("prompt = %q", got)
	}
}
