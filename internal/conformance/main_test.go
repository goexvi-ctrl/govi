package conformance

import (
	"os"
	"testing"
)

// recoverParent is the single directory under which every test run's throwaway
// recovery directory is created (shared with the goterm harness so stale runs
// can be swept wholesale with `rm -rf /var/tmp/goterm_testing`).
const recoverParent = "/var/tmp/goterm_testing"

// TestMain isolates crash-recovery files for the whole conformance binary.
//
// The harness spawns real editors -- the nvi oracle and the built govi -- for
// the ex-batch conformance and the regex fuzzers, and any edit makes them drop
// recovery files into their recovery directory. Left at the default that is
// the shared system location (/var/tmp/vi.recover), and a fuzzer run that
// kills a trial on timeout leaves its files behind, thousands per session.
// Both editors honor GOTERM_ORACLE_PRESERVE as the recovery directory (govi
// via its recdir option default in engine/options.go, the nvi oracle via
// common/options.c), so point it at a fresh throwaway directory and remove it
// when the suite ends. exec inherits the environment, so setting it here
// reaches every spawned editor.
func TestMain(m *testing.M) {
	if err := os.MkdirAll(recoverParent, 0o700); err != nil {
		panic(err)
	}
	dir, err := os.MkdirTemp(recoverParent, "vi.recover.")
	if err != nil {
		panic(err)
	}
	os.Setenv("GOTERM_ORACLE_PRESERVE", dir)

	code := m.Run()

	os.RemoveAll(dir)
	// Best effort: remove the shared parent too. This succeeds only when it is
	// empty, so a concurrent run's directory is never disturbed.
	os.Remove(recoverParent)
	os.Exit(code)
}
