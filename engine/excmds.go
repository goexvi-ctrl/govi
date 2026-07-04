package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"govi/engine/mark"
	"govi/engine/register"
)

// ex command implementations. Each receives a parsed exCmd with resolved
// addresses (defaulting to the current line when none were given) and the
// command's argument text.

const exDefaultShiftwidth = 8

// rangeOf returns the line range for a command, defaulting to the current line
// when the user gave no address. A trailing count argument (consumed from arg)
// converts the range to [addr2, addr2+count-1], as in vi.
func (e *Engine) rangeOf(c *exCmd) (int64, int64, error) {
	l1, l2 := c.addr1, c.addr2
	if c.addrCount == 0 {
		l1 = e.scr.cursor.Line
		l2 = l1
	}
	if cnt, rest, ok := leadingCount(c.arg); ok {
		l1 = l2
		l2 = l1 + cnt - 1
		c.arg = strings.TrimSpace(rest)
	}
	if l1 < 1 || l2 > e.scr.lineCount() || l1 > l2 {
		return 0, 0, fmt.Errorf("Invalid address")
	}
	return l1, l2, nil
}

// rangeNoCount is like rangeOf but never consumes a trailing count from arg; it
// is used by commands (move, copy) whose argument is a destination address.
func (e *Engine) rangeNoCount(c *exCmd) (int64, int64, error) {
	l1, l2 := c.addr1, c.addr2
	if c.addrCount == 0 {
		l1 = e.scr.cursor.Line
		l2 = l1
	}
	if l1 < 1 || l2 > e.scr.lineCount() || l1 > l2 {
		return 0, 0, fmt.Errorf("Invalid address")
	}
	return l1, l2, nil
}

func leadingCount(s string) (int64, string, bool) {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, s, false
	}
	n, _ := strconv.ParseInt(s[:i], 10, 64)
	return n, s[i:], true
}

func (e *Engine) exDelete(c *exCmd) error {
	l1, l2, err := e.rangeOf(c)
	if err != nil {
		return err
	}
	s := e.scr
	txt := e.collectLines(l1, l2)
	txt.Kind = register.LineWise
	e.beginChange()
	s.regs.StoreDelete(c.buffer, txt)
	e.deleteLines(l1, l2)
	e.endChange()
	// nvi's ex_delete sets only the line, leaving the column where it was
	// (unlike interactive dd or :move/:copy); it does not jump to first-nonblank.
	tl := clampLine(s, l1)
	col := s.cursor.Col
	if n := s.lineLen(tl); col > n-1 {
		col = n - 1
	}
	if col < 0 {
		col = 0
	}
	s.cursor = Pos{Line: tl, Col: col}
	return nil
}

func (e *Engine) exYank(c *exCmd) error {
	l1, l2, err := e.rangeOf(c)
	if err != nil {
		return err
	}
	txt := e.collectLines(l1, l2)
	txt.Kind = register.LineWise
	e.scr.regs.StoreYank(c.buffer, txt)
	return nil
}

func (e *Engine) exPut(c *exCmd) error {
	s := e.scr
	at := c.addr2
	if c.addrCount == 0 {
		at = s.cursor.Line
	}
	txt := s.regs.Get(c.buffer)
	if txt.Empty() {
		return fmt.Errorf("The buffer is empty")
	}
	e.beginChange()
	for i, ln := range txt.Lines {
		s.appendLine(at+int64(i), cloneR(ln))
	}
	e.endChange()
	last := at + int64(len(txt.Lines))
	s.cursor = Pos{Line: clampLine(s, last), Col: 0}
	return nil
}

// exMark implements :[line]k<char> and :[line]mark <char> -- set mark <char> on
// the addressed line (default current), matching the vi m command (nvi ex_mark).
func (e *Engine) exMark(c *exCmd) error {
	name := strings.TrimSpace(c.arg)
	if len([]rune(name)) != 1 {
		return fmt.Errorf("Usage: mark <character>")
	}
	lno := c.addr2
	if c.addrCount == 0 {
		lno = e.scr.cursor.Line
	}
	e.scr.marks.Set([]rune(name)[0], mark.Mark{Line: lno, Col: 0})
	return nil
}

