package tcell

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// RunShell suspends the full-screen display and runs an interactive shell, then
// resumes editing when it exits (nvi :shell / ex_shell.c).
func (f *Frontend) RunShell(shell string, inExMode bool) error {
	if !inExMode {
		if err := f.scr.Suspend(); err != nil {
			return err
		}
		defer f.scr.Resume()
		fmt.Fprintln(os.Stdout)
	}

	// nvi runs $shell -c "$shell -i".
	cmd := exec.Command(shell, "-c", shell+" -i")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("%s: exited with status %d", shell, exitErr.ExitCode())
		}
		return err
	}
	return nil
}
