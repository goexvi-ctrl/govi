//go:build !unix

package tcell

import "os"

func (f *Frontend) installSignals() chan os.Signal { return nil }

func (f *Frontend) handleSignal(sig os.Signal) (exit bool) { return false }

func signalStop(ch chan os.Signal) {}

func (f *Frontend) shutdown(msg string) {
	if f.eng != nil {
		_ = f.eng.Close()
	}
	f.Close()
}
