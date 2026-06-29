package undo

import (
	"testing"

	"govi/engine/buffer"
)

// TestSetKnownMatchesSet checks SetKnown produces the same buffer state and
// undo/redo behavior as Set when given the correct before-image.
func TestSetKnownMatchesSet(t *testing.T) {
	mk := func() *Log {
		return New(buffer.NewMemFromLines([][]rune{rs("one"), rs("two"), rs("three")}))
	}

	a := mk()
	a.Begin(Pos{})
	before, _ := a.store.Get(2)
	a.SetKnown(2, before, rs("TWO"))
	a.End(Pos{})

	b := mk()
	b.Begin(Pos{})
	b.Set(2, rs("TWO"))
	b.End(Pos{})

	if got, want := snapshot(a.store), snapshot(b.store); !sameLines(got, want) {
		t.Fatalf("SetKnown state %q != Set state %q", got, want)
	}
	a.Undo()
	b.Undo()
	if got, want := snapshot(a.store), snapshot(b.store); !sameLines(got, want) {
		t.Fatalf("after undo: SetKnown %q != Set %q", got, want)
	}
	a.Redo()
	b.Redo()
	if got, want := snapshot(a.store), snapshot(b.store); !sameLines(got, want) {
		t.Fatalf("after redo: SetKnown %q != Set %q", got, want)
	}
}

// TestSetKnownCallerMayMutate proves SetKnown copies both before and line: the
// caller's slices can be reused/overwritten after the call without disturbing
// the stored line or the undo record (the after-image, restored on redo).
func TestSetKnownCallerMayMutate(t *testing.T) {
	for _, tc := range []struct {
		name  string
		store buffer.LineStore
	}{
		{"mem", buffer.NewMemFromLines([][]rune{rs("aaa"), rs("bbb")})},
		{"paged", buffer.NewPagedBytes([]byte("aaa\nbbb\n"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := New(tc.store)
			before := rs("bbb") // caller-held before-image of line 2
			line := rs("BBBB")  // caller-held replacement
			l.Begin(Pos{})
			l.SetKnown(2, before, line)
			l.End(Pos{})

			// Scribble over the caller's slices; the store and log must not change.
			for i := range before {
				before[i] = 'X'
			}
			for i := range line {
				line[i] = 'Y'
			}

			if got, _ := tc.store.Get(2); string(got) != "BBBB" {
				t.Fatalf("stored line corrupted by caller mutation: %q", string(got))
			}
			l.Undo()
			if got, _ := tc.store.Get(2); string(got) != "bbb" {
				t.Fatalf("undo before-image corrupted: %q", string(got))
			}
			l.Redo()
			if got, _ := tc.store.Get(2); string(got) != "BBBB" {
				t.Fatalf("redo after-image corrupted: %q", string(got))
			}
		})
	}
}

// TestStoreReturnIsRetainable confirms the slice returned by Set/Insert is the
// store's own immutable copy: it stays valid and unchanged after later edits,
// which is what lets the undo log alias it as the after-image.
func TestStoreReturnIsRetainable(t *testing.T) {
	for _, tc := range []struct {
		name  string
		store buffer.LineStore
	}{
		{"mem", buffer.NewMemFromLines([][]rune{rs("x")})},
		{"paged", buffer.NewPagedBytes([]byte("x\n"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kept := tc.store.Set(1, rs("first"))
			tc.store.Insert(1, rs("second"))
			tc.store.Set(2, rs("third"))
			tc.store.Delete(1)
			if string(kept) != "first" {
				t.Fatalf("retained Set return mutated by later edits: %q", string(kept))
			}
		})
	}
}
