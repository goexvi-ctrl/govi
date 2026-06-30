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
	newScreen    bool // command was given capitalized: act in a new split screen
}

// exCmdDef describes one ex command: its full name, the minimum number of
// leading characters needed to abbreviate it, and its handler.
type exCmdDef struct {
	full      string
	min       int
	summary   string // short description for :exusage
	usage     string // usage template for :exusage cmd
	fn        func(*Engine, *exCmd) error
	newScreen bool // capable of acting in a new screen when capitalized (nvi E_NEWSCREEN)
}

// exCmds is populated in init() rather than as a static initializer: some
// command handlers (global) call back into exExecute -> findCmd -> exCmds, and
// a static initializer would form an initialization cycle.
var exCmds []exCmdDef

func init() {
	exCmds = []exCmdDef{
		{full: "delete", min: 1, fn: (*Engine).exDelete},
		{full: "move", min: 1, fn: (*Engine).exMove},
		{full: "copy", min: 2, fn: (*Engine).exCopy},
		{full: "t", min: 1, fn: (*Engine).exCopy},
		{full: "yank", min: 1, fn: (*Engine).exYank},
		{full: "put", min: 2, fn: (*Engine).exPut},
		{full: "join", min: 1, fn: (*Engine).exJoin},
		{full: "write", min: 1, fn: (*Engine).exWrite},
		{full: "wn", min: 2, fn: (*Engine).exWriteNext},
		{full: "wq", min: 2, fn: (*Engine).exWriteQuit},
		{full: "xit", min: 1, fn: (*Engine).exXit},
		{full: "quit", min: 1, fn: (*Engine).exQuit},
		{full: "read", min: 1, fn: (*Engine).exRead},
		{full: ">", min: 1, fn: (*Engine).exShiftRight},
		{full: "<", min: 1, fn: (*Engine).exShiftLeft},
		{full: "=", min: 1, fn: (*Engine).exLineNumber},
		{full: "substitute", min: 1, fn: (*Engine).exSubstitute},
		{full: "global", min: 1, fn: (*Engine).exGlobal},
		{full: "vglobal", min: 1, fn: (*Engine).exVglobal},
		{full: "set", min: 2, fn: (*Engine).exSet},
		{full: "source", min: 2, fn: (*Engine).exSource},
		{full: "map", min: 3, fn: (*Engine).exMap},
		{full: "unmap", min: 3, fn: (*Engine).exUnmap},
		{full: "abbreviate", min: 2, fn: (*Engine).exAbbreviate},
		{full: "unabbreviate", min: 3, fn: (*Engine).exUnabbreviate},
		{full: "edit", min: 1, fn: (*Engine).exEdit, newScreen: true},
		{full: "next", min: 1, fn: (*Engine).exNext, newScreen: true},
		{full: "previous", min: 4, fn: (*Engine).exPrev, newScreen: true},
		{full: "rewind", min: 3, fn: (*Engine).exRewind},
		{full: "args", min: 2, fn: (*Engine).exArgs},
		{full: "cd", min: 2, fn: (*Engine).exCd},
		{full: "chdir", min: 2, fn: (*Engine).exCd},
		{full: "file", min: 1, fn: (*Engine).exFile},
		{full: "tag", min: 2, fn: (*Engine).exTag, newScreen: true},
		{full: "preserve", min: 3, fn: (*Engine).exPreserve},
		{full: "recover", min: 3, fn: (*Engine).exRecover},
		{full: "shell", min: 2, fn: (*Engine).exShell},
		{full: "stop", min: 4, fn: (*Engine).exStop},
		{full: "suspend", min: 3, fn: (*Engine).exStop},
		{full: "version", min: 2, fn: (*Engine).exVersion},
		{full: "!", min: 1, fn: (*Engine).exBang},
		{full: "&", min: 1, fn: (*Engine).exAmp},
		{full: "~", min: 1, fn: (*Engine).exTilde},
		{full: "k", min: 1, fn: (*Engine).exMark},
		{full: "mark", min: 2, fn: (*Engine).exMark},
		{full: "print", min: 1, fn: (*Engine).exPrint},
		{full: "number", min: 2, fn: (*Engine).exNumber},
		{full: "list", min: 1, fn: (*Engine).exList},
		{full: "visual", min: 2, fn: (*Engine).exVisual, newScreen: true},
		{full: "vsplit", min: 2, fn: (*Engine).exVsplit},
		{full: "bg", min: 2, fn: (*Engine).exBg},
		{full: "fg", min: 2, fn: (*Engine).exFg, newScreen: true},
		{full: "resize", min: 3, fn: (*Engine).exResize},
		{full: "display", min: 2, fn: (*Engine).exDisplay},
		{full: "append", min: 1, fn: (*Engine).exAppend},
		{full: "insert", min: 1, fn: (*Engine).exInsert},
		{full: "change", min: 1, fn: (*Engine).exChange},
		{full: "help", min: 2, fn: (*Engine).exHelp},
		{full: "exusage", min: 3, fn: (*Engine).exExusage},
		{full: "viusage", min: 3, fn: (*Engine).exViusage},
		{full: "undo", min: 1, fn: (*Engine).exUndo},
		{full: "@", min: 1, fn: (*Engine).exAt},
		{full: "*", min: 1, fn: (*Engine).exAt},
		{full: "#", min: 1, fn: (*Engine).exNumber},
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
	err error // set by address parsing (e.g. a search address that fails)
}

func (e *Engine) parseEx(line string) (*exCmd, error) {
	p := &exParser{e: e, s: []rune(line)}
	p.skipBlanks()
	// Permit (and ignore) leading colons, e.g. ":g/re/:p" or a sourced script
	// line written as ":2d". nvi strips any run of leading colons (ex.c: "any
	// command could have preceding colons").
	for p.peek() == ':' {
		p.next()
	}
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
	if p.err != nil {
		return nil, p.err
	}

	p.skipBlanks()
	name := p.parseName()
	if name == "" {
		if c.addrCount == 0 {
			return nil, nil
		}
		return c, nil // address-only: goto
	}

	// A capital first letter on one of the screen commands requests that the
	// command act in a new split screen (nvi ex.c: "Capital letters beginning the
	// command names ex, edit, next, previous, tag and visual indicate the command
	// should happen in a new screen."). Lower-case it, then look it up.
	newScreen := false
	switch name[0] {
	case 'E', 'F', 'N', 'P', 'T', 'V':
		newScreen = true
		name = strings.ToLower(name[:1]) + name[1:]
	}

	def, err := findCmd(name)
	if err != nil {
		return nil, err
	}
	// The capitalized form resolved to a command that cannot open a new screen
	// (e.g. :P -> print); run it normally, like nvi (which drops the new-screen
	// request for print/preserve).
	if newScreen && !def.newScreen {
		newScreen = false
	}
	c.def = def
	c.newScreen = newScreen

	if p.peek() == '!' {
		p.next()
		c.force = true
	}
	p.skipBlanks()
	c.arg = strings.TrimSpace(string(p.s[p.pos:]))
	return c, nil
}

// parseName reads the command name: a run of letters, or a single special-char
// command (< > = & ~ # * ! @).
func (p *exParser) parseName() string {
	if p.eof() {
		return ""
	}
	switch p.peek() {
	case '<', '>', '=', '&', '~', '#', '*', '!', '@':
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
	case r == '/' || r == '?':
		// Search address: /pat/ scans forward from the line after the current
		// line, ?pat? backward from the line before; both wrap (wrapscan). The
		// closing delimiter is optional. An empty pattern reuses the last one.
		delim := p.next()
		dir := searchFwd
		if delim == '?' {
			dir = searchBack
		}
		pat := p.readDelimited(delim)
		lno, err := p.e.searchAddr(pat, cur, dir)
		if err != nil {
			p.err = err
			return 0, false
		}
		return lno, true
	}
	return 0, false
}

// readDelimited reads up to the next unescaped delim (which it consumes if
// present) and returns the text between. A backslash-escaped delimiter becomes a
// literal delimiter; other backslash escapes are preserved for the regex.
func (p *exParser) readDelimited(delim rune) string {
	var b strings.Builder
	for !p.eof() {
		r := p.next()
		if r == delim {
			return b.String()
		}
		if r == '\\' && !p.eof() {
			if p.peek() == delim {
				b.WriteRune(delim)
				p.next()
				continue
			}
			b.WriteRune('\\')
			b.WriteRune(p.next())
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (p *exParser) parseNumber() (int64, bool) {
	start := p.pos
	var n int64
	for !p.eof() && p.peek() >= '0' && p.peek() <= '9' {
		n = n*10 + int64(p.next()-'0')
	}
	return n, p.pos > start
}

func (p *exParser) eof() bool { return p.pos >= len(p.s) }
func (p *exParser) peek() rune {
	if p.eof() {
		return 0
	}
	return p.s[p.pos]
}
func (p *exParser) next() rune { r := p.s[p.pos]; p.pos++; return r }
func (p *exParser) skipBlanks() {
	for !p.eof() && (p.peek() == ' ' || p.peek() == '\t') {
		p.pos++
	}
}

// gotoLine moves the cursor to the first non-blank of line lno.
func (e *Engine) gotoLine(lno int64) {
	lno = clampLine(e.scr, lno)
	e.scr.cursor = Pos{Line: lno, Col: e.scr.firstNonBlank(lno)}
	e.scr.clampCursor()
}