func (e *Engine) exJoin(c *exCmd) error {
	l1, l2, err := e.rangeOf(c)
	if err != nil {
		return err
	}
	if l1 == l2 {
		l2 = l1 + 1 // ":j" with one line joins it with the next
	}
	if l2 > e.scr.lineCount() {
		return fmt.Errorf("Invalid address")
	}
	s := e.scr
	e.beginChange()
	// Join l1..l2 into one line, tracking the column of the last join boundary so
	// the cursor lands there (nvi/vi J: on the inserted separator, or the last
	// char of the first part when no separator is added with !).
	joinCol := 0
	for l1 < l2 {
		a := cloneR(s.lineRunes(l1))
		b := s.lineRunes(l1 + 1)
		i := 0
		for i < len(b) && (b[i] == ' ' || b[i] == '\t') {
			i++
		}
		b = b[i:]
		var sep []rune
		if !c.force && len(a) > 0 && a[len(a)-1] != ' ' && a[len(a)-1] != '\t' && (len(b) == 0 || b[0] != ')') {
			if ch := a[len(a)-1]; ch == '.' || ch == '?' || ch == '!' {
				sep = []rune{' ', ' '}
			} else {
				sep = []rune{' '}
			}
		}
		if len(sep) > 0 {
			joinCol = len(a) // the inserted blank
		} else if len(a) > 0 {
			joinCol = len(a) - 1 // last char of the first part
		} else {
			joinCol = 0
		}
		s.setLine(l1, append(append(a, sep...), b...))
		s.deleteLine(l1 + 1)
		l2--
	}
	e.endChange()
	s.cursor = Pos{Line: clampLine(s, l1), Col: joinCol}
	s.clampCursor()
	return nil
}

func (e *Engine) exMove(c *exCmd) error {
	l1, l2, err := e.rangeNoCount(c)
	if err != nil {
		return err
	}
	dest, ok := e.resolveAddrArg(c.arg)
	if !ok {
		return fmt.Errorf("Destination address required")
	}
	if dest >= l1 && dest <= l2 {
		return fmt.Errorf("Destination is inside the source range")
	}
	s := e.scr
	lines := e.collectLinesRaw(l1, l2)
	n := l2 - l1 + 1
	e.beginChange()
	// nvi appends the copy at the destination first and deletes the shifted
	// source after; the order is observable through a running global's
	// last-edit line (ex_g_insdel), so keep it.
	for i, ln := range lines {
		s.appendLine(dest+int64(i), ln)
	}
	cur := dest // moving down: the block settles at dest-n+1..dest
	src1, src2 := l1, l2
	if dest < l1 {
		src1, src2 = l1+n, l2+n
		cur = dest + n // moving up: the block settles at dest+1..dest+n
	}
	e.deleteLines(src1, src2)
	e.endChange()
	s.cursor = Pos{Line: clampLine(s, cur), Col: 0}
	return nil
}

func (e *Engine) exCopy(c *exCmd) error {
	l1, l2, err := e.rangeNoCount(c)
	if err != nil {
		return err
	}
	dest, ok := e.resolveAddrArg(c.arg)
	if !ok {
		return fmt.Errorf("Destination address required")
	}
	s := e.scr
	lines := e.collectLinesRaw(l1, l2)
	e.beginChange()
	for i, ln := range lines {
		s.appendLine(dest+int64(i), ln)
	}
	e.endChange()
	s.cursor = Pos{Line: clampLine(s, dest+int64(len(lines))), Col: 0}
	return nil
}

func (e *Engine) exShiftRight(c *exCmd) error { return e.shift(c, '>') }
func (e *Engine) exShiftLeft(c *exCmd) error  { return e.shift(c, '<') }

