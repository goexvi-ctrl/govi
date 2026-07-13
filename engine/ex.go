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
	absAddr      bool  // an address was "non-relative" (number, $, 'mark, /?): sets ''
	force        bool  // the ! flag
	buffer       rune  // register name, or 0
	arg          string
	def          *exCmdDef
	newScreen    bool   // command was given capitalized: act in a new split screen
	pipeRest     string // text after an unescaped '|' separator, run as the next command
	autoprint    bool   // handler requests autoprint for this invocation (the
	// range form of :!, nvi ex_bang setting E_AUTOPRINT dynamically)
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
	autoprint bool // print the new current line afterward in ex mode (nvi E_AUTOPRINT)
	// wholeLine: '|' is not a command separator for this command -- it takes the
	// rest of the line (nvi: !, global, v, and the first-arg-ex commands edit, ex,
	// next, visual, vsplit; read/write are this way only for their '!' filter form,
	// handled in code). substArg: the argument begins with a delimited RE (s / & /
	// ~), so '|' inside the RE is literal and only a '|' after the flags splits.
	wholeLine bool
	substArg  bool
}

// exCmds is populated in init() rather than as a static initializer: some
// command handlers (global) call back into exExecute -> findCmd -> exCmds, and
// a static initializer would form an initialization cycle.
var exCmds []exCmdDef

