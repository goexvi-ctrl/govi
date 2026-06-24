package tcell

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// RunBang runs :!cmd on the real terminal (nvi ex_exec_proc). The editor screen
// is suspended so the utility inherits stdin/stdout/stderr and formats output
// for a tty (e.g. multi-column ls).
func (f *Frontend) RunBang(shell, cmd, cwd string, _, _ int) (string, bool, error) {
	if err := f.scr.Suspend(); err != nil {
		return "", false, err
	}
	defer f.scr.Resume()
	fmt.Fprintln(os.Stdout)

	c := exec.Command(shell, "-c", cmd)
	if cwd != "" {
		c.Dir = cwd
	}
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", true, fmt.Errorf("exited with status %d", exitErr.ExitCode())
		}
		return "", true, err
	}
	return "", true, nil
}