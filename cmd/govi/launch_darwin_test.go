//go:build darwin

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

type consoleFileInfo struct {
	uid uint32
}

func (f consoleFileInfo) Name() string       { return "console" }
func (f consoleFileInfo) Size() int64        { return 0 }
func (f consoleFileInfo) Mode() os.FileMode  { return 0 }
func (f consoleFileInfo) ModTime() time.Time { return time.Time{} }
func (f consoleFileInfo) IsDir() bool        { return false }
func (f consoleFileInfo) Sys() any           { return &syscall.Stat_t{Uid: f.uid} }

func saveGUIHooks(t *testing.T) {
	t.Helper()
	oldStat := osStat
	oldRun := runCommand
	oldFifo := makeWaitFifoFn
	oldWait := waitOnFifo
	t.Cleanup(func() {
		osStat = oldStat
		runCommand = oldRun
		makeWaitFifoFn = oldFifo
		waitOnFifo = oldWait
	})
}

func stubConsoleOwner(t *testing.T, uid uint32) {
	t.Helper()
	osStat = func(name string) (os.FileInfo, error) {
		if name == "/dev/console" {
			return consoleFileInfo{uid: uid}, nil
		}
		return os.Stat(name)
	}
}

func guiHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func launchDir(home string) string {
	return filepath.Join(home, "Library", "Application Support", "GoVi", "launch")
}

func TestRunGUI_waitRequiresFiles(t *testing.T) {
	code := runGUI(false, true, nil)
	if code != 2 {
		t.Fatalf("runGUI(-w, no files) = %d, want 2", code)
	}
}

func TestRunGUI_wrongConsoleUser(t *testing.T) {
	saveGUIHooks(t)
	guiHome(t)
	stubConsoleOwner(t, uint32(os.Getuid())+1)

	file := filepath.Join(t.TempDir(), "file.txt")
	code := runGUI(false, false, []string{file})
	if code != 1 {
		t.Fatalf("runGUI() = %d, want 1", code)
	}
}

func TestRunGUI_openCommandFails(t *testing.T) {
	saveGUIHooks(t)
	guiHome(t)
	stubConsoleOwner(t, uint32(os.Getuid()))
	runCommand = func(cmd string, args ...string) error {
		return errors.New("open failed")
	}

	file := filepath.Join(t.TempDir(), "file.txt")
	code := runGUI(false, false, []string{file})
	if code != 1 {
		t.Fatalf("runGUI() = %d, want 1", code)
	}
}

func TestRunGUI_success(t *testing.T) {
	saveGUIHooks(t)
	home := guiHome(t)
	stubConsoleOwner(t, uint32(os.Getuid()))

	dir := t.TempDir()
	file := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	var opened string
	runCommand = func(cmd string, args ...string) error {
		if cmd != "open" || len(args) != 1 {
			t.Fatalf("runCommand(%q, %v)", cmd, args)
		}
		opened = args[0]
		return nil
	}

	code := runGUI(true, false, []string{file})
	if code != 0 {
		t.Fatalf("runGUI() = %d, want 0", code)
	}
	if !strings.HasPrefix(opened, "govi://open?ctx=") {
		t.Fatalf("opened = %q", opened)
	}

	matches, err := filepath.Glob(filepath.Join(launchDir(home), "ctx-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("payload files = %v, want 1", matches)
	}
	body, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "silent=1\n") {
		t.Fatalf("payload = %q, want silent=1", text)
	}
	if !strings.Contains(text, "file="+file+"\n") {
		t.Fatalf("payload = %q, want file=%s", text, file)
	}
	if !strings.Contains(text, "cwd=") {
		t.Fatalf("payload = %q, want cwd", text)
	}
}

func TestRunGUI_createsMissingFile(t *testing.T) {
	saveGUIHooks(t)
	guiHome(t)
	stubConsoleOwner(t, uint32(os.Getuid()))
	runCommand = func(string, ...string) error { return nil }

	dir := t.TempDir()
	missing := filepath.Join(dir, "new.txt")
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("precondition: %v", err)
	}

	code := runGUI(false, false, []string{missing})
	if code != 0 {
		t.Fatalf("runGUI() = %d, want 0", code)
	}
	if _, err := os.Stat(missing); err != nil {
		t.Fatalf("missing file not created: %v", err)
	}
}

func TestRunGUI_withWait(t *testing.T) {
	saveGUIHooks(t)
	home := guiHome(t)
	stubConsoleOwner(t, uint32(os.Getuid()))

	fifo := filepath.Join(t.TempDir(), "wait.fifo")
	makeWaitFifoFn = func() (string, error) { return fifo, nil }
	waited := false
	waitOnFifo = func(path string) {
		waited = true
		if path != fifo {
			t.Fatalf("waitOnFifo(%q), want %q", path, fifo)
		}
	}
	runCommand = func(string, ...string) error { return nil }

	file := filepath.Join(t.TempDir(), "doc.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runGUI(false, true, []string{file})
	if code != 0 {
		t.Fatalf("runGUI(-w) = %d, want 0", code)
	}
	if !waited {
		t.Fatal("waitOnFifo was not called")
	}

	body, err := os.ReadFile(firstGlob(t, filepath.Join(launchDir(home), "ctx-*")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "fifo="+fifo+"\n") {
		t.Fatalf("payload = %q, want fifo=%s", body, fifo)
	}
}

func TestRunGUI_noFilesUsesTempBuffer(t *testing.T) {
	saveGUIHooks(t)
	guiHome(t)
	stubConsoleOwner(t, uint32(os.Getuid()))
	runCommand = func(string, ...string) error { return nil }

	code := runGUI(false, false, nil)
	if code != 0 {
		t.Fatalf("runGUI() = %d, want 0", code)
	}
}

func firstGlob(t *testing.T, pattern string) string {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("glob %q = %v, want 1 match", pattern, matches)
	}
	return matches[0]
}
