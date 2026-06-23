package engine

import (
	"fmt"
	"strings"
)

// ex command parsing and dispatch. This corresponds to nvi's ex parser and
// command table (ex/ex.c, ex/ex_cmd.c). Like the vi machine it lives in the
// engine package because it operates directly on the screen/buffer state; the
// Frontend/View embedding boundary is unaffected.

// exCmd is a parsed ex command ready to execute.
type exCmd struct {
	addr1, addr2 int64 // resolved 1-based line addresses
	addrCount    int   // number of addresses the user supplied (0, 1, 2)
	force        bool  // the ! flag
	buffer       rune  // register name, or 0
	arg          string
	def          *exCmdDef
}

// exCmdDef describes one ex command: its full name, the minimum number of
// leading characters needed to abbreviate it, and its handler.
type exCmdDef struct {
	full string
	min  int
	fn   func(*Engine, *exCmd) error
}

// exCmds is populated in init() rather than as a static initializer: some
// command handlers (global) call back into exExecute -> findCmd -> exCmds, and
// a static initializer would form an initialization cycle.
var exCmds []exCmdDef

func init() {
	exCmds = []exCmdDef{
		{"delete", 1, (*Engine).exDelete},
		{"move", 1, (*Engine).exMove},
		{"copy", 2, (*Engine).exCopy},
		{"t", 1, (*Engine).exCopy},
		{"yank", 1, (*Engine).exYank},
		{"put", 2, (*Engine).exPut},
		{"join", 1, (*Engine).exJoin},
		{"write", 1, (*Engine).exWrite},
		{"wq", 2, (*Engine).exWriteQuit},
		{"xit", 1, (*Engine).exXit},
		{"quit", 1, (*Engine).exQuit},
		{"read", 1, (*Engine).exRead},
		{">", 1, (*Engine).exShiftRight},
		{"<", 1, (*Engine).exShiftLeft},
		{"=", 1, (*Engine).exLineNumber},
		{"substitute", 1, (*Engine).exSubstitute},
		{"global", 1, (*Engine).exGlobal},
		{"vglobal", 1, (*Engine).exVglobal},
		{"set", 2, (*Engine).exSet},
		{"map", 3, (*Engine).exMap},
		{"unmap", 3, (*Engine).exUnmap},
		{"abbreviate", 2, (*Engine).exAbbreviate},
		{"unabbreviate", 4, (*Engine).exUnabbreviate},
		{"edit", 1, (*Engine).exEdit},
		{"next", 1, (*Engine).exNext},
		{"previous", 4, (*Engine).exPrev},
		{"Next", 1, (*Engine).exPrev},
		{"rewind", 3, (*Engine).exRewind},
		{"args", 2, (*Engine).exArgs},
		{"tag", 2, (*Engine).exTag},
		{"preserve", 3, (*Engine).exPreserve},
		{"recover", 3, (*Engine).exRecover},
		{"!", 1, (*Engine).exBang},
		{"&", 1, (*Engine).exAmp},
		{"print", 1, (*Engine).exPrint},
		{"number", 2, (*Engine).exNumber},
		{"list", 1, (*Engine).exList},
		{"visual", 2, (*Engine).exVisual},
		{"append", 1, (*Engine).exAppend},
		{"insert", 1, (*Engine).exInsert},
		{"change", 1, (*Engine).exChange},
	}
}

// runColon executes the ex command typed on the vi colon line.
func (e *Engine) runColon(line string) {
	if err := e.exExecute(line); err != nil {
		e.scr.msg, e.scr.msgKind = err.Error(), MsgError
	}
}

func (e *Engine) exExecute(line string) error {
	c, err := e.parseEx(line)
	if err != nil {
		return err
	}
	if c == nil {
		return nil
	}
	if c.def == nil {
		if c.addrCount > 0 {
			// In ex (line) mode a bare address prints the line(s) and makes the
			// last one current; in vi it just moves the cursor (the line is
			// already visible on screen).
			if e.scr.mode == ModeExText {
				return e.exPrintLines(c.addr1, c.addr2)
			}
			e.gotoLine(c.addr2)
			return nil
		}
		return nil
	}
	return c.def.fn(e, c)
}

type exParser struct {
	e   *Engine
	s   []rune
	pos int
}

