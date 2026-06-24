//go:build unix

package engine

import (
	"bytes"
	"io"
	"os/exec"

	"github.com/creack/pty"
)

// runBangPTY runs cmd in a pty sized to cols×rows and returns combined output.
// A pty makes utilities such as ls and who format for terminal width while
// still allowing the editor to show the full transcript in the overlay.
func runBangPTY(shell, cmd, cwd string, cols, rows int) (string, error) {
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
		return "", err
	}
	defer ptmx.Close()

	var buf bytes.Buffer
	copyDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(&buf, ptmx)
		copyDone <- err
	}()

	waitErr := c.Wait()
	if copyErr := <-copyDone; copyErr != nil && waitErr == nil {
		waitErr = copyErr
	}
	return buf.String(), waitErr
}
