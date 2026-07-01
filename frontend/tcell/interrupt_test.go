package tcell

import (
	"testing"
	"time"

	tc "github.com/gdamore/tcell/v2"

	"govi/engine"
)

func TestIsInterruptEvent(t *testing.T) {
	if !isInterruptEvent(tc.NewEventKey(tc.KeyCtrlC, 0, tc.ModCtrl)) {
		t.Error("KeyCtrlC not recognized as interrupt")
	}
	if !isInterruptEvent(tc.NewEventInterrupt(nil)) {
		t.Error("EventInterrupt not recognized as interrupt")
	}
	if isInterruptEvent(tc.NewEventKey(tc.KeyRune, 'a', 0)) {
		t.Error("plain 'a' misreported as interrupt")
	}
}

// TestForwardInterruptsOutOfBand proves the forwarder sets the interrupt flag as
// soon as it polls a ^C, even when the main loop is NOT reading the outgoing
// channel (simulating a busy Engine.Input) and a plain key is queued ahead of
// the ^C.
func TestForwardInterruptsOutOfBand(t *testing.T) {
	sim := tc.NewSimulationScreen("")
	fe, err := NewWithScreen(sim)
	if err != nil {
		t.Fatal(err)
	}
	sim.SetSize(40, 10)
	eng := engine.New(fe, engine.Options{})
	fe.Attach(eng)

	raw := make(chan tc.Event)
	out := make(chan tc.Event) // deliberately never read: main loop is "busy"
	go fe.forwardInterrupts(raw, out)

	// A normal key first (it will queue, since out is not drained), then ^C.
	raw <- tc.NewEventKey(tc.KeyRune, 'a', 0)
	raw <- tc.NewEventKey(tc.KeyCtrlC, 0, tc.ModCtrl)

	deadline := time.After(2 * time.Second)
	for !eng.Interrupted() {
		select {
		case <-deadline:
			t.Fatal("interrupt flag not set while out channel was blocked")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	close(raw) // let the goroutine exit; it will close(out)
	select {
	case _, ok := <-out:
		_ = ok // out has a queued event and/or is closed; either is fine
	case <-time.After(time.Second):
	}
}

func TestForwardInterruptsClosesOut(t *testing.T) {
	sim := tc.NewSimulationScreen("")
	fe, err := NewWithScreen(sim)
	if err != nil {
		t.Fatal(err)
	}
	fe.Attach(engine.New(fe, engine.Options{}))

	raw := make(chan tc.Event)
	out := make(chan tc.Event, 4)
	done := make(chan struct{})
	go func() { fe.forwardInterrupts(raw, out); close(done) }()
	close(raw)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("forwardInterrupts did not return after raw closed")
	}
	if _, ok := <-out; ok {
		t.Fatal("out not closed after raw closed")
	}
}
