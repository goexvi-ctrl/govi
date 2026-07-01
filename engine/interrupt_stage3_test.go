//go:build unix

package engine

import (
	"errors"
	"os"
	"testing"
	"time"
)

// Stage 3: a blocking external command must abort promptly on ^C rather than
// run to completion. runShellCmd starts the process and selects on the interrupt
// channel; an interrupt kills the child and returns errInterrupted at once.

func TestInterruptAbortsBlockingShellCmd(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")

	type res struct {
		out string
		err error
	}
	done := make(chan res, 1)
	start := time.Now()
	go func() {
		out, err := e.runShellCmd("sleep 10", "", 80, 24)
		done <- res{out, err}
	}()

	// Let the child start, then interrupt as the frontend forwarder would.
	time.Sleep(150 * time.Millisecond)
	e.Interrupt()

	select {
	case r := <-done:
		if !errors.Is(r.err, errInterrupted) {
			t.Fatalf("err = %v, want errInterrupted", r.err)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("interrupt took %v; the sleep was not killed", elapsed)
		}
	case <-time.After(9 * time.Second):
		t.Fatal("runShellCmd did not return after interrupt (sleep ran to completion?)")
	}
}

func TestInterruptAbortsWrite(t *testing.T) {
	e, _, path := newTestEngine(t, "original\n")
	drive(e, "ohello\x1b") // modify the buffer (open a line)
	e.Interrupt()
	err := e.exExecute("w")
	if !errors.Is(err, errInterrupted) {
		t.Fatalf("write err = %v, want errInterrupted", err)
	}
	// The on-disk file must be untouched, and the buffer still modified.
	if data, _ := os.ReadFile(path); string(data) != "original\n" {
		t.Fatalf("file changed despite interrupted write: %q", string(data))
	}
	if !e.scr.modified {
		t.Fatal("buffer should still be modified after an interrupted write")
	}
}

func TestBlockingShellCmdRunsWithoutInterrupt(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	out, err := e.runShellCmd("printf hello", "", 80, 24)
	if err != nil {
		t.Fatalf("runShellCmd: %v", err)
	}
	if out != "hello" {
		t.Fatalf("out = %q, want %q", out, "hello")
	}
}
