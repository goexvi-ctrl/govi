package engine

import (
	"errors"
	"os/exec"
)

// errInterrupted is returned by an interruptible operation (search, :g/:v, :s)
// when the user pressed ^C part-way through. It surfaces as nvi's "Interrupted"
// message; any partial results already applied (substitutions made, global
// commands run) are kept, matching nvi.
var errInterrupted = errors.New("Interrupted")

// Cooperative interrupt (^C / SIGINT) support.
//
// The engine runs single-threaded: a host feeds events through Engine.Input on
// one goroutine and the engine never mutates state concurrently. That makes an
// in-progress command uninterruptible through the normal event path -- the
// goroutine that would deliver the ^C is the very one blocked inside the command.
//
// Interrupt is the single, deliberate exception to the "a Frontend must never
// call back into the Engine concurrently" rule. A frontend detects ^C out of
// band (on its input goroutine, while Input is busy) and calls Interrupt; the
// engine then decides *when* to look:
//
//   - CPU-bound loops (search, :g/:v, :s) poll Interrupted() and abort.
//   - Blocking operations run the blocking work on a helper goroutine that
//     reports on a buffered channel, and select between that result and
//     InterruptChan(), so a ^C wakes them the instant it arrives.
//
// clearInterrupt resets the state when a command finishes (deferred at the top
// of Engine.Input, nvi's CLR_INTERRUPT), so a stale ^C cannot abort the next
// command. It is deliberately not cleared on entry: the frontend can set the
// flag out of band ahead of the Input that launches the command it aborts, and
// clearing on entry would swallow that ^C.

// Interrupt records that the user requested an interrupt. It is safe to call
// concurrently with the goroutine driving Input -- indeed that is the point.
// It sets an atomic flag (for pollers) and, without blocking, signals the
// buffered channel (for selectors).
func (e *Engine) Interrupt() {
	e.interrupted.Store(true)
	select {
	case e.interruptCh <- struct{}{}:
	default: // already signalled; the flag is enough
	}
}

// Interrupted reports whether an interrupt has been requested since the last
// clearInterrupt. CPU-bound loops poll this between units of work.
func (e *Engine) Interrupted() bool { return e.interrupted.Load() }

// InterruptChan returns the channel a blocking operation selects on to abort
// promptly when a ^C arrives. It never carries a value; a receive means
// "interrupt requested".
func (e *Engine) InterruptChan() <-chan struct{} { return e.interruptCh }

// awaitCmd waits for an already-started command c, collecting its result with
// finish (which blocks until the output is ready and returns it), unless the
// user presses ^C first. On interrupt it kills the process and returns
// errInterrupted at once, without waiting for finish: the abandoned goroutine's
// result lands in the buffered(1) channel with no reader and is discarded, and
// killing the process unblocks it so nothing lingers. This is how govi improves
// on nvi for blocking work -- the ^C wakes the select immediately rather than
// waiting for a between-operations poll.
//
// c must already be started (so c.Process is non-nil for the kill). finish is
// run on the helper goroutine, so it may read output buffers the command fills.
func (e *Engine) awaitCmd(c *exec.Cmd, finish func() (string, error)) (string, error) {
	type result struct {
		out string
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := finish()
		done <- result{out, err}
	}()
	select {
	case r := <-done:
		return r.out, r.err
	case <-e.interruptCh:
		if c.Process != nil {
			_ = c.Process.Kill()
		}
		return "", errInterrupted
	}
}

// clearInterrupt resets both interrupt representations. It is deferred at the
// top of Engine.Input so the reset happens when the command finishes: this
// command's interruptible loops observe a ^C set (possibly out of band, ahead of
// this Input) before it is dropped, and the drop keeps it from leaking into the
// next command.
func (e *Engine) clearInterrupt() {
	e.interrupted.Store(false)
	select {
	case <-e.interruptCh:
	default:
	}
}
