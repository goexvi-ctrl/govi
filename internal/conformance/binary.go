package conformance

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