func (e *Engine) shift(c *exCmd, ch rune) error {
	// Repeated shift characters add levels: :1>> shifts twice, :1<<< thrice
	// (nvi). The first char was consumed as the command name; count the rest.
	levels := 1
	a := []rune(strings.TrimSpace(c.arg))
	i := 0
	for i < len(a) && a[i] == ch {
		levels++
		i++
	}
	c.arg = strings.TrimSpace(string(a[i:]))
	l1, l2, err := e.rangeOf(c)
	if err != nil {
		return err
	}
	dir := levels
	if ch == '<' {
		dir = -levels
	}
	e.shiftLines(l1, l2, dir)
	s := e.scr
	s.cursor = Pos{Line: clampLine(s, l2), Col: s.firstNonBlank(clampLine(s, l2))}
	return nil
}

// shiftLines shifts the indentation of lines [l1,l2] by dir shiftwidths (dir +1
// right, -1 left), rebuilding the leading whitespace with tabs to the tabstop.
// Shared by the ex < > commands and the vi < > operators.
func (e *Engine) shiftLines(l1, l2 int64, dir int) {
	s := e.scr
	ts := s.opts.Int("tabstop")
	sw := s.opts.Int("shiftwidth")
	e.beginChange()
	for ln := l1; ln <= l2; ln++ {
		line := s.lineRunes(ln)
		// Measure and strip existing leading whitespace.
		width, i := 0, 0
		for i < len(line) {
			if line[i] == ' ' {
				width++
				i++
			} else if line[i] == '\t' {
				width += ts - width%ts
				i++
			} else {
				break
			}
		}
		if i == len(line) {
			continue // blank line: vi leaves it unchanged
		}
		newWidth := width + dir*sw
		if newWidth < 0 {
			newWidth = 0
		}
		s.setLine(ln, append(makeIndent(newWidth, ts), line[i:]...))
	}
	e.endChange()
}

// makeIndent builds leading whitespace of the given display width using tabs to
// the tabstop then spaces, matching vi's default (no expandtab) indentation.
func makeIndent(width, tabstop int) []rune {
	tabs := width / tabstop
	spaces := width % tabstop
	out := make([]rune, 0, tabs+spaces)
	for i := 0; i < tabs; i++ {
		out = append(out, '\t')
	}
	for i := 0; i < spaces; i++ {
		out = append(out, ' ')
	}
	return out
}

func (e *Engine) exLineNumber(c *exCmd) error {
	n := c.addr2
	if c.addrCount == 0 {
		n = e.scr.lineCount()
	}
	// nvi ex_equal emits the number as ex output (ex_printf), not a status
	// message, so := still prints in batch mode where messages are silenced.
	e.printLine(strconv.FormatInt(n, 10))
	return nil
}

