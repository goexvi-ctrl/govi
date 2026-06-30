package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sampleC is a small C program with a couple of distinct symbols cscope can
// index: a definition (add), a global (g_count), and uses of both.
const sampleC = `int g_count;

int add(int a, int b) {
	return a + b;
}

int main(void) {
	g_count = add(1, 2);
	return g_count;
}
`

// buildCscopeDB writes sample.c into a fresh temp dir and builds a cscope
// database there, returning the directory. It skips the test when cscope is not
// installed.
func buildCscopeDB(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("cscope"); err != nil {
		t.Skip("cscope not installed")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo.c"), []byte(sampleC), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("cscope", "-b", "-k", "-f", "cscope.out", "foo.c")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cscope build failed: %v\n%s", err, out)
	}
	return dir
}

// cscopeEngine builds a database, opens foo.c, and adds the cscope connection.
func cscopeEngine(t *testing.T) (*Engine, string) {
	t.Helper()
	dir := buildCscopeDB(t)
	e := New(&captureFrontend{}, Options{})
	t.Cleanup(func() { e.Close() })
	if err := e.Open(filepath.Join(dir, "foo.c")); err != nil {
		t.Fatal(err)
	}
	e.Resize(23, 80)
	if err := e.RunEx("cscope add " + dir); err != nil {
		t.Fatalf("cscope add: %v", err)
	}
	if len(e.cscopes) != 1 {
		t.Fatalf("connections = %d, want 1", len(e.cscopes))
	}
	return e, dir
}

func TestCscopeConvMatches(t *testing.T) {
	// The converted pattern must match the exact source line, including with
	// extra interior whitespace standing in for the single blanks cscope stores.
	re, err := compileCscopePattern(cscopeConv("int add(int a, int b) {"))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, line := range []string{
		"int add(int a, int b) {",
		"int   add(int  a,  int  b) {", // flexible whitespace
		"\tint add(int a, int b) {",    // leading indent
	} {
		if _, ok := re.MatchAt([]rune(line), 0); !ok {
			t.Errorf("pattern did not match %q", line)
		}
	}
	if _, ok := re.MatchAt([]rune("int subtract(int a, int b) {"), 0); ok {
		t.Errorf("pattern wrongly matched a different line")
	}
}

func TestCscopeFindDefinition(t *testing.T) {
	e, _ := cscopeEngine(t)
	// Move away from the definition so the jump is observable.
	e.scr.cursor = Pos{Line: 8, Col: 0}
	if err := e.RunEx("cscope find g add"); err != nil {
		t.Fatalf("find: %v", err)
	}
	// add() is defined on line 3; the cursor lands on its first non-blank.
	if e.scr.cursor.Line != 3 {
		t.Fatalf("cursor line = %d, want 3", e.scr.cursor.Line)
	}
	// ^T returns to the saved location.
	if err := e.tagPop(); err != nil {
		t.Fatalf("tagpop: %v", err)
	}
	if e.scr.cursor.Line != 8 {
		t.Fatalf("after ^T cursor line = %d, want 8", e.scr.cursor.Line)
	}
}

func TestCscopeFindMultipleAndNav(t *testing.T) {
	e, _ := cscopeEngine(t)
	// g_count is used on several lines; "s" finds all of them.
	if err := e.RunEx("cscope find s g_count"); err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(e.scr.tagMatches) < 2 {
		t.Fatalf("matches = %d, want >= 2", len(e.scr.tagMatches))
	}
	first := e.scr.cursor.Line
	if err := e.RunEx("tagnext"); err != nil {
		t.Fatalf("tagnext: %v", err)
	}
	if e.scr.cursor.Line == first {
		t.Errorf("tagnext did not move from line %d", first)
	}
	if err := e.RunEx("tagprev"); err != nil {
		t.Fatalf("tagprev: %v", err)
	}
	if e.scr.cursor.Line != first {
		t.Errorf("tagprev did not return to line %d (got %d)", first, e.scr.cursor.Line)
	}
	// Stepping before the first match is an error.
	if err := e.RunEx("tagprev"); err == nil {
		t.Errorf("tagprev at first match should error")
	}
}

func TestCscopeFindNoConnections(t *testing.T) {
	e := New(&captureFrontend{}, Options{})
	t.Cleanup(func() { e.Close() })
	e.Resize(23, 80)
	err := e.RunEx("cscope find g add")
	if err == nil || !strings.Contains(err.Error(), "No cscope connections") {
		t.Fatalf("err = %v, want no-connections", err)
	}
}

func TestCscopeUnknownSearchType(t *testing.T) {
	e, _ := cscopeEngine(t)
	err := e.RunEx("cscope find z add")
	if err == nil || !strings.Contains(err.Error(), "unknown search type") {
		t.Fatalf("err = %v, want unknown-search-type", err)
	}
}

func TestCscopeNoMatches(t *testing.T) {
	e, _ := cscopeEngine(t)
	if err := e.RunEx("cscope find g nonexistent_symbol_xyz"); err != nil {
		t.Fatalf("find: %v", err)
	}
	if e.scr.msg != "No matches for query" {
		t.Fatalf("msg = %q, want %q", e.scr.msg, "No matches for query")
	}
}

func TestCscopeDisplayAndKill(t *testing.T) {
	e, dir := cscopeEngine(t)
	if err := e.RunEx("display connections"); err != nil {
		t.Fatalf("display: %v", err)
	}
	joined := strings.Join(e.scr.pendingOutput, "\n")
	if !strings.Contains(joined, dir) {
		t.Fatalf("display connections = %q, want it to name %q", joined, dir)
	}
	if err := e.RunEx("cscope kill 1"); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if len(e.cscopes) != 0 {
		t.Fatalf("connections after kill = %d, want 0", len(e.cscopes))
	}
	// Killing a nonexistent session errors.
	if err := e.RunEx("cscope kill 1"); err == nil {
		t.Errorf("kill of empty list should error")
	}
}

func TestCscopeHelp(t *testing.T) {
	e := New(&captureFrontend{}, Options{})
	t.Cleanup(func() { e.Close() })
	e.Resize(23, 80)
	if err := e.RunEx("cscope help"); err != nil {
		t.Fatalf("help: %v", err)
	}
	joined := strings.Join(e.scr.pendingOutput, "\n")
	for _, want := range []string{"cscope commands:", "add", "find", "help", "kill", "reset"} {
		if !strings.Contains(joined, want) {
			t.Errorf("help output missing %q:\n%s", want, joined)
		}
	}
}