func init() {
	exCmds = []exCmdDef{
		{full: "delete", min: 1, fn: (*Engine).exDelete, autoprint: true},
		{full: "move", min: 1, fn: (*Engine).exMove, autoprint: true},
		{full: "copy", min: 2, fn: (*Engine).exCopy, autoprint: true},
		{full: "t", min: 1, fn: (*Engine).exCopy, autoprint: true},
		{full: "yank", min: 1, fn: (*Engine).exYank},
		{full: "put", min: 2, fn: (*Engine).exPut, autoprint: true},
		{full: "join", min: 1, fn: (*Engine).exJoin, autoprint: true},
		{full: "write", min: 1, fn: (*Engine).exWrite},
		{full: "wn", min: 2, fn: (*Engine).exWriteNext},
		{full: "wq", min: 2, fn: (*Engine).exWriteQuit},
		{full: "xit", min: 1, fn: (*Engine).exXit},
		{full: "quit", min: 1, fn: (*Engine).exQuit},
		{full: "read", min: 1, fn: (*Engine).exRead},
		{full: ">", min: 1, fn: (*Engine).exShiftRight, autoprint: true},
		{full: "<", min: 1, fn: (*Engine).exShiftLeft, autoprint: true},
		{full: "=", min: 1, fn: (*Engine).exLineNumber},
		{full: "substitute", min: 1, fn: (*Engine).exSubstitute, substArg: true},
		{full: "global", min: 1, fn: (*Engine).exGlobal, wholeLine: true},
		{full: "vglobal", min: 1, fn: (*Engine).exVglobal, wholeLine: true},
		{full: "set", min: 2, fn: (*Engine).exSet},
		{full: "source", min: 2, fn: (*Engine).exSource},
		{full: "map", min: 3, fn: (*Engine).exMap},
		{full: "unmap", min: 3, fn: (*Engine).exUnmap},
		{full: "abbreviate", min: 2, fn: (*Engine).exAbbreviate},
		{full: "unabbreviate", min: 3, fn: (*Engine).exUnabbreviate},
		{full: "edit", min: 1, fn: (*Engine).exEdit, newScreen: true, wholeLine: true},
		{full: "ex", min: 2, fn: (*Engine).exEdit, newScreen: true, wholeLine: true}, // nvi: :ex is an alias of :edit (both are ex_edit)
		{full: "next", min: 1, fn: (*Engine).exNext, newScreen: true, wholeLine: true},
		{full: "previous", min: 4, fn: (*Engine).exPrev, newScreen: true},
		{full: "rewind", min: 3, fn: (*Engine).exRewind},
		{full: "args", min: 2, fn: (*Engine).exArgs},
		{full: "cd", min: 2, fn: (*Engine).exCd},
		{full: "chdir", min: 2, fn: (*Engine).exCd},
		{full: "file", min: 1, fn: (*Engine).exFile},
		{full: "tag", min: 2, fn: (*Engine).exTag, newScreen: true},
		{full: "tagnext", min: 4, fn: (*Engine).exTagNext},
		{full: "tagpop", min: 4, fn: (*Engine).exTagPop},
		{full: "tagprev", min: 4, fn: (*Engine).exTagPrev},
		{full: "tagtop", min: 4, fn: (*Engine).exTagTop},
		{full: "cscope", min: 2, fn: (*Engine).exCscope},
		{full: "preserve", min: 3, fn: (*Engine).exPreserve},
		{full: "recover", min: 3, fn: (*Engine).exRecover},
		{full: "shell", min: 2, fn: (*Engine).exShell},
		{full: "stop", min: 4, fn: (*Engine).exStop},
		{full: "suspend", min: 3, fn: (*Engine).exStop},
		{full: "version", min: 2, fn: (*Engine).exVersion},
		{full: "!", min: 1, fn: (*Engine).exBang, wholeLine: true},
		{full: "&", min: 1, fn: (*Engine).exAmp},
		{full: "~", min: 1, fn: (*Engine).exTilde},
		{full: "k", min: 1, fn: (*Engine).exMark},
		{full: "mark", min: 2, fn: (*Engine).exMark},
		{full: "print", min: 1, fn: (*Engine).exPrint},
		{full: "number", min: 2, fn: (*Engine).exNumber},
		{full: "list", min: 1, fn: (*Engine).exList},
		{full: "visual", min: 2, fn: (*Engine).exVisual, newScreen: true, wholeLine: true},
		{full: "vsplit", min: 2, fn: (*Engine).exVsplit, wholeLine: true},
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
		{full: "undo", min: 1, fn: (*Engine).exUndo, autoprint: true},
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
	// A non-relative address (number, $, 'mark, /?) records the previous context
	// at the pre-command position, whether the command is a bare goto (:15) or an
	// addressed command (:15d). nvi does this in the ex loop before execution
	// (ex/ex.c E_ABSMARK); it deliberately does not fire for . + - or %.
	if c.absAddr {
		e.setPrevContext(e.scr.cursor, Pos{}, absAlways)
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
	if err := c.def.fn(e, c); err != nil {
		return err
	}
	// autoprint: in ex (line) mode, commands flagged E_AUTOPRINT echo the new
	// current line when the autoprint option is set (nvi ex.c). It is suppressed
	// inside a :global (gMarks non-nil) and does not apply to vi colon commands.
	if (c.def.autoprint || c.autoprint) && e.scr.mode == ModeExText && e.scr.gMarks == nil && !e.exSilent &&
		e.scr.opts.Bool("autoprint") && e.scr.store.Lines() > 0 {
		e.printLine(string(e.scr.lineRunes(e.scr.cursor.Line)))
	}
	// Run any command after a '|' separator (nvi ex.c command loop).
	if c.pipeRest != "" {
		return e.exExecute(c.pipeRest)
	}
	return nil
}

type exParser struct {
	e       *Engine
	s       []rune
	pos     int
	err     error // set by address parsing (e.g. a search address that fails)
	absAddr bool  // a non-relative address base was seen (nvi E_ABSMARK)
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
	// A reversed two-address range is a specific error (nvi ex.c: this is checked
	// in the address parser, before the command runs, for any 2-address command).
	if c.addrCount == 2 && c.addr2 < c.addr1 {
		return nil, fmt.Errorf("The second address is smaller than the first")
	}
	c.absAddr = p.absAddr

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

	// A '!' after :! is not the force flag but the start of the shell command
	// (":!!" reruns the previous bang command via argument expansion).
	if p.peek() == '!' && def.full != "!" {
		p.next()
		c.force = true
	}
	p.skipBlanks()
	c.arg, c.pipeRest = p.parseArgPipe(c)
	return c, nil
}

// parseArgPipe reads a command's argument, honoring '|' as a command separator
// (nvi ex.c). Most commands end their argument at the first unescaped '|', with
// the remainder returned as the next command; '\|' is a literal '|'. Commands
// flagged wholeLine (and the '!' filter form of :read/:write) take the rest of
// the line verbatim. A substitute keeps its delimited RE intact so a '|' inside
// the pattern or replacement does not split -- only a '|' after the flags does.
func (p *exParser) parseArgPipe(c *exCmd) (arg, rest string) {
	s := p.s
	i := p.pos
	whole := func() (string, string) { return strings.TrimSpace(string(s[i:])), "" }
	// A substitute keeps trailing blanks: when the replacement has no closing
	// delimiter (":s/b/<tab>"), those blanks ARE the replacement text, and nvi
	// preserves them. The flag/count parser skips its own whitespace, so leaving
	// the blanks on does not disturb a normal ":s/b/x/ g" form.
	fin := func(v string) string {
		if c.def.substArg {
			return v
		}
		return strings.TrimSpace(v)
	}

	if c.def.wholeLine {
		return whole()
	}
	// :read !cmd and :write !cmd (a '!' as the first non-blank) are filters: the
	// rest of the line is the shell command and '|' is a shell pipe.
	if c.def.full == "read" || c.def.full == "write" {
		j := i
		for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
			j++
		}
		if j < len(s) && s[j] == '!' {
			return whole()
		}
	}

	var b strings.Builder
	// A substitute's RE (up to two more delimiters) is copied verbatim first, so
	// a '|' used as the RE delimiter or appearing inside the pattern is literal.
	if c.def.substArg {
		i = copySubstRE(s, i, &b)
	}
	for ; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == '|' {
			b.WriteRune('|')
			i++
			continue
		}
		if s[i] == '|' {
			return fin(b.String()), string(s[i+1:])
		}
		b.WriteRune(s[i])
	}
	return fin(b.String()), ""
}

