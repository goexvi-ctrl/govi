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

type bangStubFrontend struct {
	captureFrontend
	out    string
	called bool
}

func (f *bangStubFrontend) RunBang(shell, cmd, cwd string, cols, rows int) (string, error) {
	f.called = true
	return f.out, nil
}

func TestExBangShowsAllLines(t *testing.T) {
	fe := &bangStubFrontend{out: "one\ntwo\nthree\n"}
	e := New(fe, Options{})
	e.OpenArgs(nil)
	e.Resize(10, 40)

	if err := e.exExecute("!echo test"); err != nil {
		t.Fatal(err)
	}
	if !fe.called {
		t.Fatal("RunBang not called")
	}
	if got := len(e.scr.pendingOutput); got != 3 {
		t.Fatalf("pendingOutput lines = %d, want 3 (%v)", got, e.scr.pendingOutput)
	}
}

func TestExBangPipeFallback(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.Resize(10, 40)
	if err := e.exExecute("!printf 'a\\nb\\n'"); err != nil {
		t.Fatal(err)
	}
	if got := len(e.scr.pendingOutput); got != 2 {
		t.Fatalf("pendingOutput = %v, want 2 lines", e.scr.pendingOutput)
	}
}

func TestShellEnvSetsColumns(t *testing.T) {
	env := shellEnv(120, 30)
	var cols, lines string
	for _, e := range env {
		if len(e) > 8 && e[:8] == "COLUMNS=" {
			cols = e[8:]
		}
		if len(e) > 6 && e[:6] == "LINES=" {
			lines = e[6:]
		}
	}
	if cols != "120" || lines != "30" {
		t.Fatalf("env COLUMNS=%q LINES=%q", cols, lines)
	}
}
