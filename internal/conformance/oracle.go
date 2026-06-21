// Package conformance drives the real C nvi (the "oracle") and govi through
// identical scripted sessions and compares the observable result, pinning
// govi's behavior to nvi's.
//
// Two oracle paths exist:
//
//   - Ex batch mode (implemented here): nvi runs headlessly as `nvi -e -s FILE`
//     reading ex commands from stdin. This needs no pseudo-terminal and covers
//     ex-command conformance (addresses, ranges, :s, :g, :d, :m, ...).
//   - Vi keystroke mode (added in the vi-mode phase): nvi runs under a PTY and
//     is fed raw keystrokes. That path requires a PTY helper and is gated
//     separately.
//
// The oracle binary is located via FindOracle: $GOVI_NVI_ORACLE, else a binary
// at internal/conformance/oracle/nvi, else `nvi` on $PATH. Tests skip when no
// oracle is available so the suite stays green on machines without nvi.
package conformance

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FindOracle returns the path to an nvi binary to use as the oracle, or "" if
// none is available.
func FindOracle() string {
	if p := os.Getenv("GOVI_NVI_ORACLE"); p != "" {
		if isExec(p) {
			return p
		}
	}
	if _, file, ok := callerDir(); ok {
		local := filepath.Join(filepath.Dir(file), "oracle", "nvi")
		if isExec(local) {
			return local
		}
	}
	if p, err := exec.LookPath("nvi"); err == nil {
		return p
	}
	return ""
}

func isExec(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}

// ExSession is a scripted ex-mode session: an input file's initial contents and
// the ex commands to run against it.
type ExSession struct {
	Input    string   // initial file contents
	Commands []string // ex commands, e.g. {"%s/a/b/g"}; a final "wq" is appended automatically
}

// RunOracleEx runs the session against the C nvi oracle in ex batch mode and
// returns the resulting file contents.
func RunOracleEx(oracle string, s ExSession) (string, error) {
	dir, err := os.MkdirTemp("", "govi-conf-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "buf.txt")
	if err := os.WriteFile(file, []byte(s.Input), 0o644); err != nil {
		return "", err
	}

	var script strings.Builder
	for _, c := range s.Commands {
		script.WriteString(c)
		script.WriteByte('\n')
	}
	script.WriteString("wq\n")

	cmd := exec.Command(oracle, "-e", "-s", file)
	cmd.Stdin = strings.NewReader(script.String())
	cmd.Stdout = new(bytes.Buffer)
	cmd.Stderr = new(bytes.Buffer)
	// Isolate from the user's real config so behavior is reproducible.
	cmd.Env = append(os.Environ(), "NEXINIT=", "EXINIT=", "HOME="+dir)
	if err := cmd.Run(); err != nil {
		return "", err
	}

	out, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
