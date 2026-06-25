//go:build unix

package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStartupLaunchCwd(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(dir, "proj")
	if err := os.Mkdir(proj, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".exrc"), []byte("set number\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("NEXINIT", "")
	t.Setenv("EXINIT", "")
	t.Chdir(dir) // not proj: local .exrc must come from launch context

	e := newBareEngine(t)
	if err := e.exExecute("set exrc"); err != nil {
		t.Fatal(err)
	}
	e.SetLaunchContext(LaunchContext{Cwd: proj})
	if err := e.LoadStartup(); err != nil {
		t.Fatal(err)
	}
	if !e.scr.opts.Bool("number") {
		t.Fatal("expected number from launch-context cwd .exrc")
	}
}

func TestLoadStartupLaunchSilent(t *testing.T) {
	home := t.TempDir()
	exrc := filepath.Join(home, ".nexrc")
	if err := os.WriteFile(exrc, []byte("set number\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("NEXINIT", "")
	t.Setenv("EXINIT", "")

	e := newBareEngine(t)
	e.SetLaunchContext(LaunchContext{Silent: true, HomeExrc: exrc})
	if err := e.LoadStartup(); err != nil {
		t.Fatal(err)
	}
	if e.scr.opts.Bool("number") {
		t.Fatal("silent launch context should skip startup")
	}
}