func (e *Engine) exWrite(c *exCmd) error {
	arg := strings.TrimSpace(c.arg)
	// :[range]w !cmd pipes the addressed lines (default the whole file) to a
	// shell command's standard input; the buffer is not written or modified.
	if strings.HasPrefix(arg, "!") {
		cmd := strings.TrimSpace(arg[1:])
		if cmd == "" {
			return fmt.Errorf("Usage: [range]w !command")
		}
		return e.writeToCommand(c, cmd)
	}
	// :[range]w >> file appends the addressed lines to file (nvi LF_APPEND).
	// Detect and strip the ">>" before file-name expansion.
	appendMode := false
	if strings.HasPrefix(arg, ">>") {
		appendMode = true
		arg = strings.TrimSpace(arg[2:])
	}
	if arg != "" {
		// Expand %/#/glob characters in the target name (nvi ex_write.c calls
		// argv_exp2). :w takes one file, so several matches is an error.
		names, err := e.expandFileArgs(arg)
		if err != nil {
			return err
		}
		if len(names) > 1 {
			return fmt.Errorf("%s: expanded into too many file names", arg)
		}
		if len(names) == 1 {
			arg = names[0]
		}
	}
	named := arg != ""
	// nvi readonly guard (common/exf.c file_write): writing the buffer's own file
	// is refused when `readonly` is set (set by the user, or by the lock option
	// when another process holds the file), unless forced. Writing to a different
	// name is allowed -- readonly is a property of the edit session's file.
	if !c.force && (arg == "" || e.samePath(arg, e.scr.name)) && e.scr.opts.Bool("readonly") {
		return fmt.Errorf("Read-only file, not written; use ! to override")
	}
	// nvi overwrite guard (common/exf.c file_write): refuse to clobber an
	// existing file that is not the buffer's own file (or whose name was changed
	// via :f) unless the write is forced (:w!) or the `writeany` option is set.
	// Without this, :w other-existing-file silently overwrites it (data loss).
	// Appending (>>) does not clobber, so nvi skips this check for it.
	if !c.force && !appendMode && !e.scr.opts.Bool("writeany") {
		target := arg
		if target == "" {
			target = e.scr.name
		}
		noname := arg == "" || e.samePath(arg, e.scr.name)
		if target != "" && (!noname || e.scr.nameChanged) {
			if _, err := os.Stat(e.resolvePath(target)); err == nil {
				return fmt.Errorf("%s exists, not written; use ! to override", target)
			}
		}
	}
	// An appended write, or a write of a partial line range, writes only the
	// addressed lines and never clears the modified flag or touches recovery/lock
	// state (it is not a full save of the buffer to its own file). The plain
	// whole-file :w goes through Save, which keeps that bookkeeping.
	l1, l2 := int64(1), e.scr.lineCount()
	if c.addrCount > 0 {
		l1, l2 = c.addr1, c.addr2
	}
	if appendMode || l1 != 1 || l2 != e.scr.lineCount() {
		target := arg
		if target == "" {
			target = e.scr.name
		}
		if target == "" {
			return fmt.Errorf("No current filename")
		}
		n, b, err := e.writeRange(e.resolvePath(target), l1, l2, appendMode)
		if err != nil {
			return err
		}
		verb := "written"
		if appendMode {
			verb = "appended"
		}
		e.scr.msg = fmt.Sprintf("%q: %d lines, %d bytes %s", filepath.Base(target), n, b, verb)
		e.scr.msgKind = MsgInfo
		return nil
	}
	// Save resolves the name against the current directory at write time; pass it
	// through as given so the buffer's name stays relative (nvi FR_NAME).
	if err := e.Save(arg); err != nil {
		return err
	}
	// Writing a temporary buffer (govi -g, no file) to a real, named file makes
	// it an ordinary buffer that is no longer discarded on exit.
	if named && e.scr.tempFile {
		e.scr.name = arg
		e.scr.tempFile = false
		e.scr.modified = false
		e.scr.nameChanged = false
	}
	return nil
}

// SetTemporary marks the active buffer as backed by a throwaway temp file, so
// quitting warns (and discards) like nvi instead of silently writing it. Set by
// the GUI host for `govi -g` with no file (see GoviSetTemporary).
func (e *Engine) SetTemporary() { e.scr.tempFile = true }

// IsTemporary reports whether the active buffer is a throwaway temp file, so the
// GUI host can warn that closing discards it (it has no real file name).
func (e *Engine) IsTemporary() bool { return e.scr.tempFile }

// TempDiscardPending reports whether an unforced exit would discard a temporary
// buffer that holds content. Unlike a modified check, this stays true after a
// plain :w -- writing only updates the throwaway temp, which is still discarded
// on exit -- matching nvi (which warns after a write too). False for an empty
// temp buffer (nothing to lose) and for non-temp buffers.
func (e *Engine) TempDiscardPending() bool {
	if !e.scr.tempFile {
		return false
	}
	s := e.scr
	return s.store.Lines() > 1 || len(s.lineRunes(1)) > 0
}

// tempExitWarning returns nvi's warning when an unforced exit would discard a
// temporary buffer's content. The temp file is removed when its window closes,
// so writing it is pointless: the user must :w a real file, or :q!/ZQ to discard.
func (e *Engine) tempExitWarning(force bool) error {
	if !force && e.TempDiscardPending() {
		return fmt.Errorf("File is a temporary; exit will discard modifications")
	}
	return nil
}

