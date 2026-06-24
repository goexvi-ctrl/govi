package engine

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LaunchContext carries startup information from a command-line launcher when
// the editor host does not inherit the shell's cwd or environment (e.g. macOS
// Govi.app opened via the open(1) command).
type LaunchContext struct {
	Cwd      string // invocation directory for ./.nexrc / ./.exrc
	Silent   bool   // -s: skip all startup
	Nexinit  string // if set, used instead of NEXINIT
	Exinit   string // if set (and Nexinit empty), used instead of EXINIT
	SysExrc  string // explicit /etc/vi.exrc path from launcher
	HomeExrc string // explicit $HOME/.nexrc or .exrc path from launcher
}

// LaunchContextDir returns the directory where the govi launcher writes
// launch-context and companion files.
func LaunchContextDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "Govi"), nil
}

// ReadLaunchContext reads the launcher's startup context. Missing files yield a
// zero LaunchContext and no error.
func ReadLaunchContext() (LaunchContext, error) {
	dir, err := LaunchContextDir()
	if err != nil {
		return LaunchContext{}, err
	}
	path := filepath.Join(dir, "launch-context")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return LaunchContext{}, nil
		}
		return LaunchContext{}, err
	}
	defer f.Close()

	var ctx LaunchContext
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "cwd":
			ctx.Cwd = val
		case "silent":
			ctx.Silent = val == "1" || strings.EqualFold(val, "true")
		case "sys_exrc":
			ctx.SysExrc = val
		case "home_exrc":
			ctx.HomeExrc = val
		case "has_nexinit":
			if val == "1" {
				data, err := os.ReadFile(filepath.Join(dir, "nexinit"))
				if err != nil && !os.IsNotExist(err) {
					return LaunchContext{}, err
				}
				ctx.Nexinit = string(data)
			}
		case "has_exinit":
			if val == "1" {
				data, err := os.ReadFile(filepath.Join(dir, "exinit"))
				if err != nil && !os.IsNotExist(err) {
					return LaunchContext{}, err
				}
				ctx.Exinit = string(data)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return LaunchContext{}, err
	}
	return ctx, nil
}

// SetLaunchContext configures startup for the next LoadStartup call.
func (e *Engine) SetLaunchContext(ctx LaunchContext) {
	e.launchCtx = ctx
}
