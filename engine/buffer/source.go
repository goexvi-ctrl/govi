package buffer

import (
	"io"
	"os"
)

// sourceAt is the read-only byte source underlying a paged store's original
// file. It is satisfied by *os.File-backed files and by in-memory byte slices
// (and by synthetic generators in tests), so the paged store never assumes the
// whole file is resident.
type sourceAt interface {
	// ReadAt reads len(p) bytes starting at off, following io.ReaderAt
	// semantics (a short read at end of input returns io.EOF).
	ReadAt(p []byte, off int64) (int, error)
	// Size returns the total number of bytes in the source.
	Size() int64
}

// bytesAt adapts a byte slice to sourceAt, used for small in-memory originals
// and tests.
type bytesAt []byte

func (b bytesAt) Size() int64 { return int64(len(b)) }

func (b bytesAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(b)) {
		return 0, io.EOF
	}
	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// fileAt adapts an *os.File to sourceAt.
type fileAt struct {
	f    *os.File
	size int64
}

func (f *fileAt) Size() int64 { return f.size }

func (f *fileAt) ReadAt(p []byte, off int64) (int, error) {
	return f.f.ReadAt(p, off)
}
