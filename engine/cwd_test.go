//go:build unix

package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExCdAndRead(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	dataFile := filepath.Join(sub, "data.txt")
	if err := os.WriteFile(dataFile, []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	e, _, _ := newTestEngine(t, "x\n")
	e.SetCwd(dir)
	if err := e.exExecute("cd sub"); err != nil {
		t.Fatal(err)
	}
	if got := e.Cwd(); got != sub {
		t.Fatalf("cwd = %q, want %q", got, sub)
	}
	if err := e.exExecute("r data.txt"); err != nil {
		t.Fatal(err)
	}
	runes := e.scr.lineRunes(1)
	if len(runes) != 1 || string(runes[0]) != "x" {
		t.Fatalf("line 1 = %q", runes)
	}
	if got := string(e.scr.lineRunes(2)); got != "hello" {
		t.Fatalf("line 2 = %q, want hello", got)
	}
}

func TestExCdHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	e, _, _ := newTestEngine(t, "\n")
	e.SetCwd("/tmp")
	if err := e.exExecute("cd"); err != nil {
		t.Fatal(err)
	}
	if e.Cwd() != home {
		t.Fatalf("cwd = %q, want %q", e.Cwd(), home)
	}
}

func TestExCdModifiedGuard(t *testing.T) {
	e, _, _ := newTestEngine(t, "changed\n")
	e.scr.name = "relative.txt"
	e.scr.modified = true
	e.SetCwd("/tmp")
	if err := e.exExecute("cd /var"); err == nil {
		t.Fatal("cd should fail on modified buffer with relative name")
	}
}

func TestResolvePathAbsolute(t *testing.T) {
	e := newBareEngine(t)
	e.SetCwd("/tmp")
	if got := e.resolvePath("/etc/passwd"); got != "/etc/passwd" {
		t.Fatalf("resolvePath = %q", got)
	}
}

func TestCanonicalPathRelative(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	e := newBareEngine(t)
	e.SetCwd(sub)
	want := filepath.Join(sub, "foo.txt")
	if got := e.canonicalPath("foo.txt"); got != want {
		t.Fatalf("canonicalPath = %q, want %q", got, want)
	}
}
