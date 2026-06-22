package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newRecEngine opens content as a file and points recdir at an isolated temp
// directory.
func newRecEngine(t *testing.T, content string) (*Engine, string, string) {
	t.Helper()
	dir := t.TempDir()
	recdir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	e := New(&captureFrontend{}, Options{})
	if err := e.Open(path); err != nil {
		t.Fatal(err)
	}
	e.Resize(10, 40)
	e.exExecute("set recdir=" + recdir)
	return e, path, recdir
}

func TestRecoveryWrittenOnEdit(t *testing.T) {
	e, _, recdir := newRecEngine(t, "hello\nworld\n")
	drive(e, "x") // modify -> recovery written immediately
	if e.recoverPath == "" {
		t.Fatal("no recovery file created on first edit")
	}
	rec, err := parseRecovery(e.recoverPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.lines) != 2 || string(rec.lines[0]) != "ello" {
		t.Fatalf("recovery contents = %q", rec.lines)
	}
	matches, _ := filepath.Glob(filepath.Join(recdir, "recover.*"))
	if len(matches) != 1 {
		t.Fatalf("recdir has %d recover files, want 1", len(matches))
	}
}

func TestRecoverRestoresChanges(t *testing.T) {
	// Session A edits and "crashes" (never writes/quits).
	eA, path, recdir := newRecEngine(t, "alpha\nbeta\ngamma\n")
	drive(eA, "ddO inserted\x1b") // delete line 1, open a new first line
	eA.SyncRecovery()             // flush (as the idle timer / :preserve would)
	if eA.recoverPath == "" {
		t.Fatal("expected recovery file")
	}
	wantText := bufText(eA)

	// Session B recovers using the same recdir.
	eB := New(&captureFrontend{}, Options{})
	eB.Resize(10, 40)
	eB.exExecute("set recdir=" + recdir)
	if err := eB.Recover(path); err != nil {
		t.Fatalf("recover: %v", err)
	}
	if bufText(eB) != wantText {
		t.Fatalf("recovered %q, want %q", bufText(eB), wantText)
	}
	if !eB.scr.modified {
		t.Fatal("recovered buffer should be modified (unsaved)")
	}
	if eB.scr.name != path {
		t.Fatalf("recovered name = %q, want %q", eB.scr.name, path)
	}
}

func TestPreserveCommand(t *testing.T) {
	e, _, _ := newRecEngine(t, "one\ntwo\n")
	drive(e, "x") // -> "ne"; first change synced immediately

	// A further change is throttled, so the recovery file is briefly stale.
	drive(e, "jx") // line 2 -> "wo"
	if rec, _ := parseRecovery(e.recoverPath); string(rec.lines[1]) != "two" {
		t.Fatalf("expected recovery still stale; line2 = %q", string(rec.lines[1]))
	}

	// :preserve forces it current.
	if err := e.exExecute("preserve"); err != nil {
		t.Fatal(err)
	}
	rec, _ := parseRecovery(e.recoverPath)
	if string(rec.lines[0]) != "ne" || string(rec.lines[1]) != "wo" {
		t.Fatalf("after :preserve recovery = %q", rec.lines)
	}
}

func TestCleanWriteRemovesRecovery(t *testing.T) {
	e, _, recdir := newRecEngine(t, "hello\n")
	drive(e, "x")
	if e.recoverPath == "" {
		t.Fatal("expected recovery file")
	}
	e.exExecute("w")
	if e.recoverPath != "" {
		t.Fatal("recovery path not cleared after write")
	}
	if matches, _ := filepath.Glob(filepath.Join(recdir, "recover.*")); len(matches) != 0 {
		t.Fatalf("recovery file left on disk after :w: %v", matches)
	}
}

func TestQuitRemovesRecovery(t *testing.T) {
	e, _, recdir := newRecEngine(t, "hello\n")
	drive(e, "x")
	recPath := e.recoverPath
	e.exExecute("q!")
	if _, err := os.Stat(recPath); !os.IsNotExist(err) {
		t.Fatal("recovery file should be removed on quit")
	}
	if matches, _ := filepath.Glob(filepath.Join(recdir, "recover.*")); len(matches) != 0 {
		t.Fatalf("recovery left after quit: %v", matches)
	}
}

func TestOpenWarnsAboutRecovery(t *testing.T) {
	e, path, recdir := newRecEngine(t, "data\n")
	drive(e, "x") // creates a recovery file

	// Re-open the same file with the same recdir; it should warn.
	e2 := New(&captureFrontend{}, Options{})
	e2.exExecute("set recdir=" + recdir)
	if err := e2.Open(path); err != nil {
		t.Fatal(err)
	}
	msg, _ := (view{e2.scr}).Message()
	if !strings.Contains(msg, "recover") {
		t.Fatalf("open message = %q, want a recovery warning", msg)
	}
}
