package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// RunShell opens an interactive shell outside the GUI window. On macOS this
// launches Terminal.app; elsewhere :shell is not available from the GUI host.
func (h *host) RunShell(shell string, _ bool) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("Shell not available in GUI mode")
	}
	escaped := strings.ReplaceAll(shell, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	script := fmt.Sprintf(`tell application "Terminal" to do script "%s -i"`, escaped)
	if err := exec.Command("osascript", "-e", script).Run(); err != nil {
		return fmt.Errorf("Shell not available in GUI mode")
	}
	return nil
}