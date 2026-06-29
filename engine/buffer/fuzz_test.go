package buffer

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

// genFuzzLines builds a large, deliberately irregular set of lines: empty lines,
// short and medium ASCII, multibyte runes (so byte length != rune count), and a
// few lines longer than scanChunk so a single line spans multiple read blocks.
// The irregular byte offsets are what stress the original reader's block cache,
// sequential cursor, and sparse checkpoint index.
func genFuzzLines(rng *rand.Rand, n int) []string {
	lines := make([]string, n)
	for i := range lines {
		switch rng.IntN(16) {
		case 0:
			lines[i] = "" // empty line
		case 1:
			lines[i] = strings.Repeat("世a", 1+rng.IntN(30)) // multibyte (CJK + ASCII)
		case 2:
			lines[i] = "é" + strings.Repeat("e", rng.IntN(50)) // accented + ASCII
		default:
			lines[i] = fmt.Sprintf("line%d-%s", i, strings.Repeat("x", rng.IntN(140)))
		}
	}
	// Inject a few lines that exceed a single scanChunk so the reader must span
	// multiple blocks for one line.
	for _, at := range []int{3, n / 2, n - 5} {
		if at >= 0 && at < n {
			lines[at] = strings.Repeat("B", scanChunk*2+123)
		}
	}
	return lines
}

// TestPagedFuzzLargeOriginal applies the same random read/edit stream to a Mem
// reference and a Paged store built over a large, variable-length original, and
// checks every read agrees. Unlike TestPagedDifferential (tiny seed, all reads
// quickly come from the add buffer), here most lines stay in the original, so the
// block cache + sequential cursor + checkpoint paths are the ones under test --
// including forward, backward, strided, and random access, plus edits that leave
// a mix of original and add-buffer spans.
func TestPagedFuzzLargeOriginal(t *testing.T) {
	steps := 6000
	nlines := 4000
	if testing.Short() {
		steps = 1500
		nlines = 1500
	}
	rng := rand.New(rand.NewPCG(2024, 7))

	lines := genFuzzLines(rng, nlines)
	data := strings.Join(lines, "\n") + "\n"
	ref := NewMemFromLines(parseLines(data))
	pg := NewPagedBytes([]byte(data))

	if ref.Lines() != pg.Lines() {
		t.Fatalf("initial line counts differ: ref %d, pg %d", ref.Lines(), pg.Lines())
	}

	check := func(lno int64) {
		w, we := ref.Get(lno)
		g, ge := pg.Get(lno)
		if (we == nil) != (ge == nil) {
			t.Fatalf("Get(%d) error mismatch: ref %v, pg %v", lno, we, ge)
		}
		if string(w) != string(g) {
			t.Fatalf("Get(%d) differs:\n ref len=%d %.60q\n pg  len=%d %.60q",
				lno, len(w), string(w), len(g), string(g))
		}
	}

	payload := func() []rune {
		switch rng.IntN(3) {
		case 0:
			return []rune("")
		case 1:
			return []rune(fmt.Sprintf("edit-%d-%s", rng.IntN(1<<20), strings.Repeat("y", rng.IntN(60))))
		default:
			return []rune(strings.Repeat("世", 1+rng.IntN(20)))
		}
	}

	for step := 0; step < steps; step++ {
		n := ref.Lines()
		if n == 0 {
			ref.Insert(1, []rune("seed"))
			pg.Insert(1, []rune("seed"))
			continue
		}
		switch rng.IntN(10) {
		case 0, 1, 2, 3: // forward sweep window
			start := int64(rng.IntN(int(n)))
			for k := int64(0); k < 64 && start+k < n; k++ {
				check(start + k + 1)
			}
		case 4, 5: // backward sweep window
			start := int64(rng.IntN(int(n)))
			for k := int64(0); k < 64 && start-k >= 0; k++ {
				check(start - k + 1)
			}
		case 6: // strided
			stride := int64(1 + rng.IntN(2000))
			for lno := int64(1); lno <= n; lno += stride {
				check(lno)
			}
		case 7: // random scatter
			for k := 0; k < 64; k++ {
				check(int64(rng.IntN(int(n))) + 1)
			}
		case 8: // edit: set or insert
			lno := int64(rng.IntN(int(n))) + 1
			p := payload()
			if rng.IntN(2) == 0 {
				ref.Set(lno, p)
				pg.Set(lno, p)
			} else {
				ref.Insert(lno, p)
				pg.Insert(lno, p)
			}
			check(lno)
		case 9: // edit: delete
			if n > 1 {
				lno := int64(rng.IntN(int(n))) + 1
				ref.Delete(lno)
				pg.Delete(lno)
			}
		}
		if step%500 == 0 {
			compareStores(t, ref, pg)
		}
	}
	compareStores(t, ref, pg)
}
