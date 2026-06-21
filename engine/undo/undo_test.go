package undo

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"govi/engine/buffer"
)

func snapshot(ls buffer.LineStore) []string {
	out := make([]string, 0, ls.Lines())
	for i := int64(1); i <= ls.Lines(); i++ {
		ln, _ := ls.Get(i)
		out = append(out, string(ln))
	}
	return out
}

func sameLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func rs(s string) []rune { return []rune(s) }

func TestUndoRedoBasic(t *testing.T) {
	store := buffer.NewMemFromLines([][]rune{rs("one"), rs("two"), rs("three")})
	l := New(store)

	s0 := snapshot(store)

	l.Begin(Pos{Line: 1})
	l.Set(2, rs("TWO"))
	l.Delete(3)
	l.Append(1, rs("inserted"))
	l.End(Pos{Line: 2})
	s1 := snapshot(store)

	if sameLines(s0, s1) {
		t.Fatal("change set had no effect")
	}

	cur, ok := l.Undo()
	if !ok || cur.Line != 1 {
		t.Fatalf("undo cursor = %+v ok=%v, want line 1", cur, ok)
	}
	if got := snapshot(store); !sameLines(got, s0) {
		t.Fatalf("after undo: got %q, want %q", got, s0)
	}

	cur, ok = l.Redo()
	if !ok || cur.Line != 2 {
		t.Fatalf("redo cursor = %+v ok=%v, want line 2", cur, ok)
	}
	if got := snapshot(store); !sameLines(got, s1) {
		t.Fatalf("after redo: got %q, want %q", got, s1)
	}
}

func TestUndoMultiLevel(t *testing.T) {
	store := buffer.NewMem()
	l := New(store)

	var states [][]string
	states = append(states, snapshot(store))
	for i := 0; i < 5; i++ {
		l.Begin(Pos{Line: int64(i + 1)})
		l.Append(store.Lines(), rs(fmt.Sprintf("line%d", i)))
		l.End(Pos{Line: int64(i + 1)})
		states = append(states, snapshot(store))
	}

	// Undo all the way back, checking each intermediate state.
	for i := 5; i >= 1; i-- {
		if got := snapshot(store); !sameLines(got, states[i]) {
			t.Fatalf("state %d: got %q want %q", i, got, states[i])
		}
		if _, ok := l.Undo(); !ok {
			t.Fatalf("undo %d failed", i)
		}
	}
	if got := snapshot(store); !sameLines(got, states[0]) {
		t.Fatalf("fully undone: got %q want %q", got, states[0])
	}
	if _, ok := l.Undo(); ok {
		t.Fatal("undo past beginning should fail")
	}

	// Redo all the way forward.
	for i := 1; i <= 5; i++ {
		if _, ok := l.Redo(); !ok {
			t.Fatalf("redo %d failed", i)
		}
		if got := snapshot(store); !sameLines(got, states[i]) {
			t.Fatalf("redo state %d: got %q want %q", i, got, states[i])
		}
	}
}

func TestNewChangeClearsRedo(t *testing.T) {
	store := buffer.NewMemFromLines([][]rune{rs("a")})
	l := New(store)

	l.Begin(Pos{})
	l.Set(1, rs("b"))
	l.End(Pos{})
	l.Undo() // back to "a"
	if !l.CanRedo() {
		t.Fatal("expected redo available")
	}

	l.Begin(Pos{})
	l.Set(1, rs("c"))
	l.End(Pos{})
	if l.CanRedo() {
		t.Fatal("new change must clear redo stack")
	}
}

// TestUndoRoundTripRandom drives random change sets, then undoes all of them
// and checks the store returns exactly to its initial contents.
func TestUndoRoundTripRandom(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 5))
	store := buffer.NewMemFromLines(parseSeed("a\nb\nc\nd\n"))
	initial := snapshot(store)
	l := New(store)

	changes := 0
	for i := 0; i < 300; i++ {
		l.Begin(Pos{})
		ops := rng.IntN(4) + 1
		for j := 0; j < ops; j++ {
			n := store.Lines()
			switch rng.IntN(3) {
			case 0:
				l.Insert(int64(rng.IntN(int(n)+1))+1, rs(fmt.Sprintf("i%d", i)))
			case 1:
				if n > 0 {
					l.Set(int64(rng.IntN(int(n)))+1, rs(fmt.Sprintf("s%d", i)))
				}
			case 2:
				if n > 0 {
					l.Delete(int64(rng.IntN(int(n))) + 1)
				}
			}
		}
		l.End(Pos{})
		changes++
	}

	for l.CanUndo() {
		l.Undo()
	}
	if got := snapshot(store); !sameLines(got, initial) {
		t.Fatalf("after undoing %d change sets: got %q want %q", changes, got, initial)
	}
}

func parseSeed(s string) [][]rune {
	out := [][]rune{}
	cur := []rune{}
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = []rune{}
			continue
		}
		cur = append(cur, r)
	}
	return out
}