// SaveAs renames the buffer to path and writes it there, like :f path followed
// by :w. A temp buffer becomes an ordinary file at the new path; the throwaway
// vi.XXXXXX on disk is left for the host to remove when the tab closes.
func (e *Engine) SaveAs(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("No filename")
	}
	old := e.scr.name
	if old != "" && old != path {
		e.scr.altFile = old
		e.scr.nameChanged = true
	}
	e.scr.name = path // as given; Save resolves it at write time
	e.fe.SetTitle(filepath.Base(path))
	if err := e.Save(path); err != nil {
		return err
	}
	if e.scr.tempFile {
		e.scr.tempFile = false
		e.scr.modified = false
		e.scr.nameChanged = false
	}
	return nil
}

// Save writes the buffer to path (or the current file when path is empty).
// An untitled buffer adopts path as its name on the first successful save.
func (e *Engine) Save(path string) error {
	// `given` is the name as typed, or the buffer's current as-given name. It is
	// resolved against the current directory only here, at write time, so a :cd
	// between editing and :w sends the file to the new directory (nvi behavior).
	given := strings.TrimSpace(path)
	if given == "" {
		given = e.scr.name
	}
	if given == "" {
		return fmt.Errorf("No current filename")
	}
	target := e.resolvePath(given)
	// Back up the file's current contents first when the backup option is set
	// (nvi file_backup), before the write replaces it.
	if err := e.makeBackup(target); err != nil {
		return err
	}
	n, b, err := e.writeFile(target)
	if err != nil {
		return err
	}
	if e.scr.name == "" {
		e.scr.name = given
	}
	if e.samePath(given, e.scr.name) {
		e.scr.modified = false
		e.scr.nameChanged = false
		e.removeRecovery() // saved: no recovery needed
		// writeFile renamed a temp over the file, replacing the inode the lock
		// was held on; re-take it so the session keeps the lock across saves.
		e.relockAfterWrite(target)
	}
	e.scr.msg = fmt.Sprintf("%q: %d lines, %d bytes", filepath.Base(target), n, b)
	e.scr.msgKind = MsgInfo
	return nil
}

// samePath reports whether two buffer names refer to the same file once each is
// resolved against the editor's current directory. Names are kept as given
// (nvi FR_NAME), so a plain string compare is not enough.
func (e *Engine) samePath(a, b string) bool {
	if a == b {
		return true
	}
	return e.resolvePath(a) == e.resolvePath(b)
}

func (e *Engine) exWriteQuit(c *exCmd) error {
	// :wq with no name on a temporary buffer would write the throwaway temp file;
	// warn instead. :wq file writes (and adopts) a real name, so allow it.
	if strings.TrimSpace(c.arg) == "" {
		if err := e.tempExitWarning(c.force); err != nil {
			return err
		}
	}
	if err := e.exWrite(c); err != nil {
		return err
	}
	e.finishQuit()
	return nil
}

// exWriteNext implements :wn[ext] -- write the current file, then edit the next
// file in the argument list (nvi ex_next.c, the write-and-advance form).
func (e *Engine) exWriteNext(c *exCmd) error {
	if e.scr.argIdx+1 >= len(e.scr.argv) {
		return fmt.Errorf("No more files to edit")
	}
	if err := e.exWrite(c); err != nil {
		return err
	}
	e.scr.argIdx++
	return e.Open(e.scr.argv[e.scr.argIdx])
}

// exUndo implements :u[ndo] -- undo the last change. Like nvi's ex_undo, this
// shares the undo/redo direction toggle with the vi-mode 'u' command (ep->lundo),
// so alternating :undo commands alternate undo and redo.
func (e *Engine) exUndo(c *exCmd) error {
	e.vi.doUndoCommand(e, false)
	return nil
}

