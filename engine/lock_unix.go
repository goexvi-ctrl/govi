//go:build unix

package engine

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// lockResult reports the outcome of an advisory file lock attempt.
type lockResult int

const (
	lockSuccess lockResult = iota // exclusive lock acquired (held via the fd)
	lockUnavail                   // another process holds the lock
	lockFailed                    // locking unsupported (e.g. some NFS): proceed
)

// tryFileLock attempts a non-blocking exclusive advisory lock on f, mirroring
// nvi's file_lock (common/exf.c): flock(LOCK_EX|LOCK_NB), where EAGAIN/
// EWOULDBLOCK means the file is locked elsewhere and any other error means
// locking is unsupported. The lock is released when f is closed.
func tryFileLock(f *os.File) lockResult {
	if f == nil {
		return lockFailed
	}
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return lockSuccess
	}
	if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
		return lockUnavail
	}
	return lockFailed
}
