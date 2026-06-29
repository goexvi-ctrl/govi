package engine

import "testing"

// typeEx feeds an ex command line (without the leading ':' or trailing newline)
// to the engine as keystrokes.
func typeEx(e *Engine, cmd string) {
	e.Input(KeyEvent{Rune: ':'})
	for _, r := range cmd {
		e.Input(KeyEvent{Rune: r})
	}
	e.Input(KeyEvent{Key: KeyEnter})
}

// BenchmarkSubstituteAll runs :%s/o/0/g over a 10k-line buffer, the substitute
// breakdown's workload. Each iteration re-opens a fresh buffer so the work is
// constant.
func BenchmarkSubstituteAll(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		e := benchEngine(b, 10000)
		b.StartTimer()
		typeEx(e, "%s/o/0/g")
	}
}

// BenchmarkGlobalSubst runs :g/fox/s//cat/ over a 10k-line buffer, the global
// breakdown's workload.
func BenchmarkGlobalSubst(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		e := benchEngine(b, 10000)
		b.StartTimer()
		typeEx(e, "g/fox/s//cat/")
	}
}
