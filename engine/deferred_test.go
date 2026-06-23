package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIncrement(t *testing.T) {
	viCase(t, "increment", "x = 41;\n", "f4#+", "x = 42;")
	viCase(t, "decrement", "n = 100\n", "f1#-", "n = 99")
	viCase(t, "increment-count", "v 5\n", "f53#+", "v 8")
	viCase(t, "increment-negative", "t -3\n", "f3#+", "t -2")
}

func TestTildeop(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello world\n")
	e.exExecute("set tildeop")
	drive(e, "~w") // toggle-case to end of word
	if bufText(e) != "HELLO world" {
		t.Fatalf("~w with tildeop: got %q", bufText(e))
	}
	drive(e, "~~") // toggle whole line
	if bufText(e) != "hello WORLD" {
		t.Fatalf("~~ with tildeop: got %q", bufText(e))
	}
}

func TestInsertHex(t *testing.T) {
	e, _, _ := newTestEngine(t, "\n")
	drive(e, "i")
	e.Input(KeyEvent{Rune: 'x', Mods: ModCtrl}) // ^X
	drive(e, "41 ")                             // hex 41 = 'A', space terminates
	drive(e, "\x1b")
	if bufText(e) != "A " {
		t.Fatalf("^X hex insert: got %q", bufText(e))
	}
}

func TestNulReplay(t *testing.T) {
	e, _, _ := newTestEngine(t, "\n")
	drive(e, "ifoo\x1b")                        // insert "foo"
	drive(e, "o")                               // open a new line, insert mode
	e.Input(KeyEvent{Rune: '@', Mods: ModCtrl}) // NUL: replay "foo"
	drive(e, "\x1b")
	if bufText(e) != "foo\nfoo" {
		t.Fatalf("NUL replay: got %q", bufText(e))
	}
}

func TestAlternateFile(t *testing.T) {
	dir := t.TempDir()
	a := writeTemp(t, dir, "a.txt", "AAA\n")
	b := writeTemp(t, dir, "b.txt", "BBB\n")
	e := New(&captureFrontend{}, Options{})
	e.OpenArgs([]string{a})
	e.Resize(10, 40)
	e.exExecute("edit " + b)
	if bufText(e) != "BBB" {
		t.Fatalf("after edit b: %q", bufText(e))
	}
	e.Input(KeyEvent{Rune: '^', Mods: ModCtrl}) // ^^ back to a
	if bufText(e) != "AAA" {
		t.Fatalf("after ^^: %q", bufText(e))
	}
	e.Input(KeyEvent{Rune: '^', Mods: ModCtrl}) // ^^ back to b
	if bufText(e) != "BBB" {
		t.Fatalf("after ^^ again: %q", bufText(e))
	}
}

func TestShellFilter(t *testing.T) {
	exCase(t, "sort", "banana\napple\ncherry\n", []string{"%!sort"}, "apple\nbanana\ncherry")
	exCase(t, "filter-range", "3\n1\n2\nx\n", []string{"1,3!sort"}, "1\n2\n3\nx")
	exCase(t, "filter-error-output", "package main\n\nnot go\n", []string{"%!sh -c 'printf \"%s\\n\" \"1:3: expected declaration, found not\" >&2; exit 2'"}, "1:3: expected declaration, found not")
}

func TestViFilterOperator(t *testing.T) {
	e, _, _ := newTestEngine(t, "c\nb\na\n")
	drive(e, "!G") // filter to end of file: nvi prompts with "!"
	if msg, _ := (view{e.scr}).Message(); msg != "!" {
		t.Fatalf("filter prompt = %q, want !", msg)
	}
	drive(e, "sort\r") // type the command
	if bufText(e) != "a\nb\nc" {
		t.Fatalf("!G sort: got %q", bufText(e))
	}
}

func TestTags(t *testing.T) {
	dir := t.TempDir()
	src := writeTemp(t, dir, "src.txt", "package x\n\nfunc Foo() {}\n\nfunc Bar() {}\n")
	// ctags-style tags file: name<TAB>file<TAB>/pattern/
	tags := "Bar\t" + src + "\t/^func Bar/\nFoo\t" + src + "\t/^func Foo/\n"
	tagsPath := filepath.Join(dir, "tags")
	if err := os.WriteFile(tagsPath, []byte(tags), 0o644); err != nil {
		t.Fatal(err)
	}

	e := New(&captureFrontend{}, Options{})
	e.OpenArgs([]string{src})
	e.Resize(10, 40)
	e.exExecute("set tags=" + tagsPath)

	if err := e.exExecute("tag Bar"); err != nil {
		t.Fatal(err)
	}
	if e.scr.cursor.Line != 5 {
		t.Fatalf("tag Bar -> line %d, want 5", e.scr.cursor.Line)
	}
	if err := e.exExecute("tag Foo"); err != nil {
		t.Fatal(err)
	}
	if e.scr.cursor.Line != 3 {
		t.Fatalf("tag Foo -> line %d, want 3", e.scr.cursor.Line)
	}
	e.Input(KeyEvent{Rune: 't', Mods: ModCtrl}) // ^T pop -> back to Bar location (line 5)
	if e.scr.cursor.Line != 5 {
		t.Fatalf("^T -> line %d, want 5", e.scr.cursor.Line)
	}
}

func TestExModeQ(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\ntwo\nthree\n")
	drive(e, "Q") // enter ex mode
	if (view{e.scr}).Mode() != ModeExText {
		t.Fatal("Q should enter ex mode")
	}
	// Run a substitute in ex mode.
	drive(e, "%s/o/0/g\r")
	if bufText(e) != "0ne\ntw0\nthree" {
		t.Fatalf("ex-mode substitute: got %q", bufText(e))
	}
	// Print a line; it should appear in the transcript.
	drive(e, "2p\r")
	tr := (view{e.scr}).ExTranscript()
	if len(tr) == 0 || tr[len(tr)-1] != "tw0" {
		t.Fatalf("ex-mode print transcript = %v", tr)
	}
	// Return to vi mode.
	drive(e, "visual\r")
	if (view{e.scr}).Mode() != ModeCommand {
		t.Fatal(":visual should return to vi mode")
	}
}
