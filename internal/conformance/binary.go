package conformance

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The engine-level runners (govi.go) pin behavior; this file pins the binary:
// flag parsing and ex batch mode (-e -s), driven exactly like the oracle so
// the same script goes through both editors' real command lines.

var (
	goviBinOnce sync.Once
	goviBinPath string
	goviBinErr  error
)

// GoviBinary builds cmd/govi once per test process and returns the binary's
// path, so binary-level conformance always tests the current tree (never a
// stale installed copy).
func GoviBinary() (string, error) {
	goviBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "govi-bin-*")
		if err != nil {
			goviBinErr = err
			return
		}
		goviBinPath = filepath.Join(dir, "govi")
		out, err := exec.Command("go", "build", "-o", goviBinPath, "govi/cmd/govi").CombinedOutput()
		if err != nil {
			goviBinErr = fmt.Errorf("build govi: %v\n%s", err, out)
		}
	})
	return goviBinPath, goviBinErr
}

// RunBatchBinary runs an editor binary in ex batch mode (`BIN -e -s FILE`
// with the script on stdin) and returns the resulting file contents and the
// stdout the script produced. The oracle and govi take the identical
// invocation, so both sides of a comparison go through this.
func RunBatchBinary(bin string, s ExSession) (content, stdout string, err error) {
	dir, err := os.MkdirTemp("", "govi-bin-conf-*")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "buf.txt")
	if err := os.WriteFile(file, []byte(s.Input), 0o644); err != nil {
		return "", "", err
	}

	var script strings.Builder
	for _, c := range s.Commands {
		script.WriteString(c)
		script.WriteByte('\n')
	}
	script.WriteString("wq\n")

	outBuf := new(bytes.Buffer)
	cmd := exec.Command(bin, "-e", "-s", file)
	// Run from the per-test temp dir so the local oracle's relative
	// "vi.recover" recdir stays inside it.
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(script.String())
	cmd.Stdout = outBuf
	cmd.Stderr = new(bytes.Buffer)
	// Isolate from the user's real config, and keep recovery files in the
	// per-test temp dir (govi's recdir defaults to $TMPDIR).
	cmd.Env = append(os.Environ(), "NEXINIT=", "EXINIT=", "HOME="+dir, "TMPDIR="+dir)
	if err := cmd.Run(); err != nil {
		return "", "", err
	}

	out, err := os.ReadFile(file)
	if err != nil {
		return "", "", err
	}
	return string(out), stripBatchNoise(outBuf.String()), nil
}

// stripBatchNoise drops nvi's no-tty startup complaint from batch stdout: the
// C nvi probes the terminal at startup and, with stdin/stdout redirected,
// emits "Error: stderr: Inappropriate ioctl for device" before the script
// output. It is an artifact of driving nvi headlessly, not script output.
func stripBatchNoise(out string) string {
	var kept []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Error: stderr: ") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// BatchOutcome is the observable result of one ex-batch run: the on-disk
// buffer after the script, any script stdout, and whether the process exited
// zero / timed out. Content is returned even on a non-zero exit so callers can
// compare RE-error paths (nvi leaves the file unchanged and aborts the script).
type BatchOutcome struct {
	Content  string
	Stdout   string
	Stderr   string
	ExitErr  error // nil on exit 0
	TimedOut bool
}

// RunBatchBinaryFull is RunBatchBinary plus timeout and "read file even on
// failure". timeout <= 0 means no deadline. The binary is always invoked as
// `BIN -e -s FILE` with the script on stdin -- the same public surface a user
// (and nvi's re_conv / option layer) sees; it does not link Spencer's regcomp.
func RunBatchBinaryFull(bin string, s ExSession, timeout time.Duration) BatchOutcome {
	var out BatchOutcome
	dir, err := os.MkdirTemp("", "govi-bin-conf-*")
	if err != nil {
		out.ExitErr = err
		return out
	}
	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "buf.txt")
	if err := os.WriteFile(file, []byte(s.Input), 0o644); err != nil {
		out.ExitErr = err
		return out
	}

	var script strings.Builder
	for _, c := range s.Commands {
		script.WriteString(c)
		script.WriteByte('\n')
	}
	script.WriteString("wq\n")

	ctx := context.Background()
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := exec.CommandContext(ctx, bin, "-e", "-s", file)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(script.String())
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = append(os.Environ(), "NEXINIT=", "EXINIT=", "HOME="+dir, "TMPDIR="+dir)
	runErr := cmd.Run()
	out.Stdout = stripBatchNoise(stdout.String())
	out.Stderr = stderr.String()
	if runErr != nil {
		out.ExitErr = runErr
		if ctx.Err() == context.DeadlineExceeded {
			out.TimedOut = true
		}
	}
	// Best-effort read even after failure or kill: RE errors leave the buffer
	// as-written; a timeout may leave a partial write or the original.
	if b, err := os.ReadFile(file); err == nil {
		out.Content = string(b)
	}
	return out
}
