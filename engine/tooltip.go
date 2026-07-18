package engine

import (
	"fmt"
	"strings"
)

// Tooltip options (GoVi GUI only). The tooltip option selects how GoVi.app
// shows tooltips for words defined in the tooltipfile: off (never), hover
// (after the mouse rests on a known word for tooltipdelay milliseconds), or
// manual (only on an explicit request: Command-click or the context menu).
// Like the selection-mode option these are carried by the engine so an .exrc
// :set configures them, but they drive no engine behavior -- terminal govi
// accepts and ignores them.

// tooltipModes are the canonical tooltip-mode names.
var tooltipModes = []string{"off", "hover", "manual"}

// canonTooltipMode resolves a case-insensitive unique prefix of a tooltip
// mode to its canonical name.
func canonTooltipMode(val string) (string, error) {
	lower := strings.ToLower(strings.TrimSpace(val))
	if lower == "" {
		return "", fmt.Errorf("set: tooltip: empty value")
	}
	var found string
	for _, name := range tooltipModes {
		if !strings.HasPrefix(name, lower) {
			continue
		}
		if found != "" && found != name {
			return "", fmt.Errorf("set: tooltip: ambiguous value %q", val)
		}
		found = name
	}
	if found == "" {
		return "", fmt.Errorf("set: tooltip: must be off, hover, or manual")
	}
	return found, nil
}

func formatTooltipMode(val string) string {
	if canon, err := canonTooltipMode(val); err == nil {
		return canon
	}
	return val
}
