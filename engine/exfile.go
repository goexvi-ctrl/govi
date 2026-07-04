package engine

import (
	"fmt"
	"path/filepath"
	"strings"
)

// fitStatus truncates an over-long status line from the front with a leading
// "..." so it fits cols columns. The file name leads these lines, so dropping
// leading characters drops leading path components, like nvi's msgq_status. It
// runs at render time (view.Message) with the live terminal width, so it honors
// window resizes.
func fitStatus(line string, cols int) string {
	r := []rune(line)
	if cols < 4 || len(r) <= cols {
		return line
	}
	return "..." + string(r[len(r)-(cols-3):])
}

// fileStatus builds the :f / ^G status line (nvi msgq_status).
func (e *Engine) fileStatus() string {
	s := e.scr
	name := s.name
	if name == "" {
		name = "[No file]"
	}
	var b strings.Builder
	b.WriteString(name)
	b.WriteString(": ")
	if e.scr.showFileCount && len(e.scr.argv) > 1 {
		fmt.Fprintf(&b, "%d files to edit: ", len(e.scr.argv))
		e.scr.showFileCount = false
	}
	needSep := false
	if s.name == "" && s.dirty() {
		b.WriteString("new file")
		needSep = true
	} else {
		if s.nameChanged {
			b.WriteString("name changed")
			needSep = true
		}
		if s.dirty() {
			if needSep {
				b.WriteString(", ")
			}
			b.WriteString("modified")
			needSep = true
		} else {
			if needSep {
				b.WriteString(", ")
			}
			b.WriteString("unmodified")
			needSep = true
		}
	}
	if s.opts.Bool("readonly") {
		if needSep {
			b.WriteString(", ")
		}
		b.WriteString("readonly")
		needSep = true
	}
	if needSep {
		b.WriteString(": ")
	}
	n := s.lineCount()
	if n <= 1 && s.lineLen(1) == 0 {
		b.WriteString("empty file")
		return b.String()
	}
	pct := int64(0)
	if n > 0 {
		pct = (s.cursor.Line * 100) / n
	}
	fmt.Fprintf(&b, "line %d of %d [%d%%]", s.cursor.Line, n, pct)
	return b.String()
}

// noteAltName records name as the alternate file (# / ^^) when the buffer has a
// current file distinct from it -- nvi's set_alt_name rule 1 (common/exf.c): an
// ex command that takes a file-name argument (:w file, :r file, and even a :e
// that then fails) sets the alternate name to that argument. A successful :e
// instead sets it to the previous file (rule 2), which Open handles and which
// overrides this because Open runs afterward.
func (e *Engine) noteAltName(name string) {
	if name != "" && e.scr.name != "" && !e.samePath(name, e.scr.name) {
		e.scr.altFile = name
	}
}

// exFile implements :f[ile] [name] — show status and optionally rename the buffer.
func (e *Engine) exFile(c *exCmd) error {
	name := strings.TrimSpace(c.arg)
	if name != "" {
		old := e.scr.name
		if old != "" && old != name {
			e.scr.altFile = old
			e.scr.nameChanged = true
		}
		e.scr.name = name
		e.fe.SetTitle(filepath.Base(name))
	}
	e.scr.msg = e.fileStatus()
	e.scr.msgKind = MsgInfo
	return nil
}

// File-list ex commands: :edit, :next, :previous/:rewind, :args. These move
// among the files named on the command line (the argument list), mirroring
// nvi's behavior.

// exEdit implements :e[!] [file] -- edit a file, replacing the current buffer.
// With no name it re-edits the current file (discarding changes with !).
// splitPlusCmd peels a leading "+cmd" argument off an :edit/:next argument
// (nvi FL_ALL / the +command form). It returns the ex command to run once the
// file is loaded ("" when there is none) and the remaining argument (the file
// name). A bare "+" means the last line ("$"), matching the command-line form.
func splitPlusCmd(arg string) (cmd, rest string) {
	if !strings.HasPrefix(arg, "+") {
		return "", arg
	}
	rs := []rune(arg[1:])
	var b strings.Builder
	i := 0
	for i < len(rs) {
		if rs[i] == '\\' && i+1 < len(rs) { // \<space> keeps the space in the cmd
			b.WriteRune(rs[i+1])
			i += 2
			continue
		}
		if rs[i] == ' ' || rs[i] == '\t' {
			break
		}
		b.WriteRune(rs[i])
		i++
	}
	cmd = b.String()
	if cmd == "" {
		cmd = "$"
	}
	for i < len(rs) && (rs[i] == ' ' || rs[i] == '\t') {
		i++
	}
	return cmd, string(rs[i:])
}

