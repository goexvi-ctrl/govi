// Package buffer provides the text storage layer for the editor: a
// line-addressed store matching vi's model (1-based line numbers, whole-line
// get/set/insert/append/delete), with implementations ranging from a simple
// in-memory slice to a paged store for very large files.
//
// This replaces nvi's Berkeley DB recno backend and the db_* API in
// common/vi_db.c, presenting the same line-oriented operations to the rest of
// the engine.
package buffer

import "errors"

// ErrNoSuchLine is returned by Get for an out-of-range line number.
var ErrNoSuchLine = errors.New("buffer: no such line")

// LineStore is the storage abstraction the engine edits through. Line numbers
// are 1-based. A store always contains at least zero lines; an empty file is
// represented by Lines() == 0. Lines hold runes (nvi's CHAR_T), not bytes, so
// callers work in code points and leave encoding to the load/save layer.
//
// Implementations need not be safe for concurrent use; the engine drives a
// single store from one goroutine.
type LineStore interface {
	// Lines returns the number of lines currently stored.
	Lines() int64

	// Get returns the runes of line lno. The returned slice must not be
	// retained or mutated by the caller; copy if you need to keep it. Returns
	// ErrNoSuchLine if lno is not in [1, Lines()].
	Get(lno int64) ([]rune, error)

	// Set replaces line lno with a copy of line.
	Set(lno int64, line []rune)

	// Insert inserts a copy of line before line lno (so it becomes the new
	// line lno). Insert at Lines()+1 is equivalent to Append after the last
	// line.
	Insert(lno int64, line []rune)

	// Append inserts a copy of line after line lno. Append(0, ...) inserts at
	// the very beginning.
	Append(lno int64, line []rune)

	// Delete removes line lno.
	Delete(lno int64)
}
