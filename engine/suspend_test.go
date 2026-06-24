package engine

import "testing"

type suspendStubFrontend struct {
	captureFrontend
	suspended bool
}

func (f *suspendStubFrontend) Suspend() error {
	f.suspended = true
	return nil
}

func TestCtrlZSuspend(t *testing.T) {
	fe := &suspendStubFrontend{}
	e := New(fe, Options{})
	e.OpenArgs(nil)
	e.Resize(10, 40)

	e.Input(KeyEvent{Rune: 'z', Mods: ModCtrl})
	if !fe.suspended {
		t.Fatal("^Z should suspend")
	}
}

func TestCtrlZSuspendFromInsert(t *testing.T) {
	fe := &suspendStubFrontend{}
	e := New(fe, Options{})
	e.OpenArgs(nil)
	e.Resize(10, 40)

	e.Input(KeyEvent{Rune: 'i'})
	e.Input(KeyEvent{Rune: 'x'})
	e.Input(KeyEvent{Rune: 'z', Mods: ModCtrl})
	if e.scr.mode != ModeCommand {
		t.Fatalf("expected command mode after ^Z in insert, got %v", e.scr.mode)
	}
	if !fe.suspended {
		t.Fatal("^Z in insert should suspend")
	}
}

func TestExSuspend(t *testing.T) {
	fe := &suspendStubFrontend{}
	e := New(fe, Options{})
	e.OpenArgs(nil)
	e.Resize(10, 40)

	if err := e.exExecute("suspend"); err != nil {
		t.Fatal(err)
	}
	if !fe.suspended {
		t.Fatal(":suspend should suspend")
	}
}

func TestExStop(t *testing.T) {
	fe := &suspendStubFrontend{}
	e := New(fe, Options{})
	e.OpenArgs(nil)
	e.Resize(10, 40)

	if err := e.exExecute("stop"); err != nil {
		t.Fatal(err)
	}
	if !fe.suspended {
		t.Fatal(":stop should suspend")
	}
}

func TestSuspendSecure(t *testing.T) {
	fe := &suspendStubFrontend{}
	e := New(fe, Options{})
	e.OpenArgs(nil)
	e.Resize(10, 40)
	e.exExecute("set secure")

	if err := e.exExecute("suspend"); err == nil {
		t.Fatal("expected secure error")
	}
	if fe.suspended {
		t.Fatal("should not suspend in secure mode")
	}

	e.Input(KeyEvent{Rune: 'z', Mods: ModCtrl})
	if fe.suspended {
		t.Fatal("^Z should not suspend in secure mode")
	}
}

func TestSuspendNoFrontend(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if err := e.exExecute("suspend"); err == nil {
		t.Fatal("expected error without Suspender")
	}
}
