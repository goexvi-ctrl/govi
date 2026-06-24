//go:build unix

package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func newBareEngine(t *testing.T) *Engine {
	t.Helper()
	return New(&captureFrontend{}, Options{})
}

func TestLoadStartupHomeNexrc(t *testing.T) {
	home := t.TempDir()
	exrc := filepath.Join(home, ".nexrc")
	if err := os.WriteFile(exrc, []byte("set number\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("NEXINIT", "")
	t.Setenv("EXINIT", "")

	e := newBareEngine(t)
	if err := e.LoadStartup(); err != nil {
		t.Fatal(err)
	}
	if !e.scr.opts.Bool("number") {
		t.Fatal("expected number from $HOME/.nexrc")
	}
}

func TestLoadStartupEXINITSkipsHome(t *testing.T) {
	home := t.TempDir()
	exrc := filepath.Join(home, ".nexrc")
	if err := os.WriteFile(exrc, []byte("set number\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("EXINIT", "set ai")

	e := newBareEngine(t)
	if err := e.LoadStartup(); err != nil {
		t.Fatal(err)
	}
	if e.scr.opts.Bool("number") {
		t.Fatal("HOME .nexrc should be skipped when EXINIT is set")
	}
	if !e.scr.opts.Bool("autoindent") {
		t.Fatal("expected autoindent from EXINIT")
	}
}

func TestLoadStartupLocalExrcRequiresOption(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	wd := filepath.Join(dir, "proj")
	if err := os.Mkdir(wd, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wd, ".exrc"), []byte("set number\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("NEXINIT", "")
	t.Setenv("EXINIT", "")
	t.Chdir(wd)

	e := newBareEngine(t)
	if err := e.LoadStartup(); err != nil {
		t.Fatal(err)
	}
	if e.scr.opts.Bool("number") {
		t.Fatal("local .exrc should not load without exrc option")
	}

	if err := e.exExecute("set exrc"); err != nil {
		t.Fatal(err)
	}
	if err := e.LoadStartup(); err != nil {
		t.Fatal(err)
	}
	if !e.scr.opts.Bool("number") {
		t.Fatal("local .exrc should load when exrc is set")
	}
}

func TestLoadStartupExrcColors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	exrc := filepath.Join(home, ".nexrc")
	if err := os.WriteFile(exrc, []byte("set foreground=wheat background=#001122\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := newBareEngine(t)
	if err := e.LoadStartup(); err != nil {
		t.Fatal(err)
	}
	if got := e.scr.opts.Str("foreground"); got != "wheat" {
		t.Fatalf("foreground = %q, want wheat", got)
	}
	if got := e.scr.opts.Str("background"); got != "#001122" {
		t.Fatalf("background = %q, want #001122", got)
	}
}

func TestLoadStartupIgnoresComments(t *testing.T) {
	home := t.TempDir()
	exrc := filepath.Join(home, ".nexrc")
	content := "\" comment\nset number\n"
	if err := os.WriteFile(exrc, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("NEXINIT", "")
	t.Setenv("EXINIT", "")

	e := newBareEngine(t)
	if err := e.LoadStartup(); err != nil {
		t.Fatal(err)
	}
	if !e.scr.opts.Bool("number") {
		t.Fatal("expected number option")
	}
}

func TestExSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cmds")
	if err := os.WriteFile(path, []byte("set ai\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := newBareEngine(t)
	if err := e.exExecute("source " + path); err != nil {
		t.Fatal(err)
	}
	if !e.scr.opts.Bool("autoindent") {
		t.Fatal("source should run set ai")
	}
}

func TestExrcRejectsGroupWritable(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".exrc")
	if err := os.WriteFile(path, []byte("set number\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o664); err != nil {
		t.Fatal(err)
	}
	v, _, err := exrcAllowed(path, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if v != exrcNoPerm {
		t.Fatalf("group-writable .exrc should be rejected, got %v", v)
	}
}
