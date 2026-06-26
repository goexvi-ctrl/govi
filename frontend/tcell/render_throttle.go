package tcell

import (
	"time"

	tc "github.com/gdamore/tcell/v2"

	"govi/engine"
)

// During fast input (paste or autorepeat) coalesce repaints in processEvents on
// the main goroutine. Semantic handling stays per-key. The refresh option sets
// the minimum interval between paints during a burst; refresh=0 repaints after
// every event.
const burstRelayWait = 3 * time.Millisecond

func (f *Frontend) minRenderPeriod() time.Duration {
	if f.eng == nil {
		return engine.DefaultRefresh()
	}
	return f.eng.RefreshInterval()
}

func renderUrgent(v engine.View, cs engine.ChangeSet) bool {
	if cs.Full || cs.ModeChanged || cs.MessageChanged {
		return true
	}
	if v.PendingOutput() != nil {
		return true
	}
	return false
}

func (f *Frontend) paintDue(period time.Duration) bool {
	if period <= 0 {
		return true
	}
	return time.Since(f.lastPaintAt) >= period
}

func (f *Frontend) paintWait(period time.Duration) time.Duration {
	if period <= 0 {
		return 0
	}
	wait := period - time.Since(f.lastPaintAt)
	if wait < 0 {
		return 0
	}
	return wait
}

func (f *Frontend) markPainted() {
	f.lastPaintAt = time.Now()
	f.paintPending = false
	f.paintUrgent = false
}

func (f *Frontend) ensurePainted() {
	f.eng.WithView(func(v engine.View) {
		f.paintNow(v)
	})
	f.markPainted()
}

// processEvents handles one or more tcell events from a burst. Repaints are
// driven here (not from background timers) so tcell stays on one goroutine.
// closed is true when the events channel has shut down.
func (f *Frontend) processEvents(events <-chan tc.Event, first tc.Event) (closed bool) {
	if first == nil {
		return true
	}

	f.inEventBurst = true
	defer func() {
		f.inEventBurst = false
		if f.paintPending {
			f.ensurePainted()
		}
	}()

	period := f.minRenderPeriod()

	handle := func(ev tc.Event) {
		f.handleEvent(ev)
		f.paintPending = true
	}

	handle(first)

	for {
		if f.paintPending && (f.paintUrgent || f.paintDue(period)) {
			f.ensurePainted()
		}

		if f.paintPending && period > 0 && !f.scr.HasPendingEvent() {
			select {
			case ev := <-events:
				if ev == nil {
					return true
				}
				handle(ev)
				continue
			default:
				f.ensurePainted()
				return false
			}
		}

		if f.paintPending && period > 0 {
			wait := f.paintWait(period)
			select {
			case ev := <-events:
				if ev == nil {
					return true
				}
				handle(ev)
				continue
			case <-time.After(wait):
				continue
			}
		}

		select {
		case ev := <-events:
			if ev == nil {
				return true
			}
			handle(ev)
		default:
			if f.scr.HasPendingEvent() {
				select {
				case ev := <-events:
					if ev == nil {
						return true
					}
					handle(ev)
				case <-time.After(burstRelayWait):
					if !f.scr.HasPendingEvent() {
						return false
					}
				}
				continue
			}
			return false
		}
	}
}
