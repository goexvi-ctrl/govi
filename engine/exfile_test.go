package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewFileMessage(t *testing.T) {
	e, _, _ := newTestEngine(t, "")
	e.Resize(10, 200) // wide enough that the temp path is not truncated
	p := filepath.Join(t.TempDir(), "brandnew")
	if err := e.Open(p); err != nil {
		t.Fatal(err)
	}
	want := p + ": new file: line 1"
	if e.scr.msg != want {
		t.Fatalf("new-file message = %q, want %q", e.scr.msg, want)
	}
}

func TestExistingFileMessage(t *testing.T) {
	e, _, _ := newTestEngine(t, "")
	e.Resize(10, 200)
	p := writeTemp(t, t.TempDir(), "have", "ab\ncd\n") // 2 lines, 6 bytes
	if err := e.Open(p); err != nil {
		t.Fatal(err)
	}
	want := p + ": 2 lines, 6 characters"
	if e.scr.msg != want {
		t.Fatalf("existing-file message = %q, want %q", e.scr.msg, want)
	}
}

func TestStatusNameTruncated(t *testing.T) {
	e, _, _ := newTestEngine(t, "")
	e.Resize(10, 30) // narrow: the name must be truncated to fit
	p := filepath.Join(t.TempDir(), "deeply/nested/long/path/file")
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := e.Open(p); err != nil {
		t.Fatal(err)
	}
	msg, _ := (view{e.scr}).Message() // truncation happens at render time
	if !strings.HasPrefix(msg, "...") {
		t.Fatalf("expected leading \"...\"; msg = %q", msg)
	}
	if !strings.HasSuffix(msg, ": new file: line 1") {
		t.Fatalf("trailing message lost; msg = %q", msg)
	}
	if n := len([]rune(msg)); n > 30 {
		t.Fatalf("status line = %d cols, want <= 30: %q", n, msg)
	}

	// Widening the terminal must reveal the full name (truncation honors the
	// live width, not the width at open time).
	e.Resize(10, 300)
	wide, _ := (view{e.scr}).Message()
	if strings.HasPrefix(wide, "...") || !strings.Contains(wide, p) {
		t.Fatalf("after widening, expected full path; msg = %q", wide)
	}
}

func TestTemporaryBufferWarnsOnExit(t *testing.T) {
	e, _, _ := newTestEngine(t, "")
	e.Resize(10, 40)
	e.SetTemporary()
	drive(e, "iabc\x1b") // modify the buffer

	if err := e.exExecute("wq"); err == nil {
		t.Fatal(":wq on a temporary buffer should warn, not write/quit")
	}
	if e.quit {
		t.Fatal(":wq must not quit a temporary buffer")
	}
	if err := e.exExecute("q"); err == nil {
		t.Fatal(":q on a modified temporary buffer should warn")
	}

	// :wq <real file> writes a real file, adopts the name, and quits.
	p := filepath.Join(t.TempDir(), "real.txt")
	if err := e.exExecute("wq " + p); err != nil {
		t.Fatalf(":wq <file> should succeed: %v", err)
	}
	if !e.quit {
		t.Fatal(":wq <file> should have quit")
	}
}

func TestTemporaryBufferWarnsAfterWrite(t *testing.T) {
	e, _, _ := newTestEngine(t, "")
	e.Resize(10, 40)
	e.SetTemporary()
	if e.TempDiscardPending() {
		t.Fatal("an empty temporary buffer should not warn")
	}
	drive(e, "iabc\x1b")
	if !e.TempDiscardPending() {
		t.Fatal("a temporary buffer with content should warn")
	}
	// Simulate :w clearing the modified flag (it only writes the throwaway temp):
	// the warning must persist because the content is still discarded on exit.
	e.scr.modified = false
	if !e.TempDiscardPending() {
		t.Fatal("temp buffer should still warn after :w (content discarded on exit)")
	}
	if err := e.exExecute("q"); err == nil || !strings.Contains(err.Error(), "temporary") {
		t.Fatalf(":q on a written temp buffer should give the temporary warning, got %v", err)
	}
	if e.quit {
		t.Fatal(":q must not quit a temp buffer that still holds content")
	}
}

