//go:build unix

package tcell

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

func TestEmergencyFatalSignalRestoresTTY(t *testing.T) {
	bin := emergencyTestBinary(t)

	dir := t.TempDir()
	file := filepath.Join(dir, "buf.txt")
	if err := os.WriteFile(file, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, file)
	cmd.Env = append(os.Environ(), "TERM=xterm", "EXINIT=", "NEXINIT=", "HOME="+dir, "LINES=24", "COLUMNS=80")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatal(err)
	}
	defer ptmx.Close()

	go io.Copy(io.Discard, ptmx)
	time.Sleep(200 * time.Millisecond)

	if err := cmd.Process.Signal(syscall.SIGSEGV); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for SIGSEGV exit")
	}

	fd := int(ptmx.Fd())
	term, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		t.Fatal(err)
	}
	if term.Lflag&unix.ICANON == 0 || term.Lflag&unix.ECHO == 0 {
		t.Fatal("pty left in raw mode after SIGSEGV")
	}
}

func emergencyTestBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "nvi")
	modRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", bin, "./cmd/nvi")
	build.Dir = modRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build nvi: %v\n%s", err, out)
	}
	return bin
}