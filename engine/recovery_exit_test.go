package engine

import (
	"path/filepath"
	"testing"
)

// recoverFiles returns the recover.* files currently in dir.
func recoverFiles(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, "recover.*"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// TestRecoveryRemovedOnNormalExit checks that each normal-exit command deletes
// the session's recovery file, so the recovery directory does not accumulate
// cruft. A signal-driven exit (preserveAndQuit) intentionally keeps it and is
// not covered here.
// TestRecoveryPreservedSurvivesExit checks that a :preserve'd recovery file is
// NOT deleted by a normal exit (nvi RCV_PRESERVE semantics): the snapshot must
// stay behind for a later vi -r.
func TestRecoveryPreservedSurvivesExit(t *testing.T) {
	recdir := t.TempDir()
	e, _, _ := newTestEngine(t, "alpha\nbeta\ngamma\n")
	if err := e.RunEx("set recdir=" + recdir); err != nil {
		t.Fatal(err)
	}
	if err := e.RunEx("1s/alpha/ALPHA/"); err != nil {
		t.Fatal(err)
	}
	e.noteRecovery()
	if err := e.RunEx("preserve"); err != nil {
		t.Fatal(err)
	}
	if err := e.RunEx("q!"); err != nil {
		t.Fatal(err)
	}
	if got := recoverFiles(t, recdir); len(got) != 1 {
		t.Fatalf("preserved recovery file should survive a normal exit; found %v", got)
	}
}

func TestRecoveryRemovedOnNormalExit(t *testing.T) {
	for _, quit := range []string{"q!", "wq!", "x", "xit!"} {
		t.Run(quit, func(t *testing.T) {
			recdir := t.TempDir()
			e, _, _ := newTestEngine(t, "alpha\nbeta\ngamma\n")
			if err := e.RunEx("set recdir=" + recdir); err != nil {
				t.Fatal(err)
			}
			// Make a change so a recovery file is written.
			if err := e.RunEx("1s/alpha/ALPHA/"); err != nil {
				t.Fatal(err)
			}
			e.noteRecovery()
			if got := recoverFiles(t, recdir); len(got) == 0 {
				t.Fatalf("%s: expected a recovery file after an edit, found none", quit)
			}
			if err := e.RunEx(quit); err != nil {
				t.Fatalf("%s: %v", quit, err)
			}
			if !e.ShouldQuit() {
				t.Fatalf("%s: editor did not quit", quit)
			}
			if got := recoverFiles(t, recdir); len(got) != 0 {
				t.Fatalf("%s: recovery file(s) left after normal exit: %v", quit, got)
			}
		})
	}
}
