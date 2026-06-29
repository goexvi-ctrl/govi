package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// benchEngine builds an engine over a generated file for benchmarks.
func benchEngine(b *testing.B, lines int) *Engine {
	b.Helper()
	dir := b.TempDir()
	path := filepath.Join(dir, "f.txt")
	var sb strings.Builder
	for i := 0; i < lines; i++ {
		fmt.Fprintf(&sb, "the quick brown fox jumps over lazy dog %d\n", i)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		b.Fatal(err)
	}
	fe := &captureFrontend{}
	e := New(fe, Options{})
	if err := e.Open(path); err != nil {
		b.Fatal(err)
	}
	e.Resize(24, 80)
	return e
}

// BenchmarkRedrawStableMemo measures a stable-view full-screen redraw (no edit
// between paints) through the DisplayLine memo -- the common interactive case
// (typing within a line, in-screen cursor moves, status/message updates).
func BenchmarkRedrawStableMemo(b *testing.B) {
	e := benchEngine(b, 10000)
	s := e.scr
	rows := int64(s.mapRows)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for lno := s.top; lno < s.top+rows; lno++ {
			_ = s.displayLine(lno)
		}
	}
}

// BenchmarkRedrawStableNoMemo measures the same stable-view redraw recomputing
// each row from scratch (the pre-memo path), for comparison.
func BenchmarkRedrawStableNoMemo(b *testing.B) {
	e := benchEngine(b, 10000)
	s := e.scr
	rows := int64(s.mapRows)
	tab := s.opts.Int("tabstop")
	list := s.opts.Bool("list")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for lno := s.top; lno < s.top+rows; lno++ {
			_ = makeDisplayLine(s.lineRunes(lno), tab, list)
		}
	}
}
