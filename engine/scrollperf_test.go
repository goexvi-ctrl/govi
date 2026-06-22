package engine

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestGOnLargeFileIsFast guards against the O(N^2) scroll bug: G must reach the
// last line in time proportional to the screen height, not the file size.
func TestGOnLargeFileIsFast(t *testing.T) {
	nlines := 500_000
	if testing.Short() {
		nlines = 50_000
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "large")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriterSize(f, 1<<20)
	for i := 0; i < nlines; i++ {
		fmt.Fprintf(w, "line %d of the large file\n", i)
	}
	w.Flush()
	f.Close()

	e := New(&captureFrontend{}, Options{})
	fh, _ := os.Open(path)
	defer fh.Close()
	if err := e.Open(path); err != nil {
		t.Fatal(err)
	}
	e.Resize(40, 80)

	start := time.Now()
	drive(e, "G")
	elapsed := time.Since(start)

	if e.scr.cursor.Line != int64(nlines) {
		t.Fatalf("G -> line %d, want %d", e.scr.cursor.Line, nlines)
	}
	// The viewport must show the last line near the bottom.
	if e.scr.top > int64(nlines) || e.scr.top < int64(nlines)-40 {
		t.Fatalf("after G, top = %d, want near %d", e.scr.top, nlines)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("G on %d-line file took %v; should be ~O(screen height)", nlines, elapsed)
	}
	t.Logf("G on %d lines: %v", nlines, elapsed)

	// Jumping back to the top is also fast.
	start = time.Now()
	drive(e, "1G")
	if e.scr.top != 1 || time.Since(start) > 500*time.Millisecond {
		t.Fatalf("1G: top=%d in %v", e.scr.top, time.Since(start))
	}
}
