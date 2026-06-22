package engine

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

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
	tl := clampLine(s, l1)
	s.cursor = Pos{Line: tl, Col: s.firstNonBlank(tl)}
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
	// Join l1..l2 into one line.
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
			sep = []rune{' '}
		}
		s.setLine(l1, append(append(a, sep...), b...))
		s.deleteLine(l1 + 1)
		l2--
	}
	e.endChange()
	s.cursor = Pos{Line: clampLine(s, l1), Col: 0}
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
	e.deleteLines(l1, l2)
	if dest > l2 {
		dest -= n
	}
	for i, ln := range lines {
		s.appendLine(dest+int64(i), ln)
	}
	e.endChange()
	s.cursor = Pos{Line: clampLine(s, dest+n), Col: 0}
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

func (e *Engine) exShiftRight(c *exCmd) error { return e.shift(c, +1) }
func (e *Engine) exShiftLeft(c *exCmd) error  { return e.shift(c, -1) }

func (e *Engine) shift(c *exCmd, dir int) error {
	l1, l2, err := e.rangeOf(c)
	if err != nil {
		return err
	}
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
	s.cursor = Pos{Line: clampLine(s, l2), Col: s.firstNonBlank(clampLine(s, l2))}
	return nil
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
	e.scr.msg = strconv.FormatInt(n, 10)
	e.scr.msgKind = MsgInfo
	return nil
}

func (e *Engine) exWrite(c *exCmd) error {
	path := strings.TrimSpace(c.arg)
	if path == "" {
		path = e.scr.name
	}
	if path == "" {
		return fmt.Errorf("No current filename")
	}
	n, b, err := e.writeFile(path)
	if err != nil {
		return err
	}
	if path == e.scr.name {
		e.scr.modified = false
	}
	e.scr.msg = fmt.Sprintf("%q: %d lines, %d bytes", filepath.Base(path), n, b)
	e.scr.msgKind = MsgInfo
	return nil
}

func (e *Engine) exWriteQuit(c *exCmd) error {
	if err := e.exWrite(c); err != nil {
		return err
	}
	e.quit = true
	return nil
}

// exXit implements :x and ZZ -- write only if the buffer was modified, then quit.
func (e *Engine) exXit(c *exCmd) error {
	if e.scr.modified {
		if err := e.exWrite(c); err != nil {
			return err
		}
	}
	e.quit = true
	return nil
}

func (e *Engine) exQuit(c *exCmd) error {
	if e.scr.modified && !c.force {
		return fmt.Errorf("No write since last change (use ! to override)")
	}
	e.quit = true
	return nil
}

func (e *Engine) exRead(c *exCmd) error {
	path := strings.TrimSpace(c.arg)
	if path == "" {
		return fmt.Errorf("Filename required")
	}
	data, err := readFileLines(path)
	if err != nil {
		return err
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