// exAt implements :@ buffer / :* buffer -- execute the named buffer's contents
// as EX commands (nvi ex_at.c). With no buffer, or @@/@*, reuse the last executed
// buffer (the SC_AT_SET / at_lbuf state shared with vi-mode @). Unlike vi-mode @,
// the buffer's lines run as ex commands, not as vi keys.
func (e *Engine) exAt(c *exCmd) error {
	name := []rune(strings.TrimSpace(c.arg))
	b := rune('@')
	if len(name) > 0 {
		b = name[0]
	}
	if b == '@' || b == '*' {
		if e.scr.lastAtBuf == 0 {
			return fmt.Errorf("No previous buffer to execute")
		}
		b = e.scr.lastAtBuf
	}
	if !validBufferName(b) {
		return fmt.Errorf("Buffers should be specified using the English alphabet")
	}
	txt := e.scr.regs.Get(b)
	if txt.Empty() {
		return fmt.Errorf("Buffer %c is empty", b)
	}
	e.scr.lastAtBuf = b
	for _, ln := range txt.Lines {
		if err := e.exExecute(string(ln)); err != nil {
			return err
		}
		if e.quit {
			break
		}
	}
	return nil
}

// exXit implements :x and ZZ -- write only if the buffer was modified, then quit.
func (e *Engine) exXit(c *exCmd) error {
	if strings.TrimSpace(c.arg) == "" {
		if err := e.tempExitWarning(c.force); err != nil {
			return err
		}
	}
	if e.scr.dirty() {
		if err := e.exWrite(c); err != nil {
			return err
		}
	}
	e.finishQuit()
	return nil
}

func (e *Engine) exQuit(c *exCmd) error {
	if err := e.tempExitWarning(c.force); err != nil {
		return err
	}
	if e.scr.dirty() && !c.force {
		return fmt.Errorf("No write since last change (use ! to override)")
	}
	e.finishQuit() // close just this screen if others are displayed, else exit
	return nil
}

func (e *Engine) exRead(c *exCmd) error {
	arg := strings.TrimSpace(c.arg)
	if arg == "" {
		return fmt.Errorf("Filename required")
	}
	// :[line]r !cmd inserts a shell command's output instead of a file.
	var data [][]rune
	if strings.HasPrefix(arg, "!") {
		cmd := strings.TrimSpace(arg[1:])
		if cmd == "" {
			return fmt.Errorf("Usage: [line]r !command")
		}
		lines, err := e.readFromCommand(cmd)
		if err != nil {
			return err
		}
		data = lines
	} else {
		// Expand %/#/glob characters in the source name (nvi ex_read.c calls
		// argv_exp2). :r reads one file, so several matches is an error.
		names, err := e.expandFileArgs(arg)
		if err != nil {
			return err
		}
		if len(names) > 1 {
			return fmt.Errorf("%s: expanded into too many file names", arg)
		}
		if len(names) == 1 {
			arg = names[0]
		}
		lines, err := readFileLines(e.resolvePath(arg))
		if err != nil {
			return err
		}
		data = lines
	}
	s := e.scr
	at := c.addr2
	if c.addrCount == 0 {
		at = s.cursor.Line
	}
	e.beginChange()
	for i, ln := range data {
		s.appendLine(at+int64(i), ln)
	}
	e.endChange()
	if len(data) > 0 {
		s.cursor = Pos{Line: clampLine(s, at+1), Col: 0}
	}
	// nvi reports the count when more than 'report' lines are added.
	if n := len(data); n > s.opts.Int("report") {
		s.msg = fmt.Sprintf("%d lines added", n)
		s.msgKind = MsgInfo
	}
	return nil
}

// collectLinesRaw returns deep copies of lines [l1, l2] as rune slices.
func (e *Engine) collectLinesRaw(l1, l2 int64) [][]rune {
	out := make([][]rune, 0, l2-l1+1)
	for i := l1; i <= l2; i++ {
		out = append(out, cloneR(e.scr.lineRunes(i)))
	}
	return out
}

// resolveAddrArg parses a single address (used for move/copy destinations).
func (e *Engine) resolveAddrArg(arg string) (int64, bool) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return 0, false
	}
	p := &exParser{e: e, s: []rune(arg)}
	if arg[0] == '$' {
		return e.scr.lineCount(), true
	}
	return p.parseAddr(e.scr.cursor.Line)
}
