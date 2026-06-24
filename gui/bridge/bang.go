package main

import (
	"io"
	"os/exec"

	"github.com/creack/pty"
)

// RunBang runs :!cmd in a pty sized to the editor window so utilities such as
// ls produce multi-column output; output is captured for the overlay.
func (h *host) RunBang(shell, cmd, cwd string, cols, rows int) (string, bool, error) {
	if cols < 1 {
		cols = 80
	}
	if rows < 1 {
		rows = 24
	}
	c := exec.Command(shell, "-c", cmd)
	if cwd != "" {
		c.Dir = cwd
	}
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
	if err != nil {
		return "", false, err
	}
	out, readErr := io.ReadAll(ptmx)
	ptmx.Close()
	waitErr := c.Wait()
	if readErr != nil {
		return "", false, readErr
	}
	if waitErr != nil {
		return string(out), false, waitErr
	}
	return string(out), false, nil
}