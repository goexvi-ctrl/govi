package tcell

import (
	"time"

	tc "github.com/gdamore/tcell/v2"

	"govi/engine"
)

// During fast input (paste or autorepeat) repaint at most maxRenderHz and
// coalesce bursts in the event loop. Semantic handling stays per-key.
const (
	burstEventGap   = 15 * time.Millisecond
	minRenderPeriod = time.Second / 20 // 20 Hz while input is arriving fast
	burstRelayWait  = 3 * time.Millisecond
)

func renderUrgent(v engine.View, cs engine.ChangeSet) bool {
	if cs.Full || cs.ModeChanged || cs.Scrolled || cs.MessageChanged {
		return true
	}
	if v.PendingOutput() != nil {
		return true
	}
	return false
}

func (f *Frontend) noteInputEvent() {
	f.paintMu.Lock()
	f.lastEventAt = time.Now()
	f.paintMu.Unlock()
}

func (f *Frontend) shouldThrottlePaint(now time.Time) bool {
	if f.scr.HasPendingEvent() {
		return true
	}
	f.paintMu.Lock()
	defer f.paintMu.Unlock()
	if f.lastEventAt.IsZero() {
		return false
	}
	if now.Sub(f.lastEventAt) >= burstEventGap {
		return false
	}
	return now.Sub(f.lastPaintAt) < minRenderPeriod
}

func (f *Frontend) deferPaint(now time.Time) {
	f.paintMu.Lock()
	defer f.paintMu.Unlock()
	f.paintDeferred = true
	wait := minRenderPeriod - now.Sub(f.lastPaintAt)
	if wait < 0 {
		wait = 0
	}
	if f.paintTimer != nil {
		f.paintTimer.Stop()
	}
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

// processEvents handles one or more tcell events from a burst, then ensures any
// deferred repaint is flushed once the queue has gone quiet. closed is true when
// the events channel has shut down.
func (f *Frontend) processEvents(events <-chan tc.Event, first tc.Event) (closed bool) {
	if first == nil {
		return true
	}
	f.handleEvent(first)
	for {
		select {
		case ev := <-events:
			if ev == nil {
				f.flushDeferredPaint()
				return true
			}
			f.handleEvent(ev)
		default:
			if f.scr.HasPendingEvent() {
				select {
				case ev := <-events:
					if ev == nil {
						f.flushDeferredPaint()
						return true
					}
					f.handleEvent(ev)
					continue
				case <-time.After(burstRelayWait):
				}
			}
			f.flushDeferredPaint()
			return false
		}
	}
}