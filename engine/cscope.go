package engine

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// cscope support: :cscope (alias :cs) drives one or more cscope subprocesses,
// each kept running in line-oriented mode ("cscope -dl -f cscope.out"), and
// turns "cscope find" queries into tag jumps that integrate with the tag stack
// (^T) and :tagnext/:tagprev navigation. This mirrors nvi's ex/ex_cscope.c.
//
// A connection talks to its subprocess over two pipes: we write "<n><pattern>\n"
// (n is the query number) and read back "cscope: <count> lines\n", that many
// result lines, and a ">> " prompt. Each result line is
//
//	<filename> <context> <line number> <pattern>

const (
	cscopeDBFile = "cscope.out"   // default database name (nvi CSCOPE_DBFILE)
	cscopePaths  = "cscope.tpath" // optional file of result search paths
	cscopePrompt = ">> "          // cscope line-mode prompt (nvi CSCOPE_PROMPT)
)

// cscopeQueries maps a "find" type letter to its cscope query number by index:
// the position of the letter in this string is the number sent to cscope (nvi
// CSCOPE_QUERIES, where index 5 -- a space -- is the unused "change" query).
const cscopeQueries = "sgdct efi"

// findHelp is the usage text for "cscope find" (nvi FINDHELP).
const findHelp = `find c|d|e|f|g|i|s|t buffer|pattern
      c: find callers of name
      d: find all function calls made from name
      e: find pattern
      f: find files with name as substring
      g: find definition of name
      i: find files #including name
      s: find all uses of name
      t: find assignments to name`

// cscopeConn is one running cscope subprocess connection (nvi CSC).
type cscopeConn struct {
	dir   string    // directory the database lives in (csc->dname)
	paths []string  // directories to search for result file names
	mtime time.Time // database mtime; files older than it use the line number
	cmd   *exec.Cmd
	in    io.WriteCloser // cscope's stdin (we write queries here)
	out   *bufio.Reader  // cscope's stdout (we read results here)
}

// cscopeCmd is one "cscope" subcommand (nvi CC).
type cscopeCmd struct {
	name  string
	fn    func(e *Engine, arg string, force bool) error
	help  string
	usage string
}

func cscopeCmds() []cscopeCmd {
	return []cscopeCmd{
		{"add", (*Engine).cscopeAdd, "Add a new cscope database", "add file | directory"},
		{"find", func(e *Engine, arg string, force bool) error { return e.cscopeFind(arg, force) },
			"Query the databases for a pattern", findHelp},
		{"help", func(e *Engine, arg string, _ bool) error { return e.cscopeHelp(arg) },
			"Show help for cscope commands", "help [command]"},
		{"kill", func(e *Engine, arg string, _ bool) error { return e.cscopeKill(arg) },
			"Kill a cscope connection", "kill number"},
		{"reset", func(e *Engine, _ string, _ bool) error { e.cscopeReset(); return nil },
			"Discard all current cscope connections", "reset"},
	}
}

// lookupCscopeCmd returns the subcommand matched by name (a unique prefix, nvi
// lookup_ccmd: first table entry the name is a prefix of).
func lookupCscopeCmd(name string) *cscopeCmd {
	cmds := cscopeCmds()
	for i := range cmds {
		if strings.HasPrefix(cmds[i].name, name) {
			return &cmds[i]
		}
	}
	return nil
}

// exCscope implements :cscope (:cs) command [args] -- dispatch to a subcommand.
func (e *Engine) exCscope(c *exCmd) error {
	// Pick up CSCOPE_DIRS once, like nvi's start_cscopes.
	if !e.cscopeInit {
		e.cscopeInit = true
		e.startCscopesFromEnv(c.force)
	}

	arg := strings.TrimSpace(c.arg)
	sub, rest := splitFirstWord(arg)
	if sub == "" {
		return fmt.Errorf(`Use "cscope help" for help`)
	}
	ccp := lookupCscopeCmd(sub)
	if ccp == nil {
		return fmt.Errorf(`Use "cscope help" for help`)
	}
	return ccp.fn(e, rest, c.force)
}

// startCscopesFromEnv adds a connection for each directory in CSCOPE_DIRS (nvi
// EXTENSION #1), a tab/space/colon separated list, when the variable is set.
func (e *Engine) startCscopesFromEnv(force bool) {
	dirs := os.Getenv("CSCOPE_DIRS")
	if dirs == "" {
		return
	}
	for _, d := range strings.FieldsFunc(dirs, func(r rune) bool {
		return r == '\t' || r == ' ' || r == ':'
	}) {
		// Best effort, like nvi: a bad directory in the list is skipped.
		_ = e.cscopeAdd(d, force)
	}
}

// splitFirstWord splits s into its first whitespace-delimited word and the
// remainder (leading blanks of the remainder trimmed).
func splitFirstWord(s string) (word, rest string) {
	s = strings.TrimLeft(s, " \t")
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimLeft(s[i:], " \t")
}

