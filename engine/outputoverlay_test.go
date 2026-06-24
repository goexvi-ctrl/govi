package engine

import "testing"

func TestPendingOutputPagination(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.Resize(3, 40) // 3 text rows per page
	if err := e.exExecute("exusage"); err != nil {
		t.Fatal(err)
	}
	if e.scr.pendingOutput == nil {
		t.Fatal("expected overlay")
	}
	if !e.scr.pendingHasMorePages() {
		t.Fatal("exusage should span multiple pages")
	}
	v := view{e.scr}
	if got := v.PendingOutputPrompt(); got != promptMorePages {
		t.Fatalf("page0 prompt = %q", got)
	}
	if len(v.PendingOutput()) != 3 {
		t.Fatalf("page0 lines = %d, want 3", len(v.PendingOutput()))
	}

	e.Input(KeyEvent{Rune: ' '})
	if e.scr.pendingOutput == nil {
		t.Fatal("overlay dismissed after first page")
	}
	if e.scr.pendingPage != 1 {
		t.Fatalf("pendingPage = %d, want 1", e.scr.pendingPage)
	}

	// Advance to final page.
	for e.scr.pendingHasMorePages() {
		e.Input(KeyEvent{Rune: ' '})
	}
	if got := v.PendingOutputPrompt(); got != promptLastPage {
		t.Fatalf("final page prompt = %q", got)
	}

	e.Input(KeyEvent{Rune: ' '})
	if e.scr.pendingOutput != nil {
		t.Fatal("overlay should be dismissed on final page")
	}
}

func TestPendingOutputQuit(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.Resize(3, 40)
	if err := e.exExecute("viusage"); err != nil {
		t.Fatal(err)
	}
	e.Input(KeyEvent{Rune: 'q'})
	if e.scr.pendingOutput != nil {
		t.Fatal("q should dismiss overlay")
	}
}

func TestPendingOutputColonOnLastPage(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.Resize(3, 40)
	if err := e.exExecute("help"); err != nil {
		t.Fatal(err)
	}
	// help fits one page; colon on dismiss opens ex line.
	for e.scr.pendingOutput != nil {
		if e.scr.pendingHasMorePages() {
			e.Input(KeyEvent{Rune: ' '})
		} else {
			e.Input(KeyEvent{Rune: ':'})
			break
		}
	}
	if e.scr.mode != ModeExColon {
		t.Fatalf("mode = %v, want colon", e.scr.mode)
	}
}