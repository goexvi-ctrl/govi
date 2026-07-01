package engine

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
// clearInterrupt resets the state at the start of an interruptible operation
// (nvi's CLR_INTERRUPT), so a stale ^C cannot abort the next one.

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

// clearInterrupt resets both interrupt representations. Call it just before
// beginning an interruptible operation so a ^C that predates the operation does
// not cancel it.
func (e *Engine) clearInterrupt() {
	e.interrupted.Store(false)
	select {
	case <-e.interruptCh:
	default:
	}
}
