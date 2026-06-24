package engine

import (
	"fmt"
	"os"
	"strings"
)

const maxExrcSize = 1 << 20 // 1 MiB, matching nvi ex_source.c

func (e *Engine) runStartupScript(name, text string) error {
	e.startup = true
	defer func() { e.startup = false }()

	lines := strings.Split(text, "\n")
	for i, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "\"") {
			continue
		}
		if err := e.exExecute(line); err != nil {
			fmt.Fprintf(os.Stderr, "nvi: %s:%d: %v\n", name, i+1, err)
		}
		if e.quit {
			return nil
		}
	}
	return nil
}

func (e *Engine) exSource(c *exCmd) error {
	path := strings.TrimSpace(c.arg)
	if path == "" {
		return fmt.Errorf("Usage: source file")
	}
	data, err := os.ReadFile(e.resolvePath(path))
	if err != nil {
		return err
	}
	if len(data) > maxExrcSize {
		return fmt.Errorf("File too large")
	}
	return e.runStartupScript(path, string(data))
}