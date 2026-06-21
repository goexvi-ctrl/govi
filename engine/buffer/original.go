package buffer

import (
	"bytes"
	"io"
)

// indexStride controls the sparse line-offset index: one byte offset is kept
// per `indexStride` lines of the original. This keeps the index small even for
// files with hundreds of millions of lines (8 bytes per stride lines) while
// bounding the forward scan needed to reach any line to at most indexStride
// lines of bytes.
const indexStride = 1024

// scanChunk is the read granularity used when scanning/reading the original.
const scanChunk = 64 * 1024

// original is a read-only view of the unedited source file. It is never loaded
// fully into memory: a sparse index of line-start byte offsets is built once,
// and individual line contents are read on demand via the underlying sourceAt.
type original struct {
	src    sourceAt
	size   int64
	stride int64

	// checkpoints[j] is the byte offset at which line j*stride begins.
	checkpoints []int64
	nlines      int64
	// lastHasNewline reports whether the file's final line is newline
	// terminated; informational, mirroring vi's "incomplete last line".
	lastHasNewline bool
}

// newOriginal scans src once to build the sparse line index.
func newOriginal(src sourceAt) (*original, error) {
	o := &original{src: src, size: src.Size(), stride: indexStride}
	o.checkpoints = append(o.checkpoints, 0) // line 0 starts at byte 0
	if o.size == 0 {
		return o, nil
	}

	buf := make([]byte, scanChunk)
	var pos int64
	var lineNo int64
	var newlines int64
	var lastByte byte
	for pos < o.size {
		n, err := o.src.ReadAt(buf, pos)
		for i := 0; i < n; i++ {
			lastByte = buf[i]
			if buf[i] == '\n' {
				newlines++
				next := lineNo + 1
				if next%o.stride == 0 {
					o.checkpoints = append(o.checkpoints, pos+int64(i)+1)
				}
				lineNo = next
			}
		}
		pos += int64(n)
		if err == io.EOF || n == 0 {
			break
		}
	}

	if lastByte == '\n' {
		o.nlines = newlines
		o.lastHasNewline = true
	} else {
		o.nlines = newlines + 1
		o.lastHasNewline = false
	}
	return o, nil
}

// line returns the runes of 0-based original line oln (without the trailing
// newline). It seeks via the nearest checkpoint and scans forward, reading at
// most indexStride lines worth of bytes.
func (o *original) line(oln int64) ([]rune, error) {
	if oln < 0 || oln >= o.nlines {
		return nil, ErrNoSuchLine
	}

	cp := oln / o.stride
	pos := o.checkpoints[cp]
	skip := oln - cp*o.stride

	buf := make([]byte, scanChunk)

	// Skip forward over `skip` newlines to reach the start of line oln.
	for skip > 0 {
		n, err := o.src.ReadAt(buf, pos)
		consumed := 0
		for consumed < n && skip > 0 {
			b := buf[consumed]
			consumed++
			if b == '\n' {
				skip--
			}
		}
		pos += int64(consumed)
		if skip == 0 {
			break
		}
		if err == io.EOF || n == 0 {
			break
		}
	}

	// Read from pos to the next newline or EOF.
	var content []byte
	for {
		n, err := o.src.ReadAt(buf, pos)
		if n > 0 {
			if idx := bytes.IndexByte(buf[:n], '\n'); idx >= 0 {
				content = append(content, buf[:idx]...)
				break
			}
			content = append(content, buf[:n]...)
			pos += int64(n)
		}
		if err == io.EOF || n == 0 {
			break
		}
	}
	return []rune(string(content)), nil
}
