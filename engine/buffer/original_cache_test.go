package buffer

import (
	"fmt"
	"strings"
	"testing"
)

// TestOrigCacheReturnsPrivateCopy verifies that mutating a slice returned by Get
// does not corrupt the cached decode, so a later Get of the same line still sees
// the original content. This is the core safety property of the decoded-line
// cache.
func TestOrigCacheReturnsPrivateCopy(t *testing.T) {
	p := NewPagedBytes([]byte("hello\nworld\n"))

	first, err := p.Get(1)
	if err != nil {
		t.Fatalf("Get(1): %v", err)
	}
	if string(first) != "hello" {
		t.Fatalf("Get(1) = %q, want %q", string(first), "hello")
	}
	// Scribble over the caller's copy.
	for i := range first {
		first[i] = 'X'
	}

	again, _ := p.Get(1)
	if string(again) != "hello" {
		t.Fatalf("after mutating caller copy, Get(1) = %q, want %q", string(again), "hello")
	}
}

// TestOrigCacheStableAcrossEdits verifies that an unedited original line reads
// the same content before and after edits to other lines shift its position.
func TestOrigCacheStableAcrossEdits(t *testing.T) {
	p := NewPagedBytes([]byte("aaa\nbbb\nccc\nddd\n"))

	// Warm the cache for the original line "ccc" (currently line 3).
	if got, _ := p.Get(3); string(got) != "ccc" {
		t.Fatalf("Get(3) = %q, want ccc", string(got))
	}

	// Insert and delete around it so "ccc" moves to a different line number.
	p.Insert(1, []rune("NEW"))   // ccc now at line 4
	p.Delete(2)                  // remove old aaa; ccc now at line 3 again then...
	p.Set(1, []rune("CHANGED"))  // edit line 1

	// Find "ccc" and confirm it is intact.
	found := false
	for i := int64(1); i <= p.Lines(); i++ {
		if got, _ := p.Get(i); string(got) == "ccc" {
			found = true
		}
	}
	if !found {
		t.Fatal("original line ccc lost after edits")
	}
}

// TestOrigCacheLRUBounded drives the LRU directly past its rune budget and
// confirms it stays bounded, evicts least-recently-used entries, and keeps the
// most-recently-used ones.
func TestOrigCacheLRUBounded(t *testing.T) {
	c := newOrigCache()
	const lineLen = 1000
	// Insert well past the budget.
	nlines := int64(origCacheRuneBudget/lineLen) * 3
	mk := func(i int64) []rune { return make([]rune, lineLen) }
	for i := int64(0); i < nlines; i++ {
		c.put(i, mk(i))
	}
	if c.runes > origCacheRuneBudget {
		t.Fatalf("cache rune count %d exceeds budget %d", c.runes, origCacheRuneBudget)
	}
	if int64(len(c.m)) != int64(c.runes/lineLen) {
		t.Fatalf("map size %d inconsistent with runes %d", len(c.m), c.runes)
	}
	// The most-recently-inserted line must still be present; the oldest evicted.
	if _, ok := c.get(nlines - 1); !ok {
		t.Fatal("most-recently-used entry was evicted")
	}
	if _, ok := c.get(0); ok {
		t.Fatal("least-recently-used entry was not evicted")
	}

	// Re-touch an entry, then push more: the touched one should survive longer
	// than untouched neighbours.
	keep := nlines - 2
	c.get(keep) // move to front
	for i := nlines; i < nlines+10; i++ {
		c.put(i, mk(i))
	}
	if _, ok := c.get(keep); !ok {
		t.Fatal("recently-touched entry was evicted before colder ones")
	}
}

// TestOrigCacheReReadsAfterEviction confirms that after the cache evicts a line,
// re-reading it from the source still returns correct content.
func TestOrigCacheReReadsAfterEviction(t *testing.T) {
	const nlines = 3000
	const lineLen = 200 // 3000*200 = 600k runes; small, fast, but exercises misses
	var b strings.Builder
	for i := 0; i < nlines; i++ {
		fmt.Fprintf(&b, "%0*d\n", lineLen, i)
	}
	p := NewPagedBytes([]byte(b.String()))

	for _, ln := range []int64{1, nlines / 2, nlines} {
		want := fmt.Sprintf("%0*d", lineLen, ln-1)
		// Read twice: first miss populates, second hits the cache.
		for pass := 0; pass < 2; pass++ {
			if got, _ := p.Get(ln); string(got) != want {
				t.Fatalf("Get(%d) pass %d mismatch", ln, pass)
			}
		}
	}
}
