//go:build !unix

package engine

import "os"

// LoadStartup interprets NEXINIT/EXINIT on non-Unix hosts. Startup files are
// not read.
func (e *Engine) LoadStartup() error {
	if nexinit := os.Getenv("NEXINIT"); nexinit != "" {
		return e.runStartupScript("NEXINIT", nexinit)
	}
	if exinit := os.Getenv("EXINIT"); exinit != "" {
		return e.runStartupScript("EXINIT", exinit)
	}
	return nil
}
