//go:build unix

package tcell

import (
	"fmt"
	"os"
	"os/signal"

	"govi/engine"
)

func (f *Frontend) installSignals() chan os.Signal {
	ch := make(chan os.Signal, 8)
	signal.Notify(ch, engine.CatchableSignals...)
	return ch
}

func signalStop(ch chan os.Signal) {
	if ch != nil {
		signal.Stop(ch)
	}
}

// handleSignal processes one trapped signal. It returns true when the editor
// should exit (after restoring the terminal and printing any exit message).
func (f *Frontend) handleSignal(sig os.Signal) (exit bool) {
	if f.eng == nil {
		return true
	}
	info := f.eng.OnSignal(sig)
	switch info.Action {
	case engine.SigActFatal:
		f.shutdown(info.Message)
		return true
	case engine.SigActBackslash:
		if f.eng.ShouldQuit() {
			f.shutdown("")
			return true
		}
	}
	return false
}

// shutdown restores the terminal to cooked mode and prints a signal exit message.
func (f *Frontend) shutdown(msg string) {
	if f.eng != nil {
		if m := f.eng.ExitMessage(); m != "" {
			msg = m
		}
		_ = f.eng.Close()
	}
	f.Close()
	if msg != "" {
		fmt.Fprintln(os.Stderr, msg)
	}
}