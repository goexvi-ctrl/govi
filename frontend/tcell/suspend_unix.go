//go:build unix

package tcell

import (
	"syscall"
)

// Suspend restores the terminal and stops the process (^Z / :suspend).
func (f *Frontend) Suspend() error {
	if f.eng != nil && f.eng.ExActive() {
		syscall.Kill(0, syscall.SIGTSTP)
		return nil
	}
	if err := f.scr.Suspend(); err != nil {
		return err
	}
	syscall.Kill(0, syscall.SIGTSTP)
	if err := f.scr.Resume(); err != nil {
		return err
	}
	f.scr.Sync()
	w, h := f.scr.Size()
	f.eng.Resize(textRows(h), w)
	return nil
}