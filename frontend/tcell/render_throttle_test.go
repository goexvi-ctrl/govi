package tcell

import (
	"testing"
	"time"

	tc "github.com/gdamore/tcell/v2"

	"govi/engine"
)

func TestRenderUrgent(t *testing.T) {
	eng, _ := setup(t, "hi\n", 20, 4)
	var v engine.View
	eng.WithView(func(view engine.View) { v = view })

	if renderUrgent(v, engine.ChangeSet{CursorMoved: true}) {
		t.Fatal("cursor-only change should not be urgent")
	}
	if !renderUrgent(v, engine.ChangeSet{ModeChanged: true}) {
		t.Fatal("mode change should be urgent")
	}
	if !renderUrgent(v, engine.ChangeSet{Full: true}) {
		t.Fatal("full repaint should be urgent")
	}
}

func TestShouldPaintNowDuringFlood(t *testing.T) {
	sim := tc.NewSimulationScreen("")
	fe, err := NewWithScreen(sim)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	fe.lastPaintAt = now

	if fe.shouldPaintNow(now.Add(5*time.Millisecond), true) {
		t.Fatal("expected defer while interval not elapsed and input pending")
	}
	if !fe.shouldPaintNow(now.Add(55*time.Millisecond), true) {
		t.Fatal("expected paint once refreshms elapsed even with pending input")
	}
	if !fe.shouldPaintNow(now.Add(5*time.Millisecond), false) {
		t.Fatal("expected paint when input queue is empty")
	}
}

func TestRefreshMsDisablesThrottle(t *testing.T) {
	eng, _ := setup(t, "x\n", 20, 4)
	eng.RunEx("set refreshms=0")
	sim := tc.NewSimulationScreen("")
	fe, err := NewWithScreen(sim)
	if err != nil {
		t.Fatal(err)
	}
	fe.Attach(eng)
	now := time.Now()
	fe.lastPaintAt = now
	if !fe.shouldPaintNow(now.Add(5*time.Millisecond), true) {
		t.Fatal("refreshms=0 should always paint")
	}
}

func TestRefreshMsSetsInterval(t *testing.T) {
	eng, _ := setup(t, "x\n", 20, 4)
	eng.RunEx("set refreshms=100")
	sim := tc.NewSimulationScreen("")
	fe, err := NewWithScreen(sim)
	if err != nil {
		t.Fatal(err)
	}
	fe.Attach(eng)
	if got := fe.minRenderPeriod(); got != 100*time.Millisecond {
		t.Fatalf("minRenderPeriod = %v, want 100ms", got)
	}
}

func TestSchedulePaintDoesNotResetTimer(t *testing.T) {
	sim := tc.NewSimulationScreen("")
	fe, err := NewWithScreen(sim)
	if err != nil {
		t.Fatal(err)
	}
	fe.lastPaintAt = time.Now()
	fe.schedulePaint(50 * time.Millisecond)
	first := fe.paintTimer
	if first == nil {
		t.Fatal("expected timer after first schedule")
	}
	fe.schedulePaint(10 * time.Millisecond)
	if fe.paintTimer != first {
		t.Fatal("second schedulePaint should not replace an armed timer")
	}
	fe.stopPaintTimer()
}

func TestFastInsertCompletesQuickly(t *testing.T) {
	eng, _ := setup(t, "hello\n", 80, 4)
	eng.Input(engine.KeyEvent{Rune: 'i'})
	start := time.Now()
	for i := 0; i < 200; i++ {
		eng.Input(engine.KeyEvent{Rune: 'x'})
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("200 fast inserts took %v; throttle may be ineffective", elapsed)
	}
	var n int
	eng.WithView(func(v engine.View) {
		n = len(v.Line(1).Text)
	})
	if n != len("hello")+200 {
		t.Fatalf("line len = %d, want %d", n, len("hello")+200)
	}
}