func TestTemporaryBufferForceQuitDiscards(t *testing.T) {
	e, _, _ := newTestEngine(t, "")
	e.SetTemporary()
	drive(e, "ixyz\x1b")
	if err := e.exExecute("q!"); err != nil {
		t.Fatalf(":q! should discard a temporary buffer: %v", err)
	}
	if !e.quit {
		t.Fatal(":q! should have quit")
	}
}

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestExBackup covers CORNERS A-4: the backup option copies a file's current
// contents to a backup before :w overwrites it (nvi file_backup). '%' expands to
// the file name; a leading 'N' versions the backup. Verified against nvi (which
// creates f.txt.bak holding the pre-write content).
func TestExBackup(t *testing.T) {
	dir := t.TempDir()
	p := writeTemp(t, dir, "f.txt", "original\n")
	e := New(&captureFrontend{}, Options{})
	if err := e.OpenArgs([]string{p}); err != nil {
		t.Fatal(err)
	}
	e.Resize(10, 40)
	e.exExecute("set backup=%.bak")
	drive(e, "cwCHANGED\x1b")
	if err := e.exExecute("w"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if bak, err := os.ReadFile(p + ".bak"); err != nil {
		t.Fatalf("no backup created: %v", err)
	} else if string(bak) != "original\n" {
		t.Errorf("backup content = %q, want %q", bak, "original\n")
	}
	if cur, _ := os.ReadFile(p); string(cur) != "CHANGED\n" {
		t.Errorf("file content = %q, want %q", cur, "CHANGED\n")
	}
	// The backup file must be private (0600), like nvi's.
	if info, err := os.Stat(p + ".bak"); err == nil && info.Mode().Perm()&0o077 != 0 {
		t.Errorf("backup perms = %v, want no group/other access", info.Mode().Perm())
	}

	// Versioned backups: a leading N appends an incrementing number.
	e.exExecute("set backup=N%.bak")
	drive(e, "cwAGAIN\x1b")
	e.exExecute("w")
	drive(e, "cwMORE\x1b")
	e.exExecute("w")
	if _, err := os.Stat(p + ".bak1"); err != nil {
		t.Errorf("versioned backup .bak1 missing: %v", err)
	}
	if _, err := os.Stat(p + ".bak2"); err != nil {
		t.Errorf("versioned backup .bak2 missing: %v", err)
	}
}

func TestMultiFileNavigation(t *testing.T) {
	dir := t.TempDir()
	a := writeTemp(t, dir, "a.txt", "file a\n")
	b := writeTemp(t, dir, "b.txt", "file b\n")
	c := writeTemp(t, dir, "c.txt", "file c\n")

	e := New(&captureFrontend{}, Options{})
	if err := e.OpenArgs([]string{a, b, c}); err != nil {
		t.Fatal(err)
	}
	e.Resize(10, 40)
	if bufText(e) != "file a" {
		t.Fatalf("initial: %q", bufText(e))
	}

	if err := e.exExecute("next"); err != nil {
		t.Fatal(err)
	}
	if bufText(e) != "file b" {
		t.Fatalf("after :next: %q", bufText(e))
	}
	e.exExecute("n")
	if bufText(e) != "file c" {
		t.Fatalf("after :n: %q", bufText(e))
	}
	if err := e.exExecute("n"); err == nil {
		t.Fatal(":n past end should error")
	}
	e.exExecute("rewind")
	if bufText(e) != "file a" {
		t.Fatalf("after :rewind: %q", bufText(e))
	}
	e.exExecute("prev")
	if bufText(e) != "file a" { // already first; prev errors, buffer unchanged
		t.Fatalf("after :prev at first: %q", bufText(e))
	}
}

func TestEditPreservesOptions(t *testing.T) {
	dir := t.TempDir()
	a := writeTemp(t, dir, "a.txt", "AAA\n")
	b := writeTemp(t, dir, "b.txt", "BBB\n")

	e := New(&captureFrontend{}, Options{})
	e.OpenArgs([]string{a})
	e.Resize(10, 40)
	e.exExecute("set ai sw=3")
	e.exExecute("map X dd")

	if err := e.exExecute("edit " + b); err != nil {
		t.Fatal(err)
	}
	if bufText(e) != "BBB" {
		t.Fatalf("after :edit: %q", bufText(e))
	}
	// Options and maps must survive the file switch.
	if !e.scr.opts.Bool("autoindent") || e.scr.opts.Int("shiftwidth") != 3 {
		t.Fatal("options not preserved across :edit")
	}
	if _, ok := e.scr.maps.command["X"]; !ok {
		t.Fatal("maps not preserved across :edit")
	}
}

func TestExFileStatus(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "foo.txt", "a\nb\nc\n")
	e, _, _ := newTestEngine(t, "")
	e.Resize(10, 200) // wide: don't truncate the temp path in the status line
	if err := e.Open(path); err != nil {
		t.Fatal(err)
	}
	drive(e, "2G")
	if err := e.exExecute("f"); err != nil {
		t.Fatal(err)
	}
	msg, kind := (view{e.scr}).Message()
	if kind != MsgInfo {
		t.Fatalf("kind = %v", kind)
	}
	if !strings.Contains(msg, path) || !strings.Contains(msg, "unmodified") || !strings.Contains(msg, "line 2 of 3") {
		t.Fatalf(":f status = %q", msg)
	}
}

