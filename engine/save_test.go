package engine

import (
	"os"
	"path/filepath"
	"testing"
)

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