// copySubstRE copies a substitute's leading delimited RE (the /pattern/replace
// portion) from s[i:] into b verbatim and returns the index just past it, so the
// caller can scan the trailing flags for a '|' separator. If the first non-blank
// is not a delimiter (a bare :s, or ":s g" flag form) nothing is copied.
func copySubstRE(s []rune, i int, b *strings.Builder) int {
	j := i
	for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
		j++
	}
	if j >= len(s) {
		return i
	}
	delim := s[j]
	// A bare repeat or a flags-only form (alnum or '|' next) has no RE to skip.
	if delim == '|' || delim == '\\' || (delim >= 'a' && delim <= 'z') ||
		(delim >= 'A' && delim <= 'Z') || (delim >= '0' && delim <= '9') {
		return i
	}
	// Copy the blanks and opening delimiter, then up to two more unescaped
	// delimiters (end of pattern, end of replacement).
	for ; i <= j; i++ {
		b.WriteRune(s[i])
	}
	for cnt := 2; i < len(s) && cnt > 0; i++ {
		if s[i] == '\\' && i+1 < len(s) {
			b.WriteRune(s[i])
			b.WriteRune(s[i+1])
			i++
			continue
		}
		if s[i] == delim {
			cnt--
		}
		b.WriteRune(s[i])
	}
	return i
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
		p.absAddr = true
		return n, true
	case r == '.':
		p.next()
		return cur, true
	case r == '$':
		p.next()
		p.absAddr = true
		return p.e.scr.lineCount(), true
	case r == '\'':
		p.next()
		if p.eof() {
			return 0, false
		}
		name := p.next()
		if mk, ok := p.e.scr.marks.Get(name); ok {
			p.absAddr = true
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
		p.absAddr = true
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
