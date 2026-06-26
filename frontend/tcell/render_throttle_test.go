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
	if renderUrgent(v, engine.ChangeSet{Scrolled: true}) {
		t.Fatal("scroll should not bypass burst throttle")
	}
	if !renderUrgent(v, engine.ChangeSet{ModeChanged: true}) {
		t.Fatal("mode change should be urgent")
	}
	if !renderUrgent(v, engine.ChangeSet{Full: true}) {
		t.Fatal("full repaint should be urgent")
	}
}

func TestPaintDueRespectsRefresh(t *testing.T) {
	sim := tc.NewSimulationScreen("")
	fe, err := NewWithScreen(sim)
	if err != nil {
		t.Fatal(err)
	}
	fe.lastPaintAt = time.Now()

	if !fe.paintDue(0) {
		t.Fatal("zero period should always be due")
	}
	if fe.paintDue(1 * time.Second) {
		t.Fatal("expected paint not due immediately after last paint")
	}
	fe.lastPaintAt = time.Now().Add(-2 * time.Second)
	if !fe.paintDue(1 * time.Second) {
		t.Fatal("expected paint due after interval elapsed")
	}
}

func TestRefreshDisablesThrottle(t *testing.T) {
	eng, _ := setup(t, "x\n", 20, 4)
	eng.RunEx("set refresh=0")
	sim := tc.NewSimulationScreen("")
	fe, err := NewWithScreen(sim)
	if err != nil {
		t.Fatal(err)
	}
	fe.Attach(eng)
	if got := fe.minRenderPeriod(); got != 0 {
		t.Fatalf("minRenderPeriod = %v, want 0", got)
	}
	if !fe.paintDue(fe.minRenderPeriod()) {
		t.Fatal("refresh=0 should always paint when pending")
	}
}

func TestRefreshSetsInterval(t *testing.T) {
	eng, _ := setup(t, "x\n", 20, 4)
	eng.RunEx("set refresh=100ms")
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

func TestRenderDefersDuringBurst(t *testing.T) {
	sim := tc.NewSimulationScreen("")
	fe, err := NewWithScreen(sim)
	if err != nil {
		t.Fatal(err)
	}
	eng, _ := setup(t, "hi\n", 20, 4)
	fe.Attach(eng)
	fe.inEventBurst = true

	var v engine.View
	eng.WithView(func(view engine.View) { v = view })
	fe.Render(v, engine.ChangeSet{Scrolled: true})
	if !fe.paintPending {
		t.Fatal("Render during burst should defer paint")
	}
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