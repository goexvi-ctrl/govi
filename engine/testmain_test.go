package engine

import (
	"os"
	"testing"
)

// TestMain isolates the recovery directory for the whole engine test binary.
// Many tests (and benchmarks) edit buffers, and editing writes a recovery file;
// left to the default they would scatter recover.* files into the shared system
// recovery dir (/var/tmp/vi.recover, the recdir default) -- benchmarks that
// never quit, and tests that abort, leak them there. Point the default at a
// throwaway dir and remove it when the suite finishes. Tests that exercise
// recovery directly set their own recdir and are unaffected.
func TestMain(m *testing.M) {
	// Keep the path short: ":set all" display tests check that no option line
	// exceeds 80 columns, and a long recdir value would wrap the grid.
	dir, err := os.MkdirTemp("/tmp", "gvr.")
	if err != nil {
		if dir, err = os.MkdirTemp("", "gvr."); err != nil {
			panic(err)
		}
	}
	for i := range optDefs {
		if optDefs[i].name == "recdir" {
			optDefs[i].dS = dir
			break
		}
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
