package buffer

// Mem is an in-memory LineStore backed by a slice of lines. It is used for
// scratch buffers, small files, and as the reference implementation that the
// paged store is differential-tested against. Each stored line owns its rune
// slice (callers' slices are copied on the way in and out is read-only).
type Mem struct {
	lines [][]rune
}

// NewMem returns an empty in-memory store.
func NewMem() *Mem { return &Mem{} }

// NewMemFromLines returns a store seeded with copies of the given lines.
func NewMemFromLines(lines [][]rune) *Mem {
	m := &Mem{lines: make([][]rune, len(lines))}
	for i, ln := range lines {
		m.lines[i] = cloneRunes(ln)
	}
	return m
}

func (m *Mem) Lines() int64 { return int64(len(m.lines)) }

func (m *Mem) Get(lno int64) ([]rune, error) {
	if lno < 1 || lno > int64(len(m.lines)) {
		return nil, ErrNoSuchLine
	}
	return m.lines[lno-1], nil
}

func (m *Mem) Set(lno int64, line []rune) []rune {
	if lno < 1 || lno > int64(len(m.lines)) {
		return nil
	}
	c := cloneRunes(line)
	m.lines[lno-1] = c
	return c
}

func (m *Mem) Insert(lno int64, line []rune) []rune {
	if lno < 1 {
		lno = 1
	}
	if lno > int64(len(m.lines))+1 {
		lno = int64(len(m.lines)) + 1
	}
	idx := lno - 1
	m.lines = append(m.lines, nil)
	copy(m.lines[idx+1:], m.lines[idx:])
	c := cloneRunes(line)
	m.lines[idx] = c
	return c
}

func (m *Mem) Append(lno int64, line []rune) []rune {
	return m.Insert(lno+1, line)
}

func (m *Mem) Delete(lno int64) {
	if lno < 1 || lno > int64(len(m.lines)) {
		return
	}
	idx := lno - 1
	copy(m.lines[idx:], m.lines[idx+1:])
	m.lines[len(m.lines)-1] = nil
	m.lines = m.lines[:len(m.lines)-1]
}

func cloneRunes(r []rune) []rune {
	if r == nil {
		return []rune{}
	}
	out := make([]rune, len(r))
	copy(out, r)
	return out
}
