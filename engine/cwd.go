package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SetCwd sets this editor instance's working directory (per tab in Govi.app).
func (e *Engine) SetCwd(dir string) {
	if dir != "" {
		e.cwd = filepath.Clean(dir)
	}
}

// Cwd returns the working directory for relative file paths and :cd.
func (e *Engine) Cwd() string {
	return e.cwd
}

// InitCwd establishes the working directory from launch context, the process
// cwd, or $HOME when not already set.
func (e *Engine) InitCwd() { e.ensureCwd() }

// ensureCwd initializes cwd from the process working directory or $HOME.
func (e *Engine) ensureCwd() {
	if e.cwd != "" {
		return
	}
	if e.launchCtx.Cwd != "" {
		e.cwd = filepath.Clean(e.launchCtx.Cwd)
		return
	}
	if wd, err := os.Getwd(); err == nil && wd != "" {
		e.cwd = wd
		return
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		e.cwd = home
	}
}

// resolvePath maps a user-supplied path through this editor's cwd. Absolute
// paths and empty strings are returned unchanged.
func (e *Engine) resolvePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	e.ensureCwd()
	if e.cwd == "" {
		return path
	}
	return filepath.Clean(filepath.Join(e.cwd, path))
}

// canonicalPath resolves path against cwd and returns an absolute, cleaned path
// for buffer naming and :e / :w.
func (e *Engine) canonicalPath(path string) string {
	path = e.resolvePath(path)
	if path == "" {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return filepath.Clean(abs)
}

// exCd implements :cd[!] and :chdir[!] (ex/ex_cd.c).
func (e *Engine) exCd(c *exCmd) error {
	if e.scr.dirty() && !c.force {
		name := e.scr.name
		if name != "" && !filepath.IsAbs(name) {
			return fmt.Errorf("File modified since last complete write; write or use ! to override")
		}
	}

	dir := strings.TrimSpace(c.arg)
	homeCd := dir == ""
	if homeCd {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return fmt.Errorf("Unable to find home directory location")
		}
		dir = home
	}

	e.ensureCwd()
	if abs, ok := e.cdTo(dir); ok {
		e.cwd = abs
		return nil
	}

	if homeCd || cdDirectOnly(dir) {
		return fmt.Errorf("%s", dir)
	}

	cdpath := e.scr.opts.Str("cdpath")
	if cdpath == "" {
		if p := os.Getenv("CDPATH"); p != "" {
			cdpath = p
		} else {
			cdpath = ":"
		}
	}
	for _, prefix := range cdpathPrefixes(cdpath) {
		base := prefix
		if base == "." {
			base = e.cwd
		}
		candidate := filepath.Join(base, dir)
		if abs, ok := e.cdTo(candidate); ok {
			e.cwd = abs
			e.scr.msg = fmt.Sprintf("New current directory: %s", abs)
			e.scr.msgKind = MsgInfo
			return nil
		}
	}
	return fmt.Errorf("%s", dir)
}

func (e *Engine) cdTo(dir string) (string, bool) {
	target := dir
	if !filepath.IsAbs(target) {
		target = filepath.Join(e.cwd, target)
	}
	target = filepath.Clean(target)
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return "", false
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", false
	}
	return abs, true
}

func cdDirectOnly(dir string) bool {
	if filepath.IsAbs(dir) {
		return true
	}
	if dir == "." {
		return true
	}
	return strings.HasPrefix(dir, "../") || strings.HasPrefix(dir, "./")
}

func cdpathPrefixes(cdpath string) []string {
	parts := strings.Split(cdpath, ":")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" || p == "." {
			out = append(out, ".")
		} else {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"."}
	}
	return out
}
