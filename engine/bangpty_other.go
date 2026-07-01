//go:build !unix

package engine

import (
	"bytes"
	"os/exec"
	"strings"
)

func (e *Engine) runBangPTY(shell, cmd, cwd string, cols, rows int) (string, error) {
	c := exec.Command(shell, "-c", cmd)
	if cwd != "" {
		c.Dir = cwd
	}
	c.Env = shellEnv(cols, rows)
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	if err := c.Start(); err != nil {
		return "", err
	}
	return e.awaitCmd(c, func() (string, error) {
		err := c.Wait()
		return strings.TrimRight(buf.String(), "\n"), err
	})
}
