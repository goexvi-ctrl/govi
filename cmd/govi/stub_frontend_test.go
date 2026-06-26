package main

import (
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
