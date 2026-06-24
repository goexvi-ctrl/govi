//go:build unix

package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGlobFileNames(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}

	matches, err := globFileNames("f", dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0] != "foo.txt" {
		t.Fatalf("f* = %v", matches)
	}

	matches, err = globFileNames("sub", dir)
	if err != nil || len(matches) != 1 || matches[0] != "sub" {
		t.Fatalf("sub = %v, %v", matches, err)
	}
}

func TestColonFileCompleteSingle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	e, _, _ := newTestEngine(t, "x\n")
	e.SetCwd(dir)
	e.Resize(10, 40)
	drive(e, ":")
	for _, r := range "e he" {
		e.Input(KeyEvent{Rune: r})
	}
	e.Input(KeyEvent{Key: KeyTab})

	got := string(e.scr.colon)
	if got != "e hello.txt" {
		t.Fatalf("colon = %q, want %q", got, "e hello.txt")
	}
}

func TestColonFileCompleteList(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"aaa", "aab", "aac"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	e, _, _ := newTestEngine(t, "x\n")
	e.SetCwd(dir)
	e.Resize(10, 40)
	drive(e, ":")
	e.Input(KeyEvent{Rune: 'e'})
	e.Input(KeyEvent{Rune: ' '})
	e.Input(KeyEvent{Rune: 'a'})
	e.Input(KeyEvent{Key: KeyTab})

	if got := string(e.scr.colon); got != "e aa" {
		t.Fatalf("colon = %q, want %q", got, "e aa")
	}
	if e.scr.pendingOutput == nil {
		t.Fatal("expected completion listing overlay")
	}
}

func TestColonFileCompleteDirSlash(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "proj")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}

	e, _, _ := newTestEngine(t, "x\n")
	e.SetCwd(dir)
	e.Resize(10, 40)
	drive(e, ":")
	for _, r := range "e p" {
		e.Input(KeyEvent{Rune: r})
	}
	e.Input(KeyEvent{Key: KeyTab})

	if got := string(e.scr.colon); got != "e proj/" {
		t.Fatalf("colon = %q, want %q", got, "e proj/")
	}
}