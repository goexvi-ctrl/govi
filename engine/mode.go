package engine

import (
	"fmt"
	"strings"
)

// selectionModes are the canonical selection-mode names (GoVi GUI only).
var selectionModes = []string{"terminal", "gui", "contextual"}

// selectionModeAlias maps input-only aliases to their canonical name.
var selectionModeAlias = map[string]string{
	"passive": "terminal",
	"active":  "gui",
	"hybrid":  "contextual",
}

func selectionModeInputs() []string {
	names := append([]string(nil), selectionModes...)
	for alias := range selectionModeAlias {
		names = append(names, alias)
	}
	return names
}

func canonSelectionMode(val string) (string, error) {
	lower := strings.ToLower(strings.TrimSpace(val))
	if lower == "" {
		return "", fmt.Errorf("set: mode: empty value")
	}
	var found string
	for _, name := range selectionModeInputs() {
		if !strings.HasPrefix(name, lower) {
			continue
		}
		canon := name
		if c, ok := selectionModeAlias[name]; ok {
			canon = c
		}
		if found != "" && found != canon {
			return "", fmt.Errorf("set: mode: ambiguous value %q", val)
		}
		found = canon
	}
	if found == "" {
		return "", fmt.Errorf("set: mode: must be terminal, gui, or contextual")
	}
	return found, nil
}

func formatSelectionMode(val string) string {
	if canon, err := canonSelectionMode(val); err == nil {
		return canon
	}
	return val
}
