package buffer

import (
	"bufio"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parseLines splits file content into lines the same way original does, so a
// Mem store can be seeded identically to a Paged store for differential tests.
func parseLines(s string) [][]rune {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	if strings.HasSuffix(s, "\n") {
		parts = parts[:len(parts)-1]
	}
	out := make([][]rune, len(parts))
	for i, p := range parts {
		out[i] = []rune(p)
	}
	return out
}

func TestPagedFromBytes(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"\n", []string{""}},
		{"a", []string{"a"}},
		{"a\n", []string{"a"}},
		{"a\nb\n", []string{"a", "b"}},
		{"a\nb", []string{"a", "b"}},
		{"\n\n\n", []string{"", "", ""}},
		{"héllo\nwörld\n", []string{"héllo", "wörld"}}, // multibyte runes
	}
	for _, tc := range cases {
		p := NewPagedBytes([]byte(tc.in))
		got := lines(t, p)
		if len(tc.want) == 0 && len(got) == 0 {
			continue
		}
		eq(t, got, tc.want...)
	}
}

func compareStores(t *testing.T, want, got LineStore) {
	t.Helper()
	if want.Lines() != got.Lines() {
		t.Fatalf("line counts differ: want %d, got %d", want.Lines(), got.Lines())
	}
	for i := int64(1); i <= want.Lines(); i++ {
		w, _ := want.Get(i)
		g, _ := got.Get(i)
		if string(w) != string(g) {
			t.Fatalf("line %d differs: want %q, got %q", i, string(w), string(g))
		}
	}
}

// TestPagedDifferential applies the same random op stream to Mem (reference)
// and Paged and checks they stay identical.
func TestPagedDifferential(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 99))
	seed := "alpha\nbeta\ngamma\ndelta\nepsilon\n"
	ref := NewMemFromLines(parseLines(seed))
	pg := NewPagedBytes([]byte(seed))

	payload := func() []rune {
		return []rune(fmt.Sprintf("x%d", rng.IntN(100000)))
	}
	for step := 0; step < 4000; step++ {
		n := ref.Lines()
		switch rng.IntN(4) {
		case 0: // Insert
			lno := int64(rng.IntN(int(n)+1)) + 1
			line := payload()
			ref.Insert(lno, line)
			pg.Insert(lno, line)
		case 1: // Append
			lno := int64(rng.IntN(int(n) + 1))
			line := payload()
			ref.Append(lno, line)
			pg.Append(lno, line)
		case 2: // Set
			if n == 0 {
				continue
			}
			lno := int64(rng.IntN(int(n))) + 1
			line := payload()
			ref.Set(lno, line)
			pg.Set(lno, line)
		case 3: // Delete
			if n == 0 {
				continue
			}
			lno := int64(rng.IntN(int(n))) + 1
			ref.Delete(lno)
			pg.Delete(lno)
		}
		if step%200 == 0 {
			compareStores(t, ref, pg)
		}
	}
	compareStores(t, ref, pg)
}

// TestPagedLargeFile builds a paged store over a large temp file and verifies
// correct access and bounded memory (a sparse index, not per-line offsets, and
// no full materialization).
func TestPagedLargeFile(t *testing.T) {
	nlines := 1_000_000
	if testing.Short() {
		nlines = 50_000
	}

	genLine := func(i int) string { return fmt.Sprintf("line-%07d-payload-data", i) }

	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriterSize(f, 1<<20)
	for i := 0; i < nlines; i++ {
		w.WriteString(genLine(i))
		w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	p, fh, err := NewPagedFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()

	if p.Lines() != int64(nlines) {
		t.Fatalf("Lines = %d, want %d", p.Lines(), nlines)
	}

	// Bounded memory: the sparse index holds ~ nlines/stride offsets, not one
	// per line, proving we are not indexing (or holding) the whole file.
	maxCheckpoints := int64(nlines)/indexStride + 4
	if got := int64(len(p.orig.checkpoints)); got > maxCheckpoints {
		t.Fatalf("checkpoints = %d, want <= %d (index not sparse)", got, maxCheckpoints)
	}

	// Spot-check random lines against the generator.
	rng := rand.New(rand.NewPCG(7, 7))
	for i := 0; i < 1000; i++ {
		lno := int64(rng.IntN(nlines)) + 1
		got, err := p.Get(lno)
		if err != nil {
			t.Fatalf("Get(%d): %v", lno, err)
		}
		if want := genLine(int(lno - 1)); string(got) != want {
			t.Fatalf("Get(%d) = %q, want %q", lno, string(got), want)
		}
	}

	// First and last lines.
	if got, _ := p.Get(1); string(got) != genLine(0) {
		t.Fatalf("first line = %q", string(got))
	}
	if got, _ := p.Get(int64(nlines)); string(got) != genLine(nlines-1) {
		t.Fatalf("last line = %q", string(got))
	}

	// Edit in the middle and verify neighbors are intact.
	mid := int64(nlines / 2)
	p.Set(mid, []rune("EDITED"))
	p.Insert(mid, []rune("INSERTED"))
	if got, _ := p.Get(mid); string(got) != "INSERTED" {
		t.Fatalf("after insert, line %d = %q", mid, string(got))
	}
	if got, _ := p.Get(mid + 1); string(got) != "EDITED" {
		t.Fatalf("after edit, line %d = %q", mid+1, string(got))
	}
	if got, _ := p.Get(mid - 1); string(got) != genLine(int(mid-2)) {
		t.Fatalf("neighbor above corrupted: %q", string(got))
	}
	if got, _ := p.Get(mid + 2); string(got) != genLine(int(mid)) {
		t.Fatalf("neighbor below corrupted: %q", string(got))
	}
}
