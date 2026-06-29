package buffer

// Decoded-line cache for the original (read-only) source.
//
// The original's content is immutable for the lifetime of the store: edits go to
// the add buffer and the treap only ever remaps which original line numbers are
// visible -- it never changes what original line N contains. That makes a cache
// keyed by the 0-based original line number correct by construction, with no
// invalidation ever required. It collapses the dominant editing hot path, where
// (*original).line otherwise re-pread+re-decodes the same lines on every redraw
// and on every :g/:%s scan pass.
//
// The cache is bounded by total decoded runes (not entry count) so that a file
// of very long lines cannot blow memory; the piece table exists precisely to
// edit files too large to hold in RAM, so the cache must stay bounded too.

// origCacheRuneBudget caps the decoded runes held in the cache (~64 MiB at 4
// bytes/rune). Comfortably holds whole working files; for multi-GB originals it
// keeps a large sliding window (every redraw/scan working set fits) while
// bounding memory.
const origCacheRuneBudget = 16 << 20

type origCacheEntry struct {
	oln        int64
	runes      []rune // canonical decoded line; never mutated after insertion
	prev, next *origCacheEntry
}

// origCache is a rune-budget-bounded LRU keyed by original line number. Not safe
// for concurrent use; the store is driven from a single goroutine.
type origCache struct {
	m          map[int64]*origCacheEntry
	head, tail *origCacheEntry // head = most-recently-used
	runes      int
}

func newOrigCache() *origCache {
	return &origCache{m: make(map[int64]*origCacheEntry)}
}

// get returns the canonical decoded runes for oln and true on a hit, moving the
// entry to the most-recently-used position. The returned slice is the cache's
// own copy and must not be mutated by the caller.
func (c *origCache) get(oln int64) ([]rune, bool) {
	e, ok := c.m[oln]
	if !ok {
		return nil, false
	}
	c.toFront(e)
	return e.runes, true
}

// put records runes as the decoded content of oln, then evicts least-recently
// -used entries until within the rune budget. runes must be a slice the cache
// can own (the caller must not mutate it afterward).
func (c *origCache) put(oln int64, runes []rune) {
	if e, ok := c.m[oln]; ok {
		c.runes += len(runes) - len(e.runes)
		e.runes = runes
		c.toFront(e)
		c.evict()
		return
	}
	e := &origCacheEntry{oln: oln, runes: runes}
	c.m[oln] = e
	c.runes += len(runes)
	c.pushFront(e)
	c.evict()
}

// evict drops least-recently-used entries until the rune budget is satisfied,
// always keeping at least one entry.
func (c *origCache) evict() {
	for c.runes > origCacheRuneBudget && len(c.m) > 1 {
		t := c.tail
		if t == nil {
			return
		}
		c.unlink(t)
		delete(c.m, t.oln)
		c.runes -= len(t.runes)
	}
}

func (c *origCache) toFront(e *origCacheEntry) {
	if c.head == e {
		return
	}
	c.unlink(e)
	c.pushFront(e)
}

func (c *origCache) pushFront(e *origCacheEntry) {
	e.prev = nil
	e.next = c.head
	if c.head != nil {
		c.head.prev = e
	}
	c.head = e
	if c.tail == nil {
		c.tail = e
	}
}

func (c *origCache) unlink(e *origCacheEntry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else if c.head == e {
		c.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else if c.tail == e {
		c.tail = e.prev
	}
	e.prev, e.next = nil, nil
}
