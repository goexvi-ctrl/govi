package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
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

func TestZQQuitsNoWrite(t *testing.T) {
	dir := t.TempDir()
	a := writeTemp(t, dir, "a.txt", "hello\n")
	e := New(&captureFrontend{}, Options{})
	e.OpenArgs([]string{a})
	e.Resize(10, 40)
	drive(e, "xx") // modify
	drive(e, "ZQ") // quit without writing
	if !e.ShouldQuit() {
		t.Fatal("ZQ should quit")
	}
	got, _ := os.ReadFile(a)
	if string(got) != "hello\n" {
		t.Fatalf("ZQ should not write; file = %q", string(got))
	}
}