// cscopeAdd implements "cscope add file|dir": start a cscope process on the
// database in the given directory (or the given database file).
func (e *Engine) cscopeAdd(arg string, force bool) error {
	dname := strings.TrimSpace(arg)
	if dname == "" {
		return e.cscopeHelp("add")
	}
	// Expand %/#/glob like nvi's argv_exp2; "add" takes exactly one name.
	names, err := e.expandFileArgs(dname)
	if err != nil {
		return err
	}
	if len(names) > 1 {
		return fmt.Errorf("%s: expanded into too many file names", dname)
	}
	if len(names) == 1 {
		dname = names[0]
	}

	// The argument may be a database file or a directory containing one. Resolve
	// to the directory that holds the database and the database file name.
	target := e.resolvePath(dname)
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	var dir, dbname string
	if info.IsDir() {
		dir = target
		dbname = cscopeDBFile
		if _, err := os.Stat(filepath.Join(dir, dbname)); err != nil {
			return err
		}
	} else {
		dir = filepath.Dir(target)
		dbname = filepath.Base(target)
	}

	dbinfo, err := os.Stat(filepath.Join(dir, dbname))
	if err != nil {
		return err
	}

	conn := &cscopeConn{dir: dir, mtime: dbinfo.ModTime()}
	conn.paths = cscopeGetPaths(dir)
	if err := conn.start(dbname); err != nil {
		return err
	}
	if err := conn.readPrompt(); err != nil {
		conn.terminate()
		return fmt.Errorf("%s: %v", dir, err)
	}
	e.cscopes = append(e.cscopes, conn)
	return nil
}

// cscopeGetPaths returns the directories to search for files named in cscope
// results (nvi get_paths): the colon-separated entries of a cscope.tpath file in
// the database directory if present, otherwise the database directory itself.
func cscopeGetPaths(dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, cscopePaths))
	if err == nil {
		var paths []string
		for _, p := range strings.Split(string(data), ":") {
			if p = strings.TrimSpace(p); p != "" {
				paths = append(paths, p)
			}
		}
		if len(paths) > 0 {
			return paths
		}
	}
	return []string{dir}
}

// start forks the cscope subprocess in line-oriented mode (nvi run_cscope):
// "cd <dir> && exec cscope -dl -f <dbname>".
func (c *cscopeConn) start(dbname string) error {
	cmd := exec.Command("cscope", "-dl", "-f", dbname)
	cmd.Dir = c.dir
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout // nvi dups stderr to the same pipe
	if err := cmd.Start(); err != nil {
		return err
	}
	c.cmd = cmd
	c.in = in
	c.out = bufio.NewReader(out)
	return nil
}

// readPrompt reads and discards output up to and including the next ">> " prompt
// (nvi read_prompt).
func (c *cscopeConn) readPrompt() error {
	for {
		b, err := c.out.ReadByte()
		if err != nil {
			return err
		}
		if b != cscopePrompt[0] {
			continue
		}
		if b2, err := c.out.ReadByte(); err != nil {
			return err
		} else if b2 != cscopePrompt[1] {
			continue
		}
		if b3, err := c.out.ReadByte(); err != nil {
			return err
		} else if b3 != cscopePrompt[2] {
			continue
		}
		return nil
	}
}

// atPrompt reports whether the next three bytes are the ">> " prompt, without
// consuming them. It blocks until at least three bytes are available, which is
// safe at a response boundary: cscope always emits a full response (count line
// and results, or an error line) terminated by the prompt.
func (c *cscopeConn) atPrompt() bool {
	b, err := c.out.Peek(len(cscopePrompt))
	return err == nil && string(b) == cscopePrompt
}

// consumePrompt discards a ">> " prompt the caller has already confirmed with
// atPrompt.
func (c *cscopeConn) consumePrompt() error {
	_, err := c.out.Discard(len(cscopePrompt))
	return err
}

// terminate detaches from the cscope subprocess, closing its pipes and reaping
// it (nvi terminate).
func (c *cscopeConn) terminate() {
	if c.in != nil {
		c.in.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Wait()
	}
}

// cscopeReset terminates and discards every cscope connection (nvi cscope_reset).
func (e *Engine) cscopeReset() {
	for _, c := range e.cscopes {
		c.terminate()
	}
	e.cscopes = nil
}

// cscopeKill implements "cscope kill number": terminate the n'th connection
// (1-based, nvi cscope_kill).
func (e *Engine) cscopeKill(arg string) error {
	n, err := strconv.Atoi(strings.TrimSpace(arg))
	if err != nil || n < 1 || n > len(e.cscopes) {
		return fmt.Errorf("%s: no such cscope session", strings.TrimSpace(arg))
	}
	c := e.cscopes[n-1]
	c.terminate()
	e.cscopes = append(e.cscopes[:n-1], e.cscopes[n:]...)
	return nil
}

// cscopeHelp implements "cscope help [command]" (nvi csc_help).
func (e *Engine) cscopeHelp(arg string) error {
	arg = strings.TrimSpace(arg)
	if arg != "" {
		ccp := lookupCscopeCmd(arg)
		if ccp == nil {
			e.showOutput([]string{arg + " doesn't match any cscope command"})
			return nil
		}
		e.showOutput([]string{
			fmt.Sprintf("Command: %s (%s)", ccp.name, ccp.help),
			fmt.Sprintf("  Usage: %s", ccp.usage),
		})
		return nil
	}
	lines := []string{"cscope commands:"}
	for _, ccp := range cscopeCmds() {
		lines = append(lines, fmt.Sprintf("  %5s: %s", ccp.name, ccp.help))
	}
	e.showOutput(lines)
	return nil
}

// cscopeDisplay lists the running connections (nvi cscope_display), backing
// ":display connections".
func (e *Engine) cscopeDisplay() error {
	if len(e.cscopes) == 0 {
		e.showOutput([]string{"No cscope connections."})
		return nil
	}
	var lines []string
	for i, c := range e.cscopes {
		pid := 0
		if c.cmd != nil && c.cmd.Process != nil {
			pid = c.cmd.Process.Pid
		}
		lines = append(lines, fmt.Sprintf("%2d %s (process %d)", i+1, c.dir, pid))
	}
	e.showOutput(lines)
	return nil
}
