package engine

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestModifiedDuringInsert(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello\n")
	drive(e, "ix") // insert mode, type 'x' (still in insert)
	v := view{e.scr}
	if !v.Modified() {
		t.Fatal("buffer should be modified while insert mode has edits")
	}
	if err := e.exExecute("quit"); err == nil {
		t.Fatal(":quit should refuse while insert has unsaved edits")
	}
	drive(e, "\x1b") // leave insert; modified flag is set explicitly too
	if !v.Modified() {
		t.Fatal("buffer should stay modified after leaving insert")
	}
}

func TestSavePreservesFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode preservation is Unix-specific")
	}
	e, _, path := newTestEngine(t, "hello\n")
	const wantMode = 0o755
	if err := os.Chmod(path, wantMode); err != nil {
		t.Fatal(err)
	}
	drive(e, "$i!\x1b") // append '!'
	if err := e.Save(""); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != wantMode&os.ModePerm {
		t.Fatalf("mode after save = %o, want %o", info.Mode().Perm(), wantMode&os.ModePerm)
	}
}

func TestSaveAsFromTemp(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello\n")
	dir := t.TempDir()
	tempPath := filepath.Join(dir, "vi.abc123")
	if err := os.WriteFile(tempPath, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.scr.name = tempPath
	e.SetTemporary()
	outPath := filepath.Join(dir, "saved.txt")
	if err := e.SaveAs(outPath); err != nil {
		t.Fatal(err)
	}
	if e.scr.name != outPath {
		t.Fatalf("name = %q, want %q", e.scr.name, outPath)
	}
	if e.IsTemporary() {
		t.Fatal("buffer should no longer be temporary after SaveAs")
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("file = %q", data)
	}
	// The throwaway temp on disk is left unchanged for the host to delete later.
	got, err := os.ReadFile(tempPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old\n" {
		t.Fatalf("temp file = %q, want unchanged", got)
	}
}

func TestSaveUntitledAdoptsName(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello\n")
	e.scr.name = ""
	if err := e.Save(""); err == nil {
		t.Fatal("Save with no name should fail")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	if err := e.Save(path); err != nil {
		t.Fatal(err)
	}
	if e.scr.name != path {
		t.Fatalf("name = %q, want %q", e.scr.name, path)
	}
	if e.scr.modified {
		t.Fatal("buffer should be clean after save")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("file = %q", data)
	}
}
