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

func TestColonFileCompleteAmbiguous(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"aaa", "aab", "aac"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	e, fe, _ := newTestEngine(t, "x\n")
	e.SetCwd(dir)
	e.Resize(10, 40)
	drive(e, ":")
	e.Input(KeyEvent{Rune: 'e'})
	e.Input(KeyEvent{Rune: ' '})
	e.Input(KeyEvent{Rune: 'a'})
	e.Input(KeyEvent{Key: KeyTab})

	if got := string(e.scr.colon); got != "e a" {
		t.Fatalf("colon = %q, want %q", got, "e a")
	}
	if fe.bells != 1 {
		t.Fatalf("bells = %d, want 1", fe.bells)
	}
	if e.scr.pendingOutput != nil {
		t.Fatal("ambiguous completion should not show listing overlay")
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

func TestJoinDisplayPathRoot(t *testing.T) {
	if got := joinDisplayPath("/", "tmp"); got != "/tmp" {
		t.Fatalf("joinDisplayPath = %q, want %q", got, "/tmp")
	}
}

func TestGlobFileNamesAbsolute(t *testing.T) {
	matches, err := globFileNames("/tm", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range matches {
		if m == "/tmp" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("/tm* should include /tmp, got %v", matches)
	}
}

func TestColonTabDisplayExpanded(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	drive(e, ":")
	for _, r := range "set tab" {
		e.Input(KeyEvent{Rune: r})
	}
	e.Input(KeyEvent{Rune: '\t'})
	msg, _ := (view{e.scr}).Message()
	want := ":set tab        "
	if msg != want {
		t.Fatalf("msg = %q, want %q", msg, want)
	}
}

func TestColonTabInsertsOutsidePathContext(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.Resize(10, 40)
	drive(e, ":")
	for _, r := range "set tab" {
		e.Input(KeyEvent{Rune: r})
	}
	// macOS GUI sends Tab as a rune (GoviKeyRune), not KeyTab.
	e.Input(KeyEvent{Rune: '\t'})

	if got := string(e.scr.colon); got != "set tab\t" {
		t.Fatalf("colon = %q, want %q", got, "set tab\t")
	}
}

func TestColonTabInsertsBeforePathArg(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.Resize(10, 40)
	drive(e, ":")
	e.Input(KeyEvent{Rune: 'e'})
	e.Input(KeyEvent{Rune: '\t'})

	if got := string(e.scr.colon); got != "e\t" {
		t.Fatalf("colon = %q, want %q", got, "e\t")
	}
}

func TestColonFileCompleteRuneTab(t *testing.T) {
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
	e.Input(KeyEvent{Rune: '\t'})

	if got := string(e.scr.colon); got != "e hello.txt" {
		t.Fatalf("colon = %q, want %q", got, "e hello.txt")
	}
}

func TestColonFileCompleteAbsolute(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.Resize(10, 40)
	drive(e, ":")
	for _, r := range "r /tm" {
		e.Input(KeyEvent{Rune: r})
	}
	e.Input(KeyEvent{Key: KeyTab})

	if got := string(e.scr.colon); got != "r /tmp/" {
		t.Fatalf("colon = %q, want %q", got, "r /tmp/")
	}
}
