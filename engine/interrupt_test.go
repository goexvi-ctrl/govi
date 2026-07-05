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

// TestCtrlCKeyShowsInterrupted pins nvi's idle-^C behavior (vi/vi.c
// "236|Interrupted", probed live 2026-07-04): a ^C key in vi command mode
// reports Interrupted and discards any partial command. It also keeps the
// message on screen when the ^C that aborted a long operation arrives as a
// trailing key event after the abort.
func TestCtrlCKeyShowsInterrupted(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\n")
	e.Input(KeyEvent{Rune: 'c', Mods: ModCtrl})
	if msg, k := e.scr.msg, e.scr.msgKind; k != MsgError || msg != "Interrupted" {
		t.Fatalf("idle ^C msg = %q/%v, want Interrupted", msg, k)
	}
	// A pending operator is discarded, so the next key starts fresh.
	drive(e, "d")
	e.Input(KeyEvent{Rune: 'c', Mods: ModCtrl})
	drive(e, "x") // would be "dx" (delete-motion) if the d survived
	if got := string(e.scr.lineRunes(1)); got != "ne" {
		t.Fatalf("after d ^C x line = %q, want ne (x alone)", got)
	}
}
