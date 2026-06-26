package engine

import (
	"fmt"
	"time"
)

const defaultRefresh = 20 * time.Millisecond

// DefaultRefresh is the default for the refresh option (honored by the terminal
// frontend only; inert in GoVi.app).
func DefaultRefresh() time.Duration { return defaultRefresh }

func parseRefresh(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("set: refresh: %v", err)
	}
	if d < 0 {
		return 0, fmt.Errorf("set: refresh: negative duration")
	}
	return d, nil
}

func canonRefresh(s string) (string, error) {
	d, err := parseRefresh(s)
	if err != nil {
		return "", err
	}
	if d == 0 {
		return "0", nil
	}
	return d.String(), nil
}

func formatRefresh(s string) string {
	d, err := parseRefresh(s)
	if err != nil {
		return s
	}
	if d == 0 {
		return "0"
	}
	return d.String()
}
