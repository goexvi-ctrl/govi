//go:build darwin || linux

package main

// PTY-level regression tests for terminal state management: what the editor
// writes to a real terminal and the termios it leaves behind. These caught
// nothing at the goterm layer because goterm advertises TERM=ansi (no
// smcup/rmcup) and never inspects termios.
//
// The bugs pinned here: a session started as goex must never enter the
// alternate screen buffer (ex is a scrolling line interface), and quitting
// from ex mode must not leave the tty raw or in the alternate screen (the old
// runExMode unconditionally Resume()d after shutdown had already Fini'd).

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"

	"govi/internal/conformance"
)

const (
	smcup = "\x1b[?1049h" // enter alternate screen (xterm)
	rmcup = "\x1b[?1049l" // leave alternate screen
)

// ptySession spawns the built govi binary (optionally via a differently-named
// symlink) on a fresh PTY and collects everything it writes.
type ptySession struct {
	t      *testing.T
	cmd    *exec.Cmd
	master *os.File
	mu     sync.Mutex
	out    bytes.Buffer
}

func startPtySession(t *testing.T, progname string, args ...string) *ptySession {
	t.Helper()
	bin, err := conformance.GoviBinary()
	if err != nil {
		t.Fatalf("GoviBinary: %v", err)
	}
	if progname != "govi" {
		link := filepath.Join(t.TempDir(), progname)
		if err := os.Symlink(bin, link); err != nil {
			t.Fatal(err)
		}
		bin = link
	}
	cmd := exec.Command(bin, append([]string{"-n"}, args...)...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	master, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		t.Skipf("pty: %v", err)
	}
	s := &ptySession{t: t, cmd: cmd, master: master}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				s.mu.Lock()
				s.out.Write(buf[:n])
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
		master.Close()
	})
	return s
}

func (s *ptySession) output() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.out.String()
}

// waitOutput waits until the session's output satisfies pred.
func (s *ptySession) waitOutput(pred func(string) bool) {
	s.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if pred(s.output()) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.t.Fatalf("timeout waiting for output; got %q", s.output())
}

func (s *ptySession) send(text string) {
	s.t.Helper()
	if _, err := s.master.WriteString(text); err != nil {
		s.t.Fatalf("pty write: %v", err)
	}
}

// waitExit waits for the editor to exit on its own.
func (s *ptySession) waitExit() {
	s.t.Helper()
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		s.t.Fatalf("editor did not exit; output %q", s.output())
	}
}

// assertCooked checks the pty's termios is back in canonical/echo mode -- what
// a line-mode program leaves behind. The pty pair shares one termios, so the
// master fd reflects what the child set on the slave.
func (s *ptySession) assertCooked() {
	s.t.Helper()
	tio, err := unix.IoctlGetTermios(int(s.master.Fd()), ioctlReadTermios)
	if err != nil {
		s.t.Fatalf("tcgetattr: %v", err)
	}
	if tio.Lflag&unix.ICANON == 0 || tio.Lflag&unix.ECHO == 0 {
		s.t.Errorf("tty left raw after exit: ICANON=%v ECHO=%v",
			tio.Lflag&unix.ICANON != 0, tio.Lflag&unix.ECHO != 0)
	}
}

// exFixture writes a small file for the pty sessions.
func exFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestPtyGoexStaysOffAltScreen: a goex session that never enters vi mode must
// never touch the alternate screen buffer, and must leave the tty cooked.
func TestPtyGoexStaysOffAltScreen(t *testing.T) {
	s := startPtySession(t, "goex", exFixture(t))
	s.waitOutput(func(o string) bool { return strings.Contains(o, ":") })
	s.send("q\n")
	s.waitExit()
	if o := s.output(); strings.Contains(o, smcup) || strings.Contains(o, "\x1b[?47h") {
		t.Errorf("goex entered the alternate screen; output %q", o)
	}
	s.assertCooked()
}

// TestPtyGoexViTransition: :vi from a goex session enters the alternate
// screen; quitting from vi leaves it and restores the tty.
func TestPtyGoexViTransition(t *testing.T) {
	s := startPtySession(t, "goex", exFixture(t))
	s.waitOutput(func(o string) bool { return strings.Contains(o, ":") })
	s.send("vi\n")
	s.waitOutput(func(o string) bool { return strings.Contains(o, smcup) })
	s.send(":q\r")
	s.waitExit()
	out := s.output()
	if last := strings.LastIndex(out, smcup); !strings.Contains(out[last:], rmcup) {
		t.Errorf("alternate screen not left after quit; output tail %q", out[last:])
	}
	s.assertCooked()
}

// TestPtyQuitFromExRestoresTerminal: vi -> Q -> q must leave the alternate
// screen exactly once and not re-enter it (the shutdown-then-Resume bug that
// left the tty raw in a cleared screen).
func TestPtyQuitFromExRestoresTerminal(t *testing.T) {
	s := startPtySession(t, "govi", exFixture(t))
	s.waitOutput(func(o string) bool { return strings.Contains(o, smcup) })
	s.send("Q")
	s.waitOutput(func(o string) bool { return strings.Contains(o, rmcup) })
	s.send("q\n")
	s.waitExit()
	out := s.output()
	if last := strings.LastIndex(out, smcup); strings.Contains(out[last+len(smcup):], smcup) || !strings.Contains(out[last:], rmcup) {
		t.Errorf("terminal not restored after ex-mode quit; output tail %q", out[last:])
	}
	s.assertCooked()
}
