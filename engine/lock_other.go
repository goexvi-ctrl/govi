//go:build !unix

package engine

import "os"

// lockResult reports the outcome of an advisory file lock attempt.
type lockResult int

const (
	lockSuccess lockResult = iota
	lockUnavail
	lockFailed
)

// tryFileLock is a no-op on platforms without flock; locking is treated as
// unsupported so editing proceeds normally (matching nvi's LOCK_FAILED path).
func tryFileLock(f *os.File) lockResult { return lockFailed }
