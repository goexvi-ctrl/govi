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

func TestShouldThrottlePaintDuringBurst(t *testing.T) {
	sim := tc.NewSimulationScreen("")
	fe, err := NewWithScreen(sim)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	fe.lastPaintAt = now
	fe.lastEventAt = now

	if !fe.shouldThrottlePaint(now.Add(5 * time.Millisecond)) {
		t.Fatal("expected throttle while events are arriving fast")
	}
	if fe.shouldThrottlePaint(now.Add(burstEventGap + time.Millisecond)) {
		t.Fatal("expected no throttle after burst gap")
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