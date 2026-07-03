package main

import (
	"testing"

	"govi/engine"
)

// stubFrontend is a headless editor host for main package tests.
type stubFrontend struct {
	eng    *engine.Engine
	closed bool
}

func (s *stubFrontend) Render(engine.View, engine.ChangeSet) {}
func (s *stubFrontend) Bell()                                {}
func (s *stubFrontend) SetTitle(string)                      {}
func (s *stubFrontend) Close()                               { s.closed = true }
func (s *stubFrontend) Attach(e *engine.Engine)              { s.eng = e }
func (s *stubFrontend) Run()                                 {}

func useStubFrontend() {
	newEditorFrontend = func() (editorHost, error) {
		return &stubFrontend{}, nil
	}
}

// startEngineAs runs the editor to the run loop under progname with a stub
// frontend and returns the attached engine for state assertions.
func startEngineAs(t *testing.T, progname string, args []string) *engine.Engine {
	t.Helper()
	t.Setenv("TMPDIR", t.TempDir())
	var eng *engine.Engine
	code, _, stderr := captureRunAs(t, progname, args, func() {
		useStubFrontend()
		runEditor = func(fe editorHost) {
			if s, ok := fe.(*stubFrontend); ok {
				eng = s.eng
			}
		}
	})
	if code != 0 || eng == nil {
		t.Fatalf("runIO as %q = %d (stderr %q), engine %v", progname, code, stderr, eng)
	}
	return eng
}

// TestRun_goexStartsInExMode checks the nvi program-name convention: invoked
// as goex (or ex/nex), the session starts at the ex prompt, and -v forces vi
// mode back on.
func TestRun_goexStartsInExMode(t *testing.T) {
	for _, name := range []string{"goex", "ex", "nex"} {
		if eng := startEngineAs(t, name, []string{"-s"}); !eng.ExActive() {
			t.Errorf("as %q: not in ex mode", name)
		}
	}
	if eng := startEngineAs(t, "govi", []string{"-s"}); eng.ExActive() {
		t.Error("as govi: unexpectedly in ex mode")
	}
	if eng := startEngineAs(t, "goex", []string{"-v", "-s"}); eng.ExActive() {
		t.Error("goex -v: -v should force vi mode")
	}
}

// TestRun_eFlagStartsInExMode checks -e (start in ex mode) under the normal
// program name.
func TestRun_eFlagStartsInExMode(t *testing.T) {
	if eng := startEngineAs(t, "govi", []string{"-e", "-s"}); !eng.ExActive() {
		t.Error("govi -e: not in ex mode")
	}
}

// TestRun_noFileOpensTempBuffer checks that govi with no file edits a throwaway
// temp buffer (like nvi), not an empty unnamed one.
func TestRun_noFileOpensTempBuffer(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	var eng *engine.Engine
	code, _, _ := captureRun(t, []string{"-s"}, func() {
		useStubFrontend()
		runEditor = func(fe editorHost) {
			if s, ok := fe.(*stubFrontend); ok {
				eng = s.eng
			}
		}
	})
	if code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	if eng == nil || !eng.IsTemporary() {
		t.Fatal("no-file invocation should open a temporary buffer")
	}
}
