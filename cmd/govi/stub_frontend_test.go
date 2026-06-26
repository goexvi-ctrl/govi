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
