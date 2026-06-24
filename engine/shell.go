package engine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Shell filtering: the ! operator (and :range!cmd) pipe a range of lines through
// a shell command, replacing them with its output; :!cmd runs a command without
// filtering. This corresponds to nvi's ex/ex_bang.c and filter handling.

func (e *Engine) shellProg() string {
	if sh := e.scr.opts.Str("shell"); sh != "" {
		return sh
	}
	return "/bin/sh"
}

// runShellCmd runs cmd via the shell option, feeding input on stdin. It returns
// combined stdout and stderr, matching nvi's filter pipes (ex_filter.c dup2's
// both to the utility output pipe). cols and rows set COLUMNS/LINES so utilities
// such as ls format multi-column output when stdout is not a tty.
func (e *Engine) runShellCmd(cmd, input string, cols, rows int) (string, error) {
	shell := e.shellProg()
	c := exec.Command(shell, "-c", cmd)
	e.ensureCwd()
	if e.cwd != "" {
		c.Dir = e.cwd
	}
	c.Env = shellEnv(cols, rows)
	if input != "" {
		c.Stdin = strings.NewReader(input)
	}
	out, err := c.CombinedOutput()
	return string(out), err
}

func shellEnv(cols, rows int) []string {
	env := os.Environ()
	if cols < 1 {
		cols = 80
	}
	if rows < 1 {
		rows = 24
	}
	env = setEnvVar(env, "COLUMNS", fmt.Sprintf("%d", cols))
	env = setEnvVar(env, "LINES", fmt.Sprintf("%d", rows))
	return env
}

func setEnvVar(env []string, key, val string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	found := false
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			out = append(out, prefix+val)
			found = true
		} else {
			out = append(out, e)
		}
	}
	if !found {
		out = append(out, prefix+val)
	}
	return out
}

// exShell implements :sh[ell]: run an interactive shell (ex/ex_shell.c).
func (e *Engine) exShell(*exCmd) error {
	if e.scr.opts.Bool("secure") {
		return fmt.Errorf("The shell command is not supported when the secure edit option is set")
	}
	shell := e.shellProg()
	runner, ok := e.fe.(ShellRunner)
	if !ok {
		return fmt.Errorf("Shell not available")
	}
	return runner.RunShell(shell, e.ExActive())
}

// exBang implements :[range]!cmd. With a range it filters those lines; without
// one it runs the command and reports completion.
func (e *Engine) exBang(c *exCmd) error {
	cmd := strings.TrimSpace(c.arg)
	if cmd == "" {
		return fmt.Errorf("Usage: [range]!command")
	}
	if c.addrCount == 0 {
		return e.runBangNoRange(cmd)
	}
	return e.filterLines(c.addr1, c.addr2, cmd)
}

// filterLines pipes lines [l1,l2] through cmd and replaces them with the output.
func (e *Engine) filterLines(l1, l2 int64, cmd string) error {
	s := e.scr
	if l1 < 1 || l2 > s.lineCount() || l1 > l2 {
		return fmt.Errorf("Invalid address")
	}
	var in strings.Builder
	for i := l1; i <= l2; i++ {
		in.WriteString(string(s.lineRunes(i)))
		in.WriteByte('\n')
	}
	out, err := e.runShellCmd(cmd, in.String(), e.bangCols(), e.bangRows())
	if err != nil {
		// nvi still replaces the filtered lines with stdout/stderr even when
		// the utility exits non-zero (ex_filter.c reads the pipe before wait).
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return fmt.Errorf("%s: %v", cmd, err)
		}
	}
	outLines := splitOutputLines(out)

	e.beginChange()
	for i := l2; i >= l1; i-- {
		s.deleteLine(i)
	}
	at := l1 - 1
	for i, ln := range outLines {
		s.appendLine(at+int64(i), ln)
	}
	if s.store.Lines() == 0 {
		s.log.Insert(1, []rune{})
	}
	e.endChange()
	tl := clampLine(s, l1)
	s.cursor = Pos{Line: tl, Col: s.firstNonBlank(tl)}
	return nil
}

// startFilter implements the vi ! operator: prompt with "!" on the status line
// while the filter command is entered (nvi's v_filter / v_tcmd).
func (e *Engine) startFilter(l1, l2 int64) {
	e.scr.mode = ModeExColon
	e.scr.cmdPrefix = '!'
	e.scr.colon = nil
	e.scr.filterL1, e.scr.filterL2 = l1, l2
}

func splitOutputLines(s string) [][]rune {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	if strings.HasSuffix(s, "\n") {
		parts = parts[:len(parts)-1]
	}
	out := make([][]rune, len(parts))
	for i, p := range parts {
		out[i] = []rune(p)
	}
	return out
}
