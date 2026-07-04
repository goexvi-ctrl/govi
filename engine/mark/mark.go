// Package mark tracks named buffer positions (vi marks) and keeps them correct
// as lines are inserted and deleted. It corresponds to nvi's mark handling
// (common/mark.c): marks are named by a rune (a-z, plus the ' and ` context
// marks), a mark on a deleted line becomes invalid, and marks below an edit
// shift to follow their line.
//
// Phase 1 implements line-granular fixups -- the structural part shared with
// the buffer/undo layer. Intra-line column fixups (a mark moving when text is
// inserted or deleted within its own line) are applied by the editing
// operations in the vi phase.
package mark

// Mark is a named position. Line is 1-based; Col is a 0-based rune index. When
// Deleted is true the mark's line was removed and the mark no longer resolves
// to a position (vi reports "Mark ... not set" / nonexistent).
type Mark struct {
	Line    int64
	Col     int
	Deleted bool
}

// PrevContext is the previous-context mark name. nvi keeps a single "absolute"
// mark that '' and `` return to; common/mark.c (ABSMARK1 '\'', ABSMARK2 '`')
// translates a ` reference to ' on every set and get, so "m'", "m`", '' and ``
// all name the same position. normName replicates that translation.
const PrevContext = '\''

func normName(name rune) rune {
	if name == '`' {
		return PrevContext
	}
	return name
}

// Set holds the marks for one buffer.
type Set struct {
	m map[rune]Mark
}

// New returns an empty mark set.
func New() *Set { return &Set{m: make(map[rune]Mark)} }

// Get returns the mark named name and whether it is set and not deleted.
func (s *Set) Get(name rune) (Mark, bool) {
	mk, ok := s.m[normName(name)]
	if !ok || mk.Deleted {
		return Mark{}, false
	}
	return mk, true
}

// Set records mark name at the given position, clearing any deleted state.
func (s *Set) Set(name rune, mk Mark) {
	mk.Deleted = false
	s.m[normName(name)] = mk
}

// Clear removes mark name entirely.
func (s *Set) Clear(name rune) { delete(s.m, normName(name)) }

// Names returns the names of all currently valid (non-deleted) marks.
func (s *Set) Names() []rune {
	out := make([]rune, 0, len(s.m))
	for name, mk := range s.m {
		if !mk.Deleted {
			out = append(out, name)
		}
	}
	return out
}

// LinesInserted updates marks after n lines are inserted so that the first new
// line is at `at`. Marks at or below `at` shift down by n.
func (s *Set) LinesInserted(at, n int64) {
	if n <= 0 {
		return
	}
	for name, mk := range s.m {
		if mk.Deleted {
			continue
		}
		if mk.Line >= at {
			mk.Line += n
			s.m[name] = mk
		}
	}
}

// LinesDeleted updates marks after the n lines [at, at+n) are removed. A mark
// on a deleted line is marked Deleted; a mark below the deleted range shifts up
// by n.
func (s *Set) LinesDeleted(at, n int64) {
	if n <= 0 {
		return
	}
	end := at + n // exclusive
	for name, mk := range s.m {
		if mk.Deleted {
			continue
		}
		switch {
		case mk.Line >= at && mk.Line < end:
			mk.Deleted = true
		case mk.Line >= end:
			mk.Line -= n
		}
		s.m[name] = mk
	}
}
