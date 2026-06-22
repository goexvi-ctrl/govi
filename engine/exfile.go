package engine

import (
	"fmt"
	"path/filepath"
	"strings"
)

// File-list ex commands: :edit, :next, :previous/:rewind, :args. These move
// among the files named on the command line (the argument list), mirroring
// nvi's behavior.

// exEdit implements :e[!] [file] -- edit a file, replacing the current buffer.
// With no name it re-edits the current file (discarding changes with !).
func (e *Engine) exEdit(c *exCmd) error {
	path := strings.TrimSpace(c.arg)
	if path == "" {
		path = e.scr.name
		if path == "" {
			return fmt.Errorf("No current filename")
		}
	}
	if e.scr.modified && !c.force {
		return fmt.Errorf("No write since last change (use :e! to override)")
	}
	return e.Open(path)
}

// exNext implements :n[!] -- edit the next file in the argument list.
func (e *Engine) exNext(c *exCmd) error {
	if e.scr.modified && !c.force {
		return fmt.Errorf("No write since last change (use :n! to override)")
	}
	if e.argIdx+1 >= len(e.argv) {
		return fmt.Errorf("No more files to edit")
	}
	e.argIdx++
	return e.Open(e.argv[e.argIdx])
}

// exPrev implements :prev[!] / :N -- edit the previous file in the list.
func (e *Engine) exPrev(c *exCmd) error {
	if e.scr.modified && !c.force {
		return fmt.Errorf("No write since last change (use :prev! to override)")
	}
	if e.argIdx <= 0 {
		return fmt.Errorf("No previous files to edit")
	}
	e.argIdx--
	return e.Open(e.argv[e.argIdx])
}

// exRewind implements :rewind[!] -- edit the first file in the list.
func (e *Engine) exRewind(c *exCmd) error {
	if e.scr.modified && !c.force {
		return fmt.Errorf("No write since last change (use :rewind! to override)")
	}
	if len(e.argv) == 0 {
		return fmt.Errorf("No files to edit")
	}
	e.argIdx = 0
	return e.Open(e.argv[0])
}

// exArgs implements :args -- show the argument list with the current file in
// brackets.
func (e *Engine) exArgs(c *exCmd) error {
	if len(e.argv) == 0 {
		e.scr.msg, e.scr.msgKind = "No files", MsgInfo
		return nil
	}
	parts := make([]string, len(e.argv))
	for i, a := range e.argv {
		name := filepath.Base(a)
		if i == e.argIdx {
			name = "[" + name + "]"
		}
		parts[i] = name
	}
	e.scr.msg, e.scr.msgKind = strings.Join(parts, " "), MsgInfo
	return nil
}
