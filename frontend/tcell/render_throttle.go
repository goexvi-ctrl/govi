package tcell

import (
	"time"

	tc "github.com/gdamore/tcell/v2"

	"govi/engine"
)

// During fast input (paste or autorepeat) coalesce repaints in the event loop.
// Semantic handling stays per-key. The minimum interval comes from the refreshms
// option (default 50ms ≈ 20 Hz); refreshms=0 disables throttling.
//
// Repaint when elapsed >= refreshms (even mid-flood), when the event queue is
// empty, or once at the end of a burst. A single timer is armed for the
// remaining interval and is not reset on every key.
const burstRelayWait = 3 * time.Millisecond

func (f *Frontend) minRenderPeriod() time.Duration {
	if f.eng == nil {
		return 50 * time.Millisecond
	}
	ms := f.eng.RefreshMs()
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

func renderUrgent(v engine.View, cs engine.ChangeSet) bool {
	if cs.Full || cs.ModeChanged || cs.Scrolled || cs.MessageChanged {
		return true
	}
	if v.PendingOutput() != nil {
		return true
	}
	return false
}

// shouldPaintNow reports whether a non-urgent repaint should happen now.
// pending is true when tcell still has parsed events queued.
func (f *Frontend) shouldPaintNow(now time.Time, pending bool) bool {
	period := f.minRenderPeriod()
	if period <= 0 {
		return true
	}
	f.paintMu.Lock()
	elapsed := now.Sub(f.lastPaintAt)
	f.paintMu.Unlock()
	if elapsed >= period {
		return true
	}
	return !pending
}

func (f *Frontend) schedulePaint(wait time.Duration) {
	if wait < 0 {
		wait = 0
	}
	f.paintMu.Lock()
	defer f.paintMu.Unlock()
	if f.paintTimer != nil {
		return
	}
	f.paintDeferred = true
	f.paintTimer = time.AfterFunc(wait, func() { f.flushDeferredPaint() })
}

func (f *Frontend) stopPaintTimer() {
	f.paintMu.Lock()
	defer f.paintMu.Unlock()
	if f.paintTimer != nil {
		f.paintTimer.Stop()
		f.paintTimer = nil
	}
	f.paintDeferred = false
}

func (f *Frontend) markPainted() {
	f.paintMu.Lock()
	f.lastPaintAt = time.Now()
	f.paintDeferred = false
	f.paintMu.Unlock()
}

func (f *Frontend) ensurePainted() {
	f.stopPaintTimer()
	f.eng.WithView(func(v engine.View) {
		f.paintNow(v)
	})
	f.markPainted()
}

func (f *Frontend) flushDeferredPaint() {
	f.paintMu.Lock()
	deferred := f.paintDeferred
	f.paintDeferred = false
	if f.paintTimer != nil {
		f.paintTimer.Stop()
		f.paintTimer = nil
	}
	f.paintMu.Unlock()
	if !deferred {
		return
	}
	f.eng.WithView(func(v engine.View) {
		f.paintNow(v)
	})
	f.markPainted()
}

// processEvents handles one or more tcell events from a burst, then ensures the
// screen reflects the final state once the queue has gone quiet. closed is true
// when the events channel has shut down.
func (f *Frontend) processEvents(events <-chan tc.Event, first tc.Event) (closed bool) {
	if first == nil {
		return true
	}
	f.handleEvent(first)
	for {
		select {
		case ev := <-events:
			if ev == nil {
				f.ensurePainted()
				return true
			}
			f.handleEvent(ev)
		default:
			if f.scr.HasPendingEvent() {
				select {
				case ev := <-events:
					if ev == nil {
						f.ensurePainted()
						return true
					}
					f.handleEvent(ev)
					continue
				case <-time.After(burstRelayWait):
				}
			}
			f.ensurePainted()
			return false
		}
	}
}