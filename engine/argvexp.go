package engine

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// expandFileArgs expands a file-name argument the way nvi's argv_exp2 does
// (ex/ex_argv.c): first %/# filename substitution (argv_fexp), then shell word
// expansion. It returns the resulting list of file names.
//
// Mirroring nvi: after substitution the string is scanned for the first
// `shellmeta` character. With none, the argument is split on whitespace
// (argv_exp3). A bare trailing '*' (the only metacharacter, last in the string)
// triggers internal filename-prefix completion (argv_lexp). Any other
// metacharacter forks the user's shell to expand it (argv_sexp). A failed or
// empty expansion reports nvi's "Shell expansion failed".
func (e *Engine) expandFileArgs(arg string) ([]string, error) {
	// %/# substitution, identical to argv_fexp's filename characters.
	expanded, err := e.expandShellNames(arg)
	if err != nil {
		return nil, err
	}

	// Find the first shellmeta character (nvi's metacharacter scan).
	meta := e.scr.opts.Str("shellmeta")
	rs := []rune(expanded)
	metaIdx := -1
	if meta != "" {
		for i, r := range rs {
			if strings.ContainsRune(meta, r) {
				metaIdx = i
				break
			}
		}
	}

	switch {
	case metaIdx < 0:
		// No metacharacters: split on whitespace (argv_exp3).
		return strings.Fields(expanded), nil
	case len(rs)-metaIdx == 1 && rs[metaIdx] == '*':
		// A single trailing '*': internal prefix completion (argv_lexp).
		return e.globPrefix(string(rs[:metaIdx]))
	default:
		// Anything else: fork the shell to expand it (argv_sexp).
		return e.shellExpand(expanded)
	}
}

// globPrefix implements nvi's argv_lexp: return every file in the prefix's
// directory whose name begins with the prefix's final component, sorted. With no
// final component (the prefix ends in '/', or is empty) it lists the directory,
// skipping dot files, exactly as nvi does. An empty result is an error.
func (e *Engine) globPrefix(prefix string) ([]string, error) {
	dir, base := ".", prefix
	prependDir := false
	if i := strings.LastIndex(prefix, "/"); i >= 0 {
		prependDir = true
		if i == 0 {
			dir = "/"
		} else {
			dir = prefix[:i]
		}
		base = prefix[i+1:]
	}
	entries, err := os.ReadDir(e.resolvePath(dir))
	if err != nil {
		return nil, err
	}
	var matches []string
	for _, ent := range entries {
		n := ent.Name()
		if base == "" {
			if strings.HasPrefix(n, ".") {
				continue
			}
		} else if !strings.HasPrefix(n, base) {
			continue
		}
		switch {
		case !prependDir:
			matches = append(matches, n)
		case dir == "/":
			matches = append(matches, "/"+n)
		default:
			matches = append(matches, dir+"/"+n)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("Shell expansion failed")
	}
	sort.Strings(matches)
	return matches, nil
}

// shellExpand implements nvi's argv_sexp: run "echo <pattern>" through the user's
// shell (the `shell` option, defaulted from $SHELL), capturing standard output
// only -- standard error is discarded, as historic vi did, since unmatched
// patterns produce noise. The whitespace-separated words of the output are the
// expansion. A shell failure or blank output reports "Shell expansion failed",
// so behavior tracks whatever the user's shell does with an unmatched pattern.
func (e *Engine) shellExpand(pattern string) ([]string, error) {
	if e.scr.opts.Bool("secure") {
		return nil, fmt.Errorf("Shell expansions not supported when the secure edit option is set")
	}
	out, err := e.runShellStdout("echo " + pattern)
	if err != nil {
		return nil, fmt.Errorf("Shell expansion failed")
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return nil, fmt.Errorf("Shell expansion failed")
	}
	return fields, nil
}
