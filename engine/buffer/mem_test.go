package buffer

import (
	"errors"
	"testing"
)

func rs(s string) []rune { return []rune(s) }

func lines(t *testing.T, ls LineStore) []string {
	t.Helper()
	out := make([]string, 0, ls.Lines())
	for i := int64(1); i <= ls.Lines(); i++ {
		ln, err := ls.Get(i)
		if err != nil {
			t.Fatalf("Get(%d): %v", i, err)
		}
		out = append(out, string(ln))
	}
	return out
}

func eq(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("line count: got %d %q, want %d %q", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d: got %q, want %q", i+1, got[i], want[i])
		}
	}
}

func TestMemBasic(t *testing.T) {
	m := NewMem()
	if m.Lines() != 0 {
		t.Fatalf("empty store Lines = %d, want 0", m.Lines())
	}
	m.Append(0, rs("one"))     // -> [one]
	m.Append(1, rs("three"))   // -> [one three]
	m.Insert(2, rs("two"))     // -> [one two three]
	eq(t, lines(t, m), "one", "two", "three")

	m.Set(2, rs("TWO"))
	eq(t, lines(t, m), "one", "TWO", "three")

	m.Delete(1)
	eq(t, lines(t, m), "TWO", "three")

	m.Insert(1, rs("zero")) // insert at front
	eq(t, lines(t, m), "zero", "TWO", "three")
}

func TestMemGetOutOfRange(t *testing.T) {
	m := NewMemFromLines([][]rune{rs("a")})
	if _, err := m.Get(0); !errors.Is(err, ErrNoSuchLine) {
		t.Fatalf("Get(0) err = %v, want ErrNoSuchLine", err)
	}
	if _, err := m.Get(2); !errors.Is(err, ErrNoSuchLine) {
		t.Fatalf("Get(2) err = %v, want ErrNoSuchLine", err)
	}
}

func TestMemOwnsData(t *testing.T) {
	src := rs("mutable")
	m := NewMem()
	m.Append(0, src)
	src[0] = 'X' // caller mutates its slice; store must be unaffected
	got, _ := m.Get(1)
	if string(got) != "mutable" {
		t.Fatalf("store did not copy input: got %q", string(got))
	}
}