func (e *Engine) exEdit(c *exCmd) error {
	plusCmd, arg := splitPlusCmd(strings.TrimSpace(c.arg))
	path := strings.TrimSpace(arg)
	if path != "" {
		// Expand %/# and shell metacharacters: ":e #" re-edits the alternate
		// file, ":e %" the current one, ":e f*" the unique match (nvi
		// argv_exp2). :edit takes exactly one file, so a glob matching several
		// is a usage error, like nvi.
		names, err := e.expandFileArgs(path)
		if err != nil {
			return err
		}
		if len(names) != 1 {
			return c.usageError()
		}
		path = names[0]
		// nvi set_alt_name rule 1: the target becomes the alternate name even if
		// the edit then fails the modification check; a successful edit overrides
		// this in Open (rule 2: alt = the file being left).
		if !c.newScreen {
			e.noteAltName(path)
		}
	}
	// :E[dit] (capitalized) opens the file -- or the current file when no name is
	// given -- in a new split screen, leaving the current screen untouched.
	if c.newScreen {
		if err := e.editNewScreen(path); err != nil {
			return err
		}
		e.runPlusCmd(plusCmd)
		return nil
	}
	if path == "" {
		path = e.scr.name
		if path == "" {
			return fmt.Errorf("No current filename")
		}
	}
	if e.scr.dirty() && !c.force {
		return fmt.Errorf("No write since last change (use :e! to override)")
	}
	// Re-editing the current file (":e!" to discard changes) restores the cursor
	// to where it was, rather than homing to line 1 (nvi remembers the position
	// across the reload). Editing a different file starts at the top as usual.
	reediting := plusCmd == "" && e.samePath(path, e.scr.name)
	savedCursor := e.scr.cursor
	if err := e.Open(path); err != nil {
		return err
	}
	if reediting {
		e.scr.cursor = savedCursor
		e.scr.clampCursor()
	}
	// A "+cmd" argument runs once the file is loaded (:e +N file, :e +/pat file),
	// like the command-line +cmd form (nvi ex_edit -> the same c_option path).
	e.runPlusCmd(plusCmd)
	return nil
}

// runPlusCmd runs the ex command peeled off an :edit/:next "+cmd" argument, on
// the freshly loaded file. Empty cmd is a no-op. Failures land on the status
// line rather than aborting the (already successful) file switch.
func (e *Engine) runPlusCmd(cmd string) {
	if cmd == "" {
		return
	}
	e.runColon(cmd)
	// nvi positions a freshly loaded file so the +cmd's landing line is the top
	// of the window (as "vi +N file" does from the command line), rather than
	// leaving the window at line 1 with the cursor part-way down.
	s := e.scr
	s.top = clampLine(s, s.cursor.Line)
}

// exNext implements :n[!] -- edit the next file in the argument list. :N
// (capitalized) edits the next file in a new split screen.
// checkModified is nvi's file_m1 guard (common/exf.c), used by the
// file-switching commands (:next/:prev/:rewind, the tag jumps, ^^): when
// autowrite is set, an unforced switch away from a modified buffer writes it
// instead of failing. The write is skipped for a readonly session (System V
// behavior, which nvi follows), falling through to the error.
func (e *Engine) checkModified(force bool, msg string) error {
	if !e.scr.dirty() || force {
		return nil
	}
	if e.scr.opts.Bool("autowrite") && !e.scr.opts.Bool("readonly") {
		return e.Save("")
	}
	return fmt.Errorf("%s", msg)
}

func (e *Engine) exNext(c *exCmd) error {
	if e.scr.argIdx+1 >= len(e.scr.argv) {
		return fmt.Errorf("No more files to edit")
	}
	if c.newScreen {
		// :N edits the parent's next file in a new screen, which inherits the
		// arglist positioned at that file; the parent screen is left untouched.
		return e.editArgNewScreen(e.scr.argIdx + 1)
	}
	if err := e.checkModified(c.force, "No write since last change (use :n! to override)"); err != nil {
		return err
	}
	e.scr.argIdx++
	return e.Open(e.scr.argv[e.scr.argIdx])
}

// exPrev implements :prev[!] -- edit the previous file in the list. :Prev
// (capitalized) edits it in a new split screen.
func (e *Engine) exPrev(c *exCmd) error {
	if e.scr.argIdx <= 0 {
		return fmt.Errorf("No previous files to edit")
	}
	if c.newScreen {
		return e.editArgNewScreen(e.scr.argIdx - 1)
	}
	if err := e.checkModified(c.force, "No write since last change (use :prev! to override)"); err != nil {
		return err
	}
	e.scr.argIdx--
	return e.Open(e.scr.argv[e.scr.argIdx])
}

// editArgNewScreen opens the parent's argv[idx] file in a new horizontal split
// (nvi :N/:P in a new screen). Like :E, the new screen starts with an empty
// argument list; the parent screen keeps its own arglist position untouched.
func (e *Engine) editArgNewScreen(idx int) error {
	parentArgv := e.scr.argv
	if idx < 0 || idx >= len(parentArgv) {
		return fmt.Errorf("No more files to edit")
	}
	return e.editNewScreen(parentArgv[idx])
}

// exRewind implements :rewind[!] -- edit the first file in the list.
func (e *Engine) exRewind(c *exCmd) error {
	if err := e.checkModified(c.force, "No write since last change (use :rewind! to override)"); err != nil {
		return err
	}
	if len(e.scr.argv) == 0 {
		return fmt.Errorf("No files to edit")
	}
	e.scr.argIdx = 0
	return e.Open(e.scr.argv[0])
}

// exArgs implements :args -- show the argument list with the current file in
// brackets.
func (e *Engine) exArgs(c *exCmd) error {
	if len(e.scr.argv) == 0 {
		e.scr.msg, e.scr.msgKind = "No files", MsgInfo
		return nil
	}
	parts := make([]string, len(e.scr.argv))
	for i, a := range e.scr.argv {
		name := filepath.Base(a)
		if i == e.scr.argIdx {
			name = "[" + name + "]"
		}
		parts[i] = name
	}
	e.scr.msg, e.scr.msgKind = strings.Join(parts, " "), MsgInfo
	return nil
}
