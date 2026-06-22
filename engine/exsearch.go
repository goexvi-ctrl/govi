package engine

import (
	"fmt"
	"os"
	"strings"
)

// Regex-dependent ex commands. The vi-compatible regex engine and these
// implementations land in Phase 5; until then they report that they are not yet
// available rather than silently doing nothing.

func (e *Engine) exSubstitute(c *exCmd) error {
	return fmt.Errorf("substitute: not yet implemented (Phase 5)")
}

func (e *Engine) exGlobal(c *exCmd) error {
	return fmt.Errorf("global: not yet implemented (Phase 5)")
}

func (e *Engine) exVglobal(c *exCmd) error {
	return fmt.Errorf("vglobal: not yet implemented (Phase 5)")
}

// readFileLines reads path and splits it into lines, dropping a single trailing
// newline (matching how files load into the buffer).
func readFileLines(path string) ([][]rune, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := string(b)
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, "\n")
	if strings.HasSuffix(s, "\n") {
		parts = parts[:len(parts)-1]
	}
	out := make([][]rune, len(parts))
	for i, p := range parts {
		out[i] = []rune(p)
	}
	return out, nil
}
