//go:build !unix

package tcell

import "fmt"

// Suspend is not supported on non-Unix platforms.
func (f *Frontend) Suspend() error {
	return fmt.Errorf("Suspend not supported")
}
