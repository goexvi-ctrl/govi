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

// runShellStdout runs cmd through the shell and returns only its standard output.
// Used for filename expansion (nvi argv_sexp), which discards standard error so
// that an unmatched pattern's diagnostics don't leak into the expanded names.
func (e *Engine) runShellStdout(cmd string) (string, error) {
	shell := e.shellProg()
	c := exec.Command(shell, "-c", cmd)
	e.ensureCwd()
	if e.cwd != "" {
		c.Dir = e.cwd
	}
	c.Env = shellEnv(e.bangCols(), e.bangRows())
	out, err := c.Output()
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

// expandShellNames performs nvi's argv_fexp filename substitution: an unescaped
// '%' becomes the current file name and '#' the alternate file name
// (ex/ex_argv.c). A backslash escapes either to its literal (the backslash is
// removed). It errors when '%'/'#' has no file to substitute. The same
// substitution applies to shell command strings and to file-name arguments of
// commands like :e, :w and :r.
func (e *Engine) expandShellNames(cmd string) (string, error) {
	var b strings.Builder
	r := []rune(cmd)
	for i := 0; i < len(r); i++ {
		switch r[i] {
		case '\\':
			if i+1 < len(r) && (r[i+1] == '%' || r[i+1] == '#') {
				b.WriteRune(r[i+1])
				i++
				continue
			}
			b.WriteRune('\\')
		case '%':
			if e.scr.name == "" {
				return "", fmt.Errorf("No filename to substitute for %%")
			}
			b.WriteString(e.scr.name)
		case '#':
			if e.altFile == "" {
				return "", fmt.Errorf("No filename to substitute for #")
			}
			b.WriteString(e.altFile)
		default:
			b.WriteRune(r[i])
		}
	}
	return b.String(), nil
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
// This is the shared body of the vi ! operator and :[range]!cmd, so the secure
// gate here covers both (nvi marks ! as E_SECURE).
func (e *Engine) filterLines(l1, l2 int64, cmd string) error {
	if e.scr.opts.Bool("secure") {
		return fmt.Errorf("The ! command is not supported when the secure edit option is set")
	}
	s := e.scr
	if l1 < 1 || l2 > s.lineCount() || l1 > l2 {
		return fmt.Errorf("Invalid address")
	}
	cmd, err := e.expandShellNames(cmd)
	if err != nil {
		return err
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

// writeToCommand implements :[range]w !cmd -- pipe the addressed lines (default
// the whole file) to cmd's standard input and show its output. Unlike :range!cmd
// the buffer is left unchanged (nvi ex/ex_write.c, the "!" target).
func (e *Engine) writeToCommand(c *exCmd, cmd string) error {
	if e.scr.opts.Bool("secure") {
		return fmt.Errorf("The ! command is not supported when the secure edit option is set")
	}
	s := e.scr
	l1, l2 := int64(1), s.lineCount()
	if c.addrCount > 0 {
		l1, l2 = c.addr1, c.addr2
	}
	if l1 < 1 || l2 > s.lineCount() || l1 > l2 {
		return fmt.Errorf("Invalid address")
	}
	cmd, err := e.expandShellNames(cmd)
	if err != nil {
		return err
	}
	var in strings.Builder
	for i := l1; i <= l2; i++ {
		in.WriteString(string(s.lineRunes(i)))
		in.WriteByte('\n')
	}
	out, err := e.runShellCmd(cmd, in.String(), e.bangCols(), e.bangRows())
	if err != nil {
		// Like a filter, still show stdout/stderr when the utility exits
		// non-zero; only a failure to launch is reported as an error.
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return fmt.Errorf("%s: %v", cmd, err)
		}
	}
	e.presentBangOutput(out)
	return nil
}

// readFromCommand implements :[line]r !cmd -- run cmd via the shell and return
// its output as lines to insert into the buffer (nvi ex/ex_read.c).
func (e *Engine) readFromCommand(cmd string) ([][]rune, error) {
	if e.scr.opts.Bool("secure") {
		return nil, fmt.Errorf("The ! command is not supported when the secure edit option is set")
	}
	cmd, err := e.expandShellNames(cmd)
	if err != nil {
		return nil, err
	}
	out, err := e.runShellCmd(cmd, "", e.bangCols(), e.bangRows())
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return nil, fmt.Errorf("%s: %v", cmd, err)
		}
	}
	return splitOutputLines(out), nil
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
