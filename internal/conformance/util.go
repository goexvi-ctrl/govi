package conformance

import "runtime"

// callerDir returns this source file's path so FindOracle can locate the
// sibling oracle/ directory during `go test`.
func callerDir() (dir, file string, ok bool) {
	_, f, _, ok := runtime.Caller(0)
	return "", f, ok
}
