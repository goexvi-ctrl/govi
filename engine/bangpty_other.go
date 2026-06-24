//go:build !unix

package engine

import (
	"os/exec"
	"strings"
)

func runBangPTY(shell, cmd, cwd string, cols, rows int) (string, error) {
	c := exec.Command(shell, "-c", cmd)
	if cwd != "" {
		c.Dir = cwd
	}
	c.Env = shellEnv(cols, rows)
	out, err := c.CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}
