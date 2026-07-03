package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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
		if eng := startEngineAs(t, name, []string{"-n"}); !eng.ExActive() {
			t.Errorf("as %q: not in ex mode", name)
		}
	}
	if eng := startEngineAs(t, "govi", []string{"-n"}); eng.ExActive() {
		t.Error("as govi: unexpectedly in ex mode")
	}
	if eng := startEngineAs(t, "goex", []string{"-v", "-n"}); eng.ExActive() {
		t.Error("goex -v: -v should force vi mode")
	}
}

// TestRun_eFlagStartsInExMode checks -e (start in ex mode) under the normal
// program name.
func TestRun_eFlagStartsInExMode(t *testing.T) {
	if eng := startEngineAs(t, "govi", []string{"-e", "-n"}); !eng.ExActive() {
		t.Error("govi -e: not in ex mode")
	}
}

// writeFixture creates a small numbered-line file for flag tests.
func writeFixture(t *testing.T, lines string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture")
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestRun_readonlyFlagAndViewNames checks -R and the view/nview/goview
// program names set the readonly option (nvi progname handling).
func TestRun_readonlyFlagAndViewNames(t *testing.T) {
	file := writeFixture(t, "one\n")
	for _, c := range []struct {
		prog string
		args []string
	}{
		{"govi", []string{"-R", "-n", file}},
		{"view", []string{"-n", file}},
		{"nview", []string{"-n", file}},
		{"goview", []string{"-n", file}},
	} {
		eng := startEngineAs(t, c.prog, c.args)
		if err := eng.RunEx("w"); err == nil {
			t.Errorf("%s %v: :w succeeded, want readonly refusal", c.prog, c.args)
		}
	}
	eng := startEngineAs(t, "govi", []string{"-n", file})
	if err := eng.RunEx("w"); err != nil {
		t.Errorf("no -R: :w failed: %v", err)
	}
}

// TestRun_secureFlag checks -S disables shell access.
func TestRun_secureFlag(t *testing.T) {
	eng := startEngineAs(t, "govi", []string{"-S", "-n"})
	if err := eng.RunEx("!echo hi"); err == nil {
		t.Error("-S: shell escape was allowed")
	}
}

// TestRun_commandFlag checks -c and the historic +command form run an ex
// command after the file loads.
func TestRun_commandFlag(t *testing.T) {
	file := writeFixture(t, "one\ntwo\nthree\n")
	for _, args := range [][]string{
		{"-n", "-c", "$", file},
		{"-n", "+$", file},
	} {
		eng := startEngineAs(t, "govi", args)
		eng.WithView(func(v engine.View) {
			if v.Cursor().Line != 3 {
				t.Errorf("%v: cursor line %d, want 3 (last line)", args, v.Cursor().Line)
			}
		})
	}
}

// TestRun_windowFlag checks -w records the window size and the first Resize
// applies it to the vi map.
func TestRun_windowFlag(t *testing.T) {
	file := writeFixture(t, "one\ntwo\nthree\n")
	eng := startEngineAs(t, "govi", []string{"-n", "-w", "5", file})
	eng.Resize(24, 80)
	eng.WithView(func(v engine.View) {
		if got := v.Viewport().MapRows; got != 5 {
			t.Errorf("-w 5 after Resize: MapRows = %d, want 5", got)
		}
	})
}

// TestRun_tagFlag checks -t starts at the named tag.
func TestRun_tagFlag(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("alpha\nthe tag line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tags := "sym\ttarget.txt\t/^the tag line$/\n"
	if err := os.WriteFile(filepath.Join(dir, "tags"), []byte(tags), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	eng := startEngineAs(t, "govi", []string{"-n", "-t", "sym"})
	eng.WithView(func(v engine.View) {
		if v.Name() != "target.txt" || v.Cursor().Line != 2 {
			t.Errorf("-t sym: at %s line %d, want target.txt line 2", v.Name(), v.Cursor().Line)
		}
	})
}

// runBatchIO runs a batch invocation (no frontend hooks needed: batch mode
// builds its own nullHost) and returns exit code, stdout, stderr.
func runBatchIO(t *testing.T, progname string, args []string, scripted bool, script string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runIO(progname, args, scripted, strings.NewReader(script), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// TestRun_exBatchMode checks -s: the ex script on stdin runs headlessly (nvi
// ex batch mode) and edits the file.
func TestRun_exBatchMode(t *testing.T) {
	file := writeFixture(t, "one\ntwo\n")
	code, stdout, stderr := runBatchIO(t, "goex", []string{"-s", file}, false,
		"%s/one/ONE/\n1p\nwq\n")
	if code != 0 {
		t.Fatalf("goex -s = %d, stderr %q", code, stderr)
	}
	// Only :1p's output appears: load/write messages are suppressed (nvi
	// SC_EX_SILENT).
	if stdout != "ONE\n" {
		t.Errorf("batch stdout = %q, want only the :1p line", stdout)
	}
	got, err := os.ReadFile(file)
	if err != nil || string(got) != "ONE\ntwo\n" {
		t.Errorf("file after batch = %q, %v; want substituted content", got, err)
	}
}

// TestRun_exBatchAbortsOnError checks nvi's script semantics: the first
// failing command reports "script, N:" and aborts with exit 1, without
// running the rest of the script.
func TestRun_exBatchAbortsOnError(t *testing.T) {
	file := writeFixture(t, "a\n")
	code, stdout, _ := runBatchIO(t, "goex", []string{"-s", file}, false,
		"notacommand\ns/a/X/\nwq\n")
	if code != 1 {
		t.Fatalf("bad script exit = %d, want 1", code)
	}
	if !strings.Contains(stdout, "script, 1:") {
		t.Errorf("stdout = %q, want a script, 1: error", stdout)
	}
	if got, _ := os.ReadFile(file); string(got) != "a\n" {
		t.Errorf("file = %q; the script should have aborted before s/// and wq", got)
	}
}

// TestRun_scriptedStdinImpliesBatch checks nvi's G_SCRIPTED rule: in ex mode a
// redirected stdin batches even without -s.
func TestRun_scriptedStdinImpliesBatch(t *testing.T) {
	file := writeFixture(t, "alpha\n")
	code, _, stderr := runBatchIO(t, "goex", []string{file}, true, "s/alpha/beta/\nwq\n")
	if code != 0 {
		t.Fatalf("scripted goex = %d, stderr %q", code, stderr)
	}
	if got, _ := os.ReadFile(file); string(got) != "beta\n" {
		t.Errorf("file after scripted batch = %q, want beta", got)
	}
}

// TestRun_batchOnlyForEx checks nvi's rule that -s is only applicable to ex.
func TestRun_batchOnlyForEx(t *testing.T) {
	code, _, stderr := runBatchIO(t, "govi", []string{"-s"}, false, "q\n")
	if code != 1 || !strings.Contains(stderr, "only applicable to ex") {
		t.Errorf("govi -s: code %d stderr %q", code, stderr)
	}
}

// TestRun_noFileOpensTempBuffer checks that govi with no file edits a throwaway
// temp buffer (like nvi), not an empty unnamed one.
func TestRun_noFileOpensTempBuffer(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	var eng *engine.Engine
	code, _, _ := captureRun(t, []string{"-n"}, func() {
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
