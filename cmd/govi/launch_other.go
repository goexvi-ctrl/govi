//go:build !darwin

package main

import (
	"fmt"
	"os"
)

// runGUI is a stub on non-macOS platforms: GoVi.app and `govi -g` are macOS
// only. The terminal editor works everywhere.
func runGUI(silent, wait bool, files []string) int {
	fmt.Fprintln(os.Stderr, "govi: -g (GUI mode) is only supported on macOS")
	return 1
}
