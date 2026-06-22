package conformance

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/creack/pty"
)

// ViSession is a scripted vi-mode session: an input file's initial contents and
// a raw keystroke script (ESC as 0x1b, Enter as \r). The harness appends an
// ":wq\r" so the result is flushed to disk.
type ViSession struct {
	Input string
	Keys  string
}

// RunOracleVi runs the session against the C nvi oracle under a pseudo-terminal
// and returns the resulting file contents. nvi draws to the terminal, so the
// PTY output is drained and discarded.
func RunOracleVi(oracle string, s ViSession) (string, error) {
	dir, err := os.MkdirTemp("", "govi-vi-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "buf.txt")
	if err := os.WriteFile(file, []byte(s.Input), 0o644); err != nil {
		return "", err
	}

	cmd := exec.Command(oracle, file)
	cmd.Env = append(os.Environ(),
		"TERM=xterm", "EXINIT=", "NEXINIT=", "HOME="+dir, "LINES=24", "COLUMNS=80")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		return "", err
	}
	defer ptmx.Close()

	// Drain terminal output so nvi never blocks writing escape sequences.
	go io.Copy(io.Discard, ptmx)

	// Give vi a moment to initialize, then type the script and write+quit.
	time.Sleep(150 * time.Millisecond)
	if _, err := ptmx.Write([]byte(s.Keys)); err != nil {
		return "", err
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := ptmx.Write([]byte("\x1b:wq\r")); err != nil {
		return "", err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		return "", errTimeout
	}

	out, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// errTimeout signals the oracle did not exit in time.
var errTimeout = io.ErrUnexpectedEOF

// trimTrailingNewline normalizes a file's single trailing newline so buffer
// comparisons ignore it.
func normalizeBuf(s string) string {
	return string(bytes.TrimRight([]byte(s), "\n"))
}
