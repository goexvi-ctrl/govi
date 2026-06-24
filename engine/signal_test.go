package engine

import (
	"syscall"
	"testing"
)

func TestCtrlBackslashViToEx(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello\n")
	if e.ExActive() {
		t.Fatal("should start in vi mode")
	}
	e.handleCtrlBackslash()
	if !e.ExActive() {
		t.Fatal("Ctrl-\\ should switch to ex mode")
	}
	if e.ShouldQuit() {
		t.Fatal("Ctrl-\\ in vi should not quit")
	}
}

func TestCtrlBackslashExQuits(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello\n")
	e.enterExMode()
	e.handleCtrlBackslash()
	if !e.ShouldQuit() {
		t.Fatal("Ctrl-\\ in ex should quit")
	}
}

func TestQuitCommandDisplay(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.enterExMode()
	e.exLineMode = true
	e.quitFromBackslash()
	if len(e.exOut) != 1 || e.exOut[0] != QuitCommandDisplay {
		t.Fatalf("exOut = %v, want %q", e.exOut, QuitCommandDisplay)
	}
}

func TestExFeedLineBackslash(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.enterExMode()
	out := e.ExFeedLine("\x1c")
	if len(out) != 1 || out[0] != QuitCommandDisplay {
		t.Fatalf("out = %v", out)
	}
	if !e.ShouldQuit() {
		t.Fatal("FS line should quit")
	}
}

func TestOnSignalHangup(t *testing.T) {
	e, _, _ := newTestEngine(t, "data\n")
	drive(e, "x")
	info := e.OnSignal(syscall.SIGHUP)
	if info.Action != SigActFatal || info.Message != "Hangup" {
		t.Fatalf("info = %+v", info)
	}
	if !e.ShouldQuit() {
		t.Fatal("SIGHUP should set quit")
	}
	if e.ExitMessage() != "Hangup" {
		t.Fatalf("ExitMessage = %q", e.ExitMessage())
	}
}

func TestOnSignalInterrupt(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.enterCmdline(':')
	e.OnSignal(syscall.SIGINT)
	if e.scr.mode != ModeCommand {
		t.Fatalf("SIGINT should cancel colon line, mode = %v", e.scr.mode)
	}
	if e.ShouldQuit() {
		t.Fatal("SIGINT should not quit")
	}
}

func TestCatchableSignalsOmitsSIGTSTP(t *testing.T) {
	for _, sig := range CatchableSignals {
		if sig == syscall.SIGTSTP {
			t.Fatal("SIGTSTP must not be trapped; suspend uses kill(2) job control")
		}
	}
}

func TestIsBackslashLine(t *testing.T) {
	if !IsBackslashLine("\x1c") {
		t.Fatal("FS line")
	}
	if IsBackslashLine("quit") {
		t.Fatal("not FS")
	}
}