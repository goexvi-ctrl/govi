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

	// cache memoizes decoded lines by original line number. The original is
	// immutable, so cached entries never go stale.
	cache *origCache

	// blockBuf caches the most recently read scanChunk-sized block of the
	// original. blockPos is its byte offset and blockLen its valid length. A run
	// of nearby line reads is served from this block, so sequential access issues
	// roughly one syscall per block instead of one per line. The original is
	// immutable, so a cached block never goes stale.
	blockBuf []byte
	blockPos int64
	blockLen int

	// seqValid/seqLine/seqPos form a one-entry sequential cursor: seqPos is the
	// byte offset at which line seqLine begins, set after each read so a
	// subsequent forward read can resume without rescanning from a checkpoint.
	seqValid bool
	seqLine  int64
	seqPos   int64
}

// blockAt returns the cached bytes starting at file offset pos, loading a fresh
// scanChunk-sized block if pos falls outside the currently cached one. The
// returned slice aliases blockBuf and is only valid until the next blockAt call;
// callers must copy out anything they keep. An empty result means EOF at pos.
func (o *original) blockAt(pos int64) []byte {
	if o.blockLen > 0 && pos >= o.blockPos && pos < o.blockPos+int64(o.blockLen) {
		return o.blockBuf[pos-o.blockPos : o.blockLen]
	}
	if o.blockBuf == nil {
		o.blockBuf = make([]byte, scanChunk)
	}
	n, _ := o.src.ReadAt(o.blockBuf, pos)
	o.blockPos = pos
	o.blockLen = n
	return o.blockBuf[:n]
}

// newOriginal scans src once to build the sparse line index.
func newOriginal(src sourceAt) (*original, error) {
	o := &original{src: src, size: src.Size(), stride: indexStride, cache: newOrigCache()}
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

	// The original is immutable, so a cached decode is always valid. Hand back a
	// private copy to preserve the LineStore contract (callers may treat Get's
	// result as their own) and protect the cached slice from mutation.
	if r, ok := o.cache.get(oln); ok {
		return cloneRunes(r), nil
	}

	// Pick the closest known line start at or before oln: the sparse checkpoint,
	// or the sequential cursor left by the previous read. The cursor makes a
	// forward scan O(1) skips for sequential access (the common :g/:%s and
	// initial-paint pattern), instead of re-scanning from the checkpoint -- which
	// is O(stride) per line, i.e. quadratic across a stride block. Byte offsets
	// into the immutable original never go stale, so the cursor needs no
	// invalidation.
	startLine := (oln / o.stride) * o.stride
	pos := o.checkpoints[oln/o.stride]
	if o.seqValid && o.seqLine <= oln && o.seqLine > startLine {
		startLine = o.seqLine
		pos = o.seqPos
	}
	skip := oln - startLine

	// Skip forward over `skip` newlines to reach the start of line oln, reading
	// through the block cache so a run of nearby lines shares one syscall.
	for skip > 0 {
		chunk := o.blockAt(pos)
		if len(chunk) == 0 {
			break // EOF
		}
		consumed := 0
		for consumed < len(chunk) && skip > 0 {
			if chunk[consumed] == '\n' {
				skip--
			}
			consumed++
		}
		pos += int64(consumed)
	}

	// Read from pos to the next newline or EOF, advancing pos so it ends at the
	// first byte of the next line.
	var content []byte
	for {
		chunk := o.blockAt(pos)
		if len(chunk) == 0 {
			break // EOF
		}
		if idx := bytes.IndexByte(chunk, '\n'); idx >= 0 {
			content = append(content, chunk[:idx]...)
			pos += int64(idx) + 1
			break
		}
		content = append(content, chunk...)
		pos += int64(len(chunk))
	}

	// Leave a sequential cursor at the start of the next line.
	o.seqValid = true
	o.seqLine = oln + 1
	o.seqPos = pos

	decoded := []rune(string(content))
	o.cache.put(oln, decoded)
	return cloneRunes(decoded), nil
}
