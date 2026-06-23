package engine

import "testing"

type shellStubFrontend struct {
	captureFrontend
	shell  string
	inEx   bool
	called bool
	retErr error
}

func (f *shellStubFrontend) RunShell(shell string, inExMode bool) error {
	f.called = true
	f.shell = shell
	f.inEx = inExMode
	return f.retErr
}

func TestExShell(t *testing.T) {
	fe := &shellStubFrontend{}
	e := New(fe, Options{})
	e.OpenArgs(nil)
	e.Resize(10, 40)
	e.exExecute("set shell=/bin/ksh")

	if err := e.exExecute("shell"); err != nil {
		t.Fatal(err)
	}
	if !fe.called || fe.shell != "/bin/ksh" {
		t.Fatalf("RunShell: called=%v shell=%q", fe.called, fe.shell)
	}
	if fe.inEx {
		t.Fatal("RunShell from vi should not set inExMode")
	}
}

func TestExShellAbbrev(t *testing.T) {
	fe := &shellStubFrontend{}
	e := New(fe, Options{})
	e.OpenArgs(nil)
	e.Resize(10, 40)

	if err := e.exExecute("sh"); err != nil {
		t.Fatal(err)
	}
	if !fe.called {
		t.Fatal("sh did not invoke RunShell")
	}
}

func TestExShellSecure(t *testing.T) {
	fe := &shellStubFrontend{}
	e := New(fe, Options{})
	e.OpenArgs(nil)
	e.Resize(10, 40)
	e.exExecute("set secure")

	err := e.exExecute("shell")
	if err == nil {
		t.Fatal("expected secure error")
	}
	if fe.called {
		t.Fatal("RunShell should not run in secure mode")
	}
}

func TestExShellNoRunner(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if err := e.exExecute("shell"); err == nil {
		t.Fatal("expected error without ShellRunner")
	}
}