func (e *Engine) parseEx(line string) (*exCmd, error) {
	p := &exParser{e: e, s: []rune(line)}
	p.skipBlanks()
	if p.eof() {
		return nil, nil
	}

	c := &exCmd{}
	cur := e.scr.cursor.Line

	// '%' is shorthand for the whole file (1,$).
	if p.peek() == '%' {
		p.next()
		c.addr1, c.addr2, c.addrCount = 1, e.scr.lineCount(), 2
	} else {
		if a1, ok := p.parseAddr(cur); ok {
			c.addr1, c.addr2, c.addrCount = a1, a1, 1
			p.skipBlanks()
			for p.peek() == ',' || p.peek() == ';' {
				sep := p.next()
				if sep == ';' {
					cur = c.addr1
				}
				p.skipBlanks()
				if a2, ok := p.parseAddr(cur); ok {
					c.addr1 = c.addr2
					c.addr2 = a2
					c.addrCount = 2
				} else {
					c.addr2 = cur
					c.addrCount = 2
				}
				p.skipBlanks()
			}
		}
	}

	p.skipBlanks()
	name := p.parseName()
	if name == "" {
		if c.addrCount == 0 {
			return nil, nil
		}
		return c, nil // address-only: goto
	}

	def, err := findCmd(name)
	if err != nil {
		return nil, err
	}
	c.def = def

	if p.peek() == '!' {
		p.next()
		c.force = true
	}
	p.skipBlanks()
	c.arg = strings.TrimSpace(string(p.s[p.pos:]))
	return c, nil
}

// parseName reads the command name: a run of letters, or a single special-char
// command (< > = & ~ # *).
func (p *exParser) parseName() string {
	if p.eof() {
		return ""
	}
	switch p.peek() {
	case '<', '>', '=', '&', '~', '#', '*', '!':
		return string(p.next())
	}
	var b strings.Builder
	for !p.eof() {
		r := p.peek()
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			b.WriteRune(r)
			p.next()
			continue
		}
		break
	}
	return b.String()
}

func findCmd(name string) (*exCmdDef, error) {
	for i := range exCmds {
		d := &exCmds[i]
		if len(name) >= d.min && len(name) <= len(d.full) && strings.HasPrefix(d.full, name) {
			return d, nil
		}
	}
	return nil, fmt.Errorf("The %s command is unknown", name)
}

// --- address parsing ---

func (p *exParser) parseAddr(cur int64) (int64, bool) {
	p.skipBlanks()
	base, ok := p.parseAddrBase(cur)
	if !ok {
		// A bare +/- offset is relative to the current line.
		if p.peek() == '+' || p.peek() == '-' {
			base, ok = cur, true
		} else {
			return 0, false
		}
	}
	// Offsets: +N, -N (repeatable).
	for {
		p.skipBlanks()
		r := p.peek()
		if r != '+' && r != '-' {
			break
		}
		p.next()
		p.skipBlanks()
		n, hadNum := p.parseNumber()
		if !hadNum {
			n = 1
		}
		if r == '+' {
			base += n
		} else {
			base -= n
		}
	}
	return base, true
}

func (p *exParser) parseAddrBase(cur int64) (int64, bool) {
	if p.eof() {
		return 0, false
	}
	switch r := p.peek(); {
	case r >= '0' && r <= '9':
		n, _ := p.parseNumber()
		return n, true
	case r == '.':
		p.next()
		return cur, true
	case r == '$':
		p.next()
		return p.e.scr.lineCount(), true
	case r == '\'':
		p.next()
		if p.eof() {
			return 0, false
		}
		name := p.next()
		if mk, ok := p.e.scr.marks.Get(name); ok {
			return mk.Line, true
		}
		return 0, false
	}
	return 0, false
}

func (p *exParser) parseNumber() (int64, bool) {
	start := p.pos
	var n int64
	for !p.eof() && p.peek() >= '0' && p.peek() <= '9' {
		n = n*10 + int64(p.next()-'0')
	}
	return n, p.pos > start
}

func (p *exParser) eof() bool      { return p.pos >= len(p.s) }
func (p *exParser) peek() rune     { if p.eof() { return 0 }; return p.s[p.pos] }
func (p *exParser) next() rune     { r := p.s[p.pos]; p.pos++; return r }
func (p *exParser) skipBlanks()    { for !p.eof() && (p.peek() == ' ' || p.peek() == '\t') { p.pos++ } }

// gotoLine moves the cursor to the first non-blank of line lno.
func (e *Engine) gotoLine(lno int64) {
	lno = clampLine(e.scr, lno)
	e.scr.cursor = Pos{Line: lno, Col: e.scr.firstNonBlank(lno)}
	e.scr.clampCursor()
}
