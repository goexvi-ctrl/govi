package engine

import (
	"fmt"
	"os"
	"syscall"
)

// QuitCommandDisplay is the ex transcript line shown when quitting via ^\.
const QuitCommandDisplay = ":^\\Quit"

// SignalAction tells a host how to respond to an asynchronous signal.
type SignalAction int

const (
	SigActNone SignalAction = iota
	SigActInterrupt
	SigActBackslash // SIGQUIT / ^\: vi -> ex, ex -> quit
	SigActSuspend
	SigActResize // host may already handle geometry; ignore here
	SigActFatal    // preserve and exit; display the signal name
)

// SignalInfo describes a trapped signal.
type SignalInfo struct {
	Action  SignalAction
	Message string // human-readable name, e.g. "Hangup"
}

// CatchableSignals is the set of signals the terminal host should trap.
var CatchableSignals = []os.Signal{
	syscall.SIGHUP,
	syscall.SIGINT,
	syscall.SIGQUIT,
	syscall.SIGTERM,
	syscall.SIGTSTP,
	syscall.SIGWINCH,
	syscall.SIGPIPE,
	syscall.SIGALRM,
	syscall.SIGUSR1,
	syscall.SIGUSR2,
}

// SignalInfoFor maps a signal to the action the engine expects and a display
// name (nvi ex_shell.c sigmsg).
func SignalInfoFor(sig os.Signal) SignalInfo {
	if sig == nil {
		return SignalInfo{}
	}
	switch sig {
	case syscall.SIGHUP:
		return SignalInfo{SigActFatal, "Hangup"}
	case syscall.SIGINT:
		return SignalInfo{SigActInterrupt, "Interrupt"}
	case syscall.SIGQUIT:
		return SignalInfo{SigActBackslash, "Quit"}
	case syscall.SIGTERM:
		return SignalInfo{SigActFatal, "Terminated"}
	case syscall.SIGTSTP:
		return SignalInfo{SigActSuspend, "Suspended"}
	case syscall.SIGWINCH:
		return SignalInfo{SigActResize, ""}
	case syscall.SIGPIPE:
		return SignalInfo{SigActFatal, "Broken pipe"}
	case syscall.SIGALRM:
		return SignalInfo{SigActFatal, "Alarm clock"}
	case syscall.SIGUSR1:
		return SignalInfo{SigActFatal, "User defined signal 1"}
	case syscall.SIGUSR2:
		return SignalInfo{SigActFatal, "User defined signal 2"}
	}
	if s, ok := sig.(syscall.Signal); ok {
		return SignalInfo{SigActFatal, fmt.Sprintf("Signal %d", s)}
	}
	return SignalInfo{SigActFatal, fmt.Sprintf("Signal %v", sig)}
}

// OnSignal handles an asynchronous signal delivered by the host. For fatal
// signals it preserves modified buffers and sets quit; the host should restore
// the terminal and print ExitMessage() before exiting.
func (e *Engine) OnSignal(sig os.Signal) SignalInfo {
	info := SignalInfoFor(sig)
	switch info.Action {
	case SigActInterrupt:
		e.flushMapPending()
		e.interrupt()
	case SigActBackslash:
		e.handleCtrlBackslash()
	case SigActSuspend:
		_ = e.doSuspend(false)
	case SigActFatal:
		e.preserveAndQuit(info.Message)
	case SigActResize, SigActNone:
	}
	return info
}

// ExitMessage returns text to print when the editor is exiting because of a
// signal, or "" when the exit was not signal-driven.
func (e *Engine) ExitMessage() string { return e.exitMsg }

func (e *Engine) preserveAndQuit(msg string) {
	if e.scr != nil && e.scr.modified {
		e.SyncRecovery()
	}
	e.exitMsg = msg
	e.quit = true
}

// handleCtrlBackslash implements nvi's ^\ behavior: in vi (any non-ex mode)
// switch to ex; in ex mode quit (displaying :^\Quit).
func (e *Engine) handleCtrlBackslash() {
	if e.ExActive() {
		e.quitFromBackslash()
		return
	}
	if e.scr.mode == ModeInsert || e.scr.mode == ModeReplace {
		e.vi.finishInsert(e)
	}
	e.enterExMode()
}

// quitFromBackslash quits from ex mode the way ^\ does, echoing :^\Quit.
func (e *Engine) quitFromBackslash() {
	if e.scr.mode == ModeExText {
		e.exEcho(QuitCommandDisplay)
	}
	e.removeRecovery()
	e.quit = true
}

// IsBackslashLine reports whether a cooked-mode ex input line is ^\ (FS).
func IsBackslashLine(line string) bool {
	if line == "" {
		return false
	}
	r := []rune(line)
	for _, c := range r {
		if c != '\x1c' {
			return false
		}
	}
	return len(r) > 0
}