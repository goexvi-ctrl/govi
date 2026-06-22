// Package register implements vi's cut buffers (registers): the unnamed
// register, the named registers a-z (with A-Z appending), and the numbered
// delete registers 1-9. It corresponds to nvi's CB/cut machinery
// (common/cut.c, put.c). Text is stored linewise or characterwise so puts can
// reproduce vi's p/P behavior exactly.
package register

// Kind distinguishes a linewise register (whole lines, put above/below) from a
// characterwise one (inline text, put before/after the cursor).
type Kind int

const (
	CharWise Kind = iota
	LineWise
)

// Text is register contents. Lines holds the text split on newlines: for a
// linewise register these are whole buffer lines; for a characterwise register
// Lines[0] is the text after the start column and Lines[len-1] the text before
// the end column, with any full lines between.
type Text struct {
	Kind  Kind
	Lines [][]rune
}

// Clone returns a deep copy so stored register data is never aliased with the
// buffer.
func (t Text) Clone() Text {
	out := Text{Kind: t.Kind, Lines: make([][]rune, len(t.Lines))}
	for i, ln := range t.Lines {
		c := make([]rune, len(ln))
		copy(c, ln)
		out.Lines[i] = c
	}
	return out
}

// Empty reports whether the register holds nothing.
func (t Text) Empty() bool { return len(t.Lines) == 0 }

// Set holds all registers for one editor.
type Set struct {
	unnamed  Text
	named    map[rune]Text
	numbered [9]Text // index 0 == register "1"
}

// New returns an empty register set.
func New() *Set { return &Set{named: make(map[rune]Text)} }

// Get returns the contents of the register named name. name == 0 or '"' selects
// the unnamed register; 'a'-'z' (and 'A'-'Z') the named registers; '1'-'9' the
// numbered ones. An unknown name returns the unnamed register.
func (s *Set) Get(name rune) Text {
	switch {
	case name == 0 || name == '"':
		return s.unnamed
	case name >= 'a' && name <= 'z':
		return s.named[name]
	case name >= 'A' && name <= 'Z':
		return s.named[name-'A'+'a']
	case name >= '1' && name <= '9':
		return s.numbered[name-'1']
	default:
		return s.unnamed
	}
}

// StoreYank records a yank. With no name it goes to the unnamed register; with
// a name a-z it goes there (A-Z appends), leaving the numbered registers
// untouched (matching vi: yanks do not rotate the delete registers).
func (s *Set) StoreYank(name rune, t Text) {
	t = t.Clone()
	s.unnamed = t
	s.storeNamed(name, t)
}

// StoreDelete records a delete. With no name and a linewise delete it rotates
// the numbered registers (1 newest). With a name a-z it goes there (A-Z
// appends). The unnamed register always reflects the most recent delete.
func (s *Set) StoreDelete(name rune, t Text) {
	t = t.Clone()
	s.unnamed = t
	if name != 0 {
		s.storeNamed(name, t)
		return
	}
	if t.Kind == LineWise {
		copy(s.numbered[1:], s.numbered[:8]) // shift 1..8 -> 2..9
		s.numbered[0] = t
	}
}

func (s *Set) storeNamed(name rune, t Text) {
	switch {
	case name >= 'a' && name <= 'z':
		s.named[name] = t
	case name >= 'A' && name <= 'Z':
		s.named[name-'A'+'a'] = appendText(s.named[name-'A'+'a'], t)
	}
}

// appendText concatenates b onto a following vi's A-Z append semantics. If both
// are characterwise the join is inline at the seam; otherwise lines accumulate.
func appendText(a, b Text) Text {
	if a.Empty() {
		return b
	}
	if b.Empty() {
		return a
	}
	out := a.Clone()
	bl := b.Clone().Lines
	if a.Kind == CharWise && b.Kind == CharWise {
		// Join b's first line onto a's last line.
		last := len(out.Lines) - 1
		out.Lines[last] = append(out.Lines[last], bl[0]...)
		out.Lines = append(out.Lines, bl[1:]...)
		return out
	}
	out.Kind = LineWise
	out.Lines = append(out.Lines, bl...)
	return out
}
