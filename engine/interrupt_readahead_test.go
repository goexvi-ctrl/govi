package engine

import "testing"

// Regression for the tcell read-ahead race (parity audit item 10).
//
// The terminal frontend records a ^C out of band on a separate goroutine
// (frontend/tcell forwardInterrupts) and reads ahead of the main loop, so a ^C
// typed to abort a command can be set BEFORE the main loop runs the Input() that
// launches that command -- exactly the interleaving
// tcell.TestForwardInterruptsOutOfBand produces (a plain key queued ahead of the
// ^C, the flag set while the main loop is busy). Input() must honor that
// interrupt, not discard it on entry.
func TestInterruptSetBeforeLaunchingInputIsHonored(t *testing.T) {
	e, _, _ := newTestEngine(t, "aaa\naaa\naaa\n")

	// Type the colon line but do not submit it yet (the main loop keeps up).
	e.Input(KeyEvent{Rune: ':'})
	for _, r := range "%s/a/b/g" {
		e.Input(KeyEvent{Rune: r})
	}

	// forwardInterrupts sets the flag out of band, a hair before the main loop
	// dequeues the Enter that submits the command.
	e.Interrupt()

	// Enter launches the substitution. It must observe the ^C and abort.
	e.Input(KeyEvent{Key: KeyEnter})

	if got := string(e.scr.lineRunes(1)); got != "aaa" {
		t.Fatalf("interrupt lost: substitute ran despite a ^C set before Enter (line1=%q, want aaa)", got)
	}
	if e.scr.msg != "Interrupted" {
		t.Fatalf("msg = %q, want Interrupted", e.scr.msg)
	}
}

// A ^C consumed (or simply outlived) by one command must not leak into the next:
// the deferred clear still enforces nvi's CLR_INTERRUPT between commands.
func TestInterruptDoesNotLeakToNextCommand(t *testing.T) {
	e, _, _ := newTestEngine(t, "aaa\naaa\n")

	e.Interrupt()
	e.Input(KeyEvent{Rune: 'l'}) // a non-interruptible command completes and clears it
	if e.Interrupted() {
		t.Fatal("interrupt survived into the next command")
	}

	// A subsequent substitution launched via Input must run to completion; the
	// earlier ^C is gone.
	e.Input(KeyEvent{Rune: ':'})
	for _, r := range "%s/a/b/g" {
		e.Input(KeyEvent{Rune: r})
	}
	e.Input(KeyEvent{Key: KeyEnter})
	if got := string(e.scr.lineRunes(1)); got != "bbb" {
		t.Fatalf("later substitute wrongly aborted (stale interrupt leaked): line1=%q, want bbb", got)
	}
}
