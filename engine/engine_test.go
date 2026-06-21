package engine

import (
	"os"
	"path/filepath"
	"testing"

	"govi/engine/undo"
)

// captureFrontend is a headless Frontend that records the last View snapshot.
// It demonstrates that the engine runs with no terminal at all -- the same
// property Phase 8 relies on for GUI embedding.
type captureFrontend struct {
	renders int
	bells   int
	lastCS  ChangeSet
	title   string
}

func (f *captureFrontend) Render(v View, cs ChangeSet) {
	f.renders++
	f.lastCS = cs
}
func (f *captureFrontend) Bell()             { f.bells++ }
func (f *captureFrontend) SetTitle(s string) { f.title = s }

func newTestEngine(t *testing.T, content string) (*Engine, *captureFrontend, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	fe := &captureFrontend{}
	e := New(fe, Options{})
	if err := e.Open(path); err != nil {
		t.Fatal(err)
	}
	e.Resize(10, 40)
	return e, fe, path
}

func (e *Engine) cursor() Pos { return e.scr.cursor }

func TestEngineOpenAndMotion(t *testing.T) {
	e, fe, _ := newTestEngine(t, "alpha\nbeta\ngamma\n")

	v := view{e.scr}
	if v.LineCount() != 3 {
		t.Fatalf("LineCount = %d, want 3", v.LineCount())
	}
	if got := string(v.Line(2).Text); got != "beta" {
		t.Fatalf("Line(2) = %q, want beta", got)
	}
	if e.cursor() != (Pos{Line: 1, Col: 0}) {
		t.Fatalf("initial cursor = %+v", e.cursor())
	}

	r0 := fe.renders
	e.Input(KeyEvent{Rune: 'j'})
	e.Input(KeyEvent{Rune: 'l'})
	e.Input(KeyEvent{Rune: 'l'})
	if e.cursor() != (Pos{Line: 2, Col: 2}) {
		t.Fatalf("after jll cursor = %+v, want line2 col2", e.cursor())
	}
	if fe.renders != r0+3 {
		t.Fatalf("expected 3 renders, got %d", fe.renders-r0)
	}

	// $ goes to last rune of the line; clamped within the line.
	e.Input(KeyEvent{Rune: '$'})
	if e.cursor() != (Pos{Line: 2, Col: 3}) {
		t.Fatalf("after $ cursor = %+v, want col3", e.cursor())
	}

	// k then $ on a 5-rune line.
	e.Input(KeyEvent{Rune: 'k'})
	e.Input(KeyEvent{Rune: '$'})
	if e.cursor() != (Pos{Line: 1, Col: 4}) {
		t.Fatalf("after k$ cursor = %+v, want line1 col4", e.cursor())
	}

	// G to last line.
	e.Input(KeyEvent{Rune: 'G'})
	if e.cursor().Line != 3 {
		t.Fatalf("after G line = %d, want 3", e.cursor().Line)
	}
}

func TestEngineClampAtEdges(t *testing.T) {
	e, _, _ := newTestEngine(t, "ab\n")
	e.Input(KeyEvent{Rune: 'k'}) // already top
	e.Input(KeyEvent{Rune: 'h'}) // already col 0
	if e.cursor() != (Pos{Line: 1, Col: 0}) {
		t.Fatalf("cursor = %+v, want line1 col0", e.cursor())
	}
	for i := 0; i < 10; i++ {
		e.Input(KeyEvent{Rune: 'l'})
	}
	if e.cursor().Col != 1 { // "ab" -> last rune index is 1 in command mode
		t.Fatalf("cursor col = %d, want 1", e.cursor().Col)
	}
}

func TestEngineColonQuitGuard(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.scr.modified = true

	typeColon(e, "q")
	if e.ShouldQuit() {
		t.Fatal(":q with unsaved changes must not quit")
	}
	if _, k := (view{e.scr}).Message(); k != MsgError {
		t.Fatal("expected error message")
	}

	typeColon(e, "q!")
	if !e.ShouldQuit() {
		t.Fatal(":q! must quit")
	}
}

func TestEngineColonWrite(t *testing.T) {
	e, _, path := newTestEngine(t, "one\ntwo\n")
	// Force a buffer change at the store level, then write.
	e.scr.log.Begin(undo.Pos{Line: 2})
	e.scr.log.Set(1, []rune("ONE"))
	e.scr.log.End(undo.Pos{Line: 2})
	e.scr.modified = true

	typeColon(e, "wq")
	if !e.ShouldQuit() {
		t.Fatal(":wq should quit after successful write")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ONE\ntwo\n" {
		t.Fatalf("written file = %q, want %q", string(got), "ONE\ntwo\n")
	}
}

func TestEngineColonModeMessage(t *testing.T) {
	e, _, _ := newTestEngine(t, "hi\n")
	e.Input(KeyEvent{Rune: ':'})
	e.Input(KeyEvent{Rune: 's'})
	e.Input(KeyEvent{Rune: 'e'})
	if txt, _ := (view{e.scr}).Message(); txt != ":se" {
		t.Fatalf("colon line = %q, want :se", txt)
	}
	if (view{e.scr}).Mode() != ModeExColon {
		t.Fatal("expected ModeExColon")
	}
	e.Input(KeyEvent{Key: KeyEscape})
	if (view{e.scr}).Mode() != ModeCommand {
		t.Fatal("Escape should return to command mode")
	}
}

func typeColon(e *Engine, cmd string) {
	e.Input(KeyEvent{Rune: ':'})
	for _, r := range cmd {
		e.Input(KeyEvent{Rune: r})
	}
	e.Input(KeyEvent{Key: KeyEnter})
}
