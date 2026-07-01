package engine

import "testing"

func TestInterruptPlumbing(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello\n")

	if e.Interrupted() {
		t.Fatal("fresh engine reports interrupted")
	}
	select {
	case <-e.InterruptChan():
		t.Fatal("fresh engine has a pending interrupt on the channel")
	default:
	}

	// Interrupt sets both representations.
	e.Interrupt()
	if !e.Interrupted() {
		t.Fatal("Interrupt did not set the flag")
	}

	// A second Interrupt must not block or panic even though nothing has drained
	// the buffered(1) channel yet.
	e.Interrupt()

	// The channel carries exactly one signal (coalesced).
	select {
	case <-e.InterruptChan():
	default:
		t.Fatal("Interrupt did not signal the channel")
	}
	select {
	case <-e.InterruptChan():
		t.Fatal("channel held more than one signal")
	default:
	}

	// The flag survives draining the channel (they are independent views).
	if !e.Interrupted() {
		t.Fatal("flag cleared by draining the channel")
	}

	// clearInterrupt resets both.
	e.clearInterrupt()
	if e.Interrupted() {
		t.Fatal("clearInterrupt left the flag set")
	}
	select {
	case <-e.InterruptChan():
		t.Fatal("clearInterrupt left a signal on the channel")
	default:
	}
}

func TestInterruptClearedAtNextInput(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello\n")
	e.Interrupt()
	if !e.Interrupted() {
		t.Fatal("Interrupt did not set the flag")
	}
	// The next event begins a fresh command and must clear a stale interrupt
	// (nvi CLR_INTERRUPT at the top of the command loop).
	e.Input(KeyEvent{Rune: 'l'})
	if e.Interrupted() {
		t.Fatal("interrupt flag survived into the next command")
	}
}