func TestExFileRename(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "old.txt", "hi\n")
	e, fe, _ := newTestEngine(t, "")
	e.Resize(10, 200)
	if err := e.Open(path); err != nil {
		t.Fatal(err)
	}
	newPath := filepath.Join(dir, "new.txt")
	if err := e.exExecute("f " + newPath); err != nil {
		t.Fatal(err)
	}
	if e.scr.name != newPath {
		t.Fatalf("name = %q, want %q", e.scr.name, newPath)
	}
	if e.scr.altFile != path {
		t.Fatalf("altFile = %q, want %q", e.scr.altFile, path)
	}
	msg, _ := (view{e.scr}).Message()
	if !strings.Contains(msg, "name changed") {
		t.Fatalf(":f rename status = %q", msg)
	}
	if fe.title != "new.txt" {
		t.Fatalf("title = %q", fe.title)
	}
}

func TestEditModifiedGuard(t *testing.T) {
	dir := t.TempDir()
	a := writeTemp(t, dir, "a.txt", "AAA\n")
	b := writeTemp(t, dir, "b.txt", "BBB\n")
	e := New(&captureFrontend{}, Options{})
	e.OpenArgs([]string{a})
	e.Resize(10, 40)
	drive(e, "x") // modify

	if err := e.exExecute("edit " + b); err == nil {
		t.Fatal(":edit with unsaved changes should error")
	}
	if err := e.exExecute("edit! " + b); err != nil {
		t.Fatal(":edit! should override")
	}
	if bufText(e) != "BBB" {
		t.Fatalf("after :edit!: %q", bufText(e))
	}
}

// TestEditAlternateFileExpansion checks that ":e #" re-edits the alternate file
// (the previously edited one) instead of opening a file literally named "#".
func TestEditAlternateFileExpansion(t *testing.T) {
	dir := t.TempDir()
	a := writeTemp(t, dir, "a.txt", "AAA\n")
	b := writeTemp(t, dir, "b.txt", "BBB\n")
	e := New(&captureFrontend{}, Options{})
	e.OpenArgs([]string{a, b})
	e.Resize(10, 40)

	if err := e.exExecute("n"); err != nil { // :n -> b.txt, alternate becomes a.txt
		t.Fatal(err)
	}
	if bufText(e) != "BBB" {
		t.Fatalf("after :n: %q", bufText(e))
	}
	if e.scr.altFile != a {
		t.Fatalf("altFile = %q, want %q", e.scr.altFile, a)
	}
	if err := e.exExecute("e #"); err != nil { // :e # -> back to a.txt
		t.Fatal(err)
	}
	if e.scr.name != a {
		t.Fatalf(":e # opened %q, want %q", e.scr.name, a)
	}
	if bufText(e) != "AAA" {
		t.Fatalf("after :e #: %q", bufText(e))
	}
}

