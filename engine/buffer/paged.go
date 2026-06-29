package buffer

import (
	"math/rand/v2"
	"os"
)

// Paged is a LineStore for arbitrarily large files. Unedited content stays on
// disk in the original (read on demand); edits append whole lines to an
// in-memory add buffer. The current line ordering is held as a balanced tree
// (a treap) of spans, where each span is a contiguous run of lines drawn from
// one source. Positional access, insert, and delete are O(log n) in the number
// of spans, and memory is bounded by the sparse original index plus the edited
// lines -- never the whole file.
//
// This is the large-file counterpart to Mem and replaces nvi's recno DB paging.
type Paged struct {
	orig *original
	add  [][]rune // append-only; span indices into this stay valid forever
	root *node
}

// span is a run of `count` consecutive lines from one source. When fromAdd is
// true the lines are add[start : start+count]; otherwise they are original
// lines start : start+count.
type span struct {
	fromAdd bool
	start   int64
	count   int64
}

func (s span) splitAt(d int64) (span, span) {
	return span{s.fromAdd, s.start, d}, span{s.fromAdd, s.start + d, s.count - d}
}

type node struct {
	sp          span
	prio        uint64
	left, right *node
	lcount      int64 // total lines in this subtree
}

func lc(n *node) int64 {
	if n == nil {
		return 0
	}
	return n.lcount
}

func update(n *node) {
	if n != nil {
		n.lcount = n.sp.count + lc(n.left) + lc(n.right)
	}
}

func newNode(sp span) *node {
	n := &node{sp: sp, prio: rand.Uint64(), lcount: sp.count}
	return n
}

// split divides t into a (first k lines) and b (the rest).
func split(t *node, k int64) (a, b *node) {
	if t == nil {
		return nil, nil
	}
	ls := lc(t.left)
	switch {
	case k <= ls:
		la, lb := split(t.left, k)
		t.left = lb
		update(t)
		return la, t
	case k >= ls+t.sp.count:
		ra, rb := split(t.right, k-ls-t.sp.count)
		t.right = ra
		update(t)
		return t, rb
	default:
		// Split falls inside this node's span.
		within := k - ls
		lsp, rsp := t.sp.splitAt(within)
		ln := newNode(lsp)
		ln.left = t.left
		update(ln)
		rn := newNode(rsp)
		rn.right = t.right
		update(rn)
		return ln, rn
	}
}

// merge concatenates a (whose lines precede b's) into one treap.
func merge(a, b *node) *node {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case a.prio > b.prio:
		a.right = merge(a.right, b)
		update(a)
		return a
	default:
		b.left = merge(a, b.left)
		update(b)
		return b
	}
}

// locate returns the span and the 0-based offset within it for 0-based line k.
func locate(t *node, k int64) (span, int64) {
	for {
		ls := lc(t.left)
		if k < ls {
			t = t.left
			continue
		}
		if k < ls+t.sp.count {
			return t.sp, k - ls
		}
		k -= ls + t.sp.count
		t = t.right
	}
}

// NewPagedBytes builds a paged store over the given file bytes (held in memory).
// Useful for tests and small originals.
func NewPagedBytes(b []byte) *Paged { return newPaged(bytesAt(b)) }

// NewPagedFile opens path and builds a paged store over it without reading the
// whole file into memory. The caller is responsible for keeping the file
// readable for the lifetime of the store; Close releases the handle.
func NewPagedFile(path string) (*Paged, *os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return newPaged(&fileAt{f: f, size: fi.Size()}), f, nil
}

func newPaged(src sourceAt) *Paged {
	o, _ := newOriginal(src)
	p := &Paged{orig: o}
	if o.nlines > 0 {
		p.root = newNode(span{fromAdd: false, start: 0, count: o.nlines})
	}
	return p
}

// addSpan appends a copy of line to the add buffer and returns a single-line
// span referring to it along with the stored copy. The add buffer is
// append-only and never mutated, so the returned slice is safe to retain
// read-only (the undo log keeps it as the record's after-image).
func (p *Paged) addSpan(line []rune) (span, []rune) {
	c := cloneRunes(line)
	p.add = append(p.add, c)
	return span{fromAdd: true, start: int64(len(p.add) - 1), count: 1}, c
}

func (p *Paged) Lines() int64 { return lc(p.root) }

func (p *Paged) Get(lno int64) ([]rune, error) {
	if lno < 1 || lno > p.Lines() {
		return nil, ErrNoSuchLine
	}
	sp, off := locate(p.root, lno-1)
	if sp.fromAdd {
		return cloneRunes(p.add[sp.start+off]), nil
	}
	return p.orig.line(sp.start + off)
}

func (p *Paged) Set(lno int64, line []rune) []rune {
	if lno < 1 || lno > p.Lines() {
		return nil
	}
	left, rest := split(p.root, lno-1)
	_, right := split(rest, 1) // drop the old single line
	sp, stored := p.addSpan(line)
	p.root = merge(merge(left, newNode(sp)), right)
	return stored
}

func (p *Paged) Insert(lno int64, line []rune) []rune {
	k := lno - 1
	if k < 0 {
		k = 0
	}
	if k > p.Lines() {
		k = p.Lines()
	}
	left, right := split(p.root, k)
	sp, stored := p.addSpan(line)
	p.root = merge(merge(left, newNode(sp)), right)
	return stored
}

func (p *Paged) Append(lno int64, line []rune) []rune { return p.Insert(lno+1, line) }

func (p *Paged) Delete(lno int64) {
	if lno < 1 || lno > p.Lines() {
		return
	}
	left, rest := split(p.root, lno-1)
	_, right := split(rest, 1)
	p.root = merge(left, right)
}
