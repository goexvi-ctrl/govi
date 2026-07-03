package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_help(t *testing.T) {
	code, _, _ := captureRun(t, []string{"-h"}, nil)
	if code != 0 {
		t.Fatalf("run(-h) = %d, want 0", code)
	}
}

func TestRun_unknownFlag(t *testing.T) {
	code, _, stderr := captureRun(t, []string{"-not-a-flag"}, nil)
	if code != 2 {
		t.Fatalf("run(-not-a-flag) = %d, want 2", code)
	}
	if !strings.Contains(stderr, "flag provided but not defined") {
		t.Fatalf("stderr = %q, want unknown-flag message", stderr)
	}
}

func TestRun_waitRequiresGUI(t *testing.T) {
	code, _, stderr := captureRun(t, []string{"-w"}, nil)
	if code != 2 {
		t.Fatalf("run(-w) = %d, want 2", code)
	}
	if !strings.Contains(stderr, "-w is only valid with -g") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRun_guiDelegatesToLauncher(t *testing.T) {
	var gotSilent, gotWait bool
	var gotFiles []string
	code, _, _ := captureRun(t, []string{"-g", "-s", "-w", "a", "b"}, func() {
		launchGUI = func(silent, wait bool, files []string) int {
			gotSilent, gotWait = silent, wait
			gotFiles = append([]string(nil), files...)
			return 0
		}
	})
	if code != 0 {
		t.Fatalf("run(-g) = %d, want 0", code)
	}
	if !gotSilent || !gotWait || len(gotFiles) != 2 || gotFiles[0] != "a" || gotFiles[1] != "b" {
		t.Fatalf("launchGUI(silent=%v wait=%v files=%v)", gotSilent, gotWait, gotFiles)
	}
}

func TestRun_terminalInitFailure(t *testing.T) {
	code, _, stderr := captureRun(t, []string{"-s"}, func() {
		newEditorFrontend = func() (editorHost, error) {
			return nil, errors.New("no tty")
		}
	})
	if code != 1 {
		t.Fatalf("run() = %d, want 1", code)
	}
	if !strings.Contains(stderr, "cannot initialize terminal") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRun_openUnreadableFile(t *testing.T) {
	// Missing paths open as a new file (vi behavior).
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(path, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := captureRun(t, []string{"-s", path}, useStubFrontend)
	if code != 1 {
		t.Fatalf("run() = %d, want 1", code)
	}
	if stderr == "" {
		t.Fatal("expected open error on stderr")
	}
}

func TestRun_editorWithoutRunLoop(t *testing.T) {
	var ran bool
	code, _, _ := captureRun(t, []string{"-s"}, func() {
		useStubFrontend()
		runEditor = func(editorHost) { ran = true }
	})
	if code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	if !ran {
		t.Fatal("runEditor was not called")
	}
}

func TestRun_openFileStartsEditor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var ran bool
	code, _, _ := captureRun(t, []string{"-s", path}, func() {
		useStubFrontend()
		runEditor = func(editorHost) { ran = true }
	})
	if code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	if !ran {
		t.Fatal("runEditor was not called")
	}
}

func TestRun_recoverListEmpty(t *testing.T) {
	recdir := t.TempDir()
	t.Setenv("EXINIT", "set recdir="+recdir)
	t.Setenv("NEXINIT", "")
	t.Setenv("HOME", t.TempDir())

	code, stdout, _ := captureRun(t, []string{"-r"}, useStubFrontend)
	if code != 0 {
		t.Fatalf("run(-r) = %d, want 0", code)
	}
	if !strings.Contains(stdout, "No files to recover") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRun_recoverList(t *testing.T) {
	recdir := t.TempDir()
	orig := filepath.Join(t.TempDir(), "edited.txt")
	body := fmt.Sprintf("govi recovery\nFile: %s\nTime: 1\nLines: 1\n\nhello\n", orig)
	if err := os.WriteFile(filepath.Join(recdir, "recover.edited.txt"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXINIT", "set recdir="+recdir)
	t.Setenv("NEXINIT", "")
	t.Setenv("HOME", t.TempDir())

	code, stdout, _ := captureRun(t, []string{"-r"}, useStubFrontend)
	if code != 0 {
		t.Fatalf("run(-r) = %d, want 0", code)
	}
	if !strings.Contains(stdout, orig) {
		t.Fatalf("stdout = %q, want path %q", stdout, orig)
	}
}

func TestRun_recoverNamedFile(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(orig, []byte("on disk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recdir := t.TempDir()
	body := fmt.Sprintf("govi recovery\nFile: %s\nTime: 1\nLines: 1\n\nrecovered\n", orig)
	if err := os.WriteFile(filepath.Join(recdir, "recover.doc.txt"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXINIT", "set recdir="+recdir)
	t.Setenv("NEXINIT", "")
	t.Setenv("HOME", t.TempDir())

	var ran bool
	code, _, stderr := captureRun(t, []string{"-r", orig}, func() {
		useStubFrontend()
		runEditor = func(editorHost) { ran = true }
	})
	if code != 0 {
		t.Fatalf("run(-r file) = %d, want 0; stderr=%q", code, stderr)
	}
	if !ran {
		t.Fatal("runEditor was not called after recover")
	}
}

func TestRun_startupFailure(t *testing.T) {
	home := t.TempDir()
	exrc := filepath.Join(home, ".exrc")
	if err := os.WriteFile(exrc, []byte("set number\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(exrc, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("EXINIT", "")
	t.Setenv("NEXINIT", "")

	code, _, stderr := captureRun(t, []string{}, useStubFrontend)
	if code != 1 {
		t.Fatalf("run() = %d, want 1", code)
	}
	if !strings.Contains(stderr, "govi:") {
		t.Fatalf("stderr = %q, want startup error", stderr)
	}
}

// captureRun calls runIO with hooked I/O as the default program name "govi".
// setup runs after hooks are saved and before runIO; use it to replace
// newEditorFrontend, runEditor, or launchGUI.
func captureRun(t *testing.T, args []string, setup func()) (int, string, string) {
	t.Helper()
	return captureRunAs(t, "govi", args, setup)
}

// captureRunAs is captureRun with an explicit program name (argv[0] base),
// for the ex/goex invocation tests.
func captureRunAs(t *testing.T, progname string, args []string, setup func()) (int, string, string) {
	t.Helper()

	oldNew := newEditorFrontend
	oldRun := runEditor
	oldGUI := launchGUI
	t.Cleanup(func() {
		newEditorFrontend = oldNew
		runEditor = oldRun
		launchGUI = oldGUI
	})

	if setup != nil {
		setup()
	}

	var stdout, stderr bytes.Buffer
	code := runIO(progname, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}