// TestWriteCurrentFileExpansion checks that ":w %" writes to the current file.
func TestWriteCurrentFileExpansion(t *testing.T) {
	dir := t.TempDir()
	a := writeTemp(t, dir, "a.txt", "hello\n")
	e := New(&captureFrontend{}, Options{})
	e.OpenArgs([]string{a})
	e.Resize(10, 40)
	drive(e, "x") // "ello", modified
	if err := e.exExecute("w %"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(a)
	if string(got) != "ello\n" {
		t.Fatalf(":w %% wrote %q, want %q", string(got), "ello\n")
	}
}

// TestEditGlobPrefixUnique checks that ":e prefix*" with a single matching file
// opens it via internal prefix completion (nvi argv_lexp), no shell involved.
func TestEditGlobPrefixUnique(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "unique.txt", "U\n")
	writeTemp(t, dir, "other.dat", "O\n")
	e, _, _ := newTestEngine(t, "")
	e.Resize(10, 200)
	if err := e.exExecute("e " + filepath.Join(dir, "uniq") + "*"); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "unique.txt")
	if e.scr.name != want {
		t.Fatalf(":e uniq* opened %q, want %q", e.scr.name, want)
	}
	if bufText(e) != "U" {
		t.Fatalf("buffer = %q", bufText(e))
	}
}

// TestEditGlobPrefixNoMatch checks nvi's "Shell expansion failed" when a trailing
// '*' prefix matches nothing.
func TestEditGlobPrefixNoMatch(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "a.txt", "A\n")
	e, _, _ := newTestEngine(t, "")
	e.Resize(10, 200)
	err := e.exExecute("e " + filepath.Join(dir, "zzz") + "*")
	if err == nil || !strings.Contains(err.Error(), "Shell expansion failed") {
		t.Fatalf(":e zzz* err = %v, want Shell expansion failed", err)
	}
}

// TestEditGlobPrefixTooMany checks that a prefix matching several files is a usage
// error for :edit, which takes exactly one file (nvi).
func TestEditGlobPrefixTooMany(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "aa.txt", "1\n")
	writeTemp(t, dir, "ab.txt", "2\n")
	e, _, _ := newTestEngine(t, "")
	e.Resize(10, 200)
	err := e.exExecute("e " + filepath.Join(dir, "a") + "*")
	if err == nil || !strings.HasPrefix(err.Error(), "Usage:") {
		t.Fatalf(":e a* err = %v, want Usage", err)
	}
}

// TestWriteGlobTooMany checks the "expanded into too many file names" error for
// :w when the shell-forked glob yields several names (nvi ex_write.c).
func TestWriteGlobTooMany(t *testing.T) {
	dir := t.TempDir()
	a := writeTemp(t, dir, "a.txt", "A\n")
	writeTemp(t, dir, "b.txt", "B\n")
	e := New(&captureFrontend{}, Options{})
	e.OpenArgs([]string{a})
	e.Resize(10, 40)
	if err := e.exExecute("set shell=/bin/sh"); err != nil { // deterministic glob
		t.Fatal(err)
	}
	pat := filepath.Join(dir, "*.txt")
	err := e.exExecute("w " + pat)
	if err == nil || !strings.Contains(err.Error(), "expanded into too many file names") {
		t.Fatalf(":w %s err = %v, want too-many-file-names", pat, err)
	}
}

func TestZZWritesAndQuits(t *testing.T) {
	dir := t.TempDir()
	a := writeTemp(t, dir, "a.txt", "hello\n")
	e := New(&captureFrontend{}, Options{})
	e.OpenArgs([]string{a})
	e.Resize(10, 40)
	drive(e, "x")  // delete 'h' -> "ello", modified
	drive(e, "ZZ") // write and quit
	if !e.ShouldQuit() {
		t.Fatal("ZZ should quit")
	}
	got, _ := os.ReadFile(a)
	if string(got) != "ello\n" {
		t.Fatalf("ZZ wrote %q, want %q", string(got), "ello\n")
	}
}

// nvi has only ZZ; ZQ is not an nvi command (it bells), so govi must not quit on
// it (matching the oracle, verified via goterm).
func TestZQIsNotACommand(t *testing.T) {
	dir := t.TempDir()
	a := writeTemp(t, dir, "a.txt", "hello\n")
	e := New(&captureFrontend{}, Options{})
	e.OpenArgs([]string{a})
	e.Resize(10, 40)
	drive(e, "ZQ")
	if e.ShouldQuit() {
		t.Fatal("ZQ is not an nvi command; it must not quit")
	}
}
