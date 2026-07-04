package engine

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode"

	"govi/engine/regex"
)

// Regex-dependent ex commands: :s (substitute) and :g / :v (global).

// exSubstitute implements :[range]s/pattern/replacement/[flags][count].
func (e *Engine) exSubstitute(c *exCmd) error {
	// A bare :s (no pattern/delimiter) repeats the last substitution, like :&
	// (nvi). The current line is the default range.
	if strings.TrimSpace(c.arg) == "" {
		return e.exAmp(c)
	}
	l1, l2, err := e.rangeNoCount(c)
	if err != nil {
		return err
	}
	pattern, repl, rawFlags, err := splitSubst(c.arg)
	if err != nil {
		return err
	}
	flags, count, hasCount, err := splitFlagsCount(rawFlags)
	if err != nil {
		return err
	}
	// A trailing count applies the substitution to count lines starting at the
	// last addressed line (nvi/POSIX).
	if hasCount {
		l1 = l2
		l2 = l1 + count - 1
		if l2 > e.scr.lineCount() {
			return fmt.Errorf("Invalid address")
		}
	}
	re, err := e.compilePattern(pattern)
	if err != nil {
		return err
	}
	global := strings.ContainsRune(flags, 'g')

	s := e.scr
	// An unescaped ~ in the replacement stands for the previous replacement text
	// (historic vi; under nomagic the sense flips and \~ expands). Expand it
	// textually against the prior replacement before it becomes the new
	// "previous"; a literal tilde is left for the per-match stage.
	repl = expandReplTilde(repl, s.lastSubstRepl, s.opts.Bool("magic"))
	s.lastSubstRepl = repl
	s.lastSubstFlags = flags
	replRunes := []rune(repl) // decode once, not per line
	// The c flag asks about each replacement (nvi "Confirm change? [n]"). The
	// command returns to the event loop with the prompt up and the y/n/q
	// answers arrive as later keys (substConfirmKey). Inside a :g the body
	// runs synchronously and cannot pause for input, so the flag is ignored
	// there and the substitution is applied unconditionally.
	if strings.ContainsRune(flags, 'c') && s.gMarks == nil {
		return e.startSubstConfirm(re, replRunes, global, l1, l2)
	}
	e.beginChange()
	any := false
	var lastLine int64
	lno := l1
	end := l2
	for lno <= end {
		if e.Interrupted() {
			e.endChange() // balance beginChange; keep the subs already made
			return errInterrupted
		}
		in := s.lineRunes(lno)
		out, n, replaced := substituteLine(re, in, replRunes, global, s.opts.Bool("magic"))
		if replaced {
			any = true
			lastLine = lno
			// out may contain newlines (from a literal ^V-quoted CR in the
			// replacement): split it into one or more buffer lines.
			segs := splitRunes(out, '\n')
			s.setLineKnown(lno, in, segs[0])
			for i := 1; i < len(segs); i++ {
				s.appendLine(lno+int64(i-1), segs[i])
			}
			added := int64(len(segs) - 1)
			end += added
			lno += added
			_ = n
		}
		lno++
	}
	e.endChange()
	if !any {
		return fmt.Errorf("No match on lines %d,%d", l1, l2)
	}
	s.cursor = Pos{Line: clampLine(s, lastLine), Col: s.firstNonBlank(clampLine(s, lastLine))}
	return nil
}

// confirmPrompt is nvi's status-line question for each :s///c candidate.
const confirmPrompt = "Confirm change? [n]"

// substConfirm is the paused state of a :s///c substitution: the scan
// position between candidate matches while the engine waits for the y/n/q
// answer. The whole exchange sits inside one beginChange/endChange bracket so
// the accepted replacements undo as a unit, like the unconfirmed command.
type substConfirm struct {
	re      *regex.Regex
	repl    []rune
	global  bool
	lno     int64       // line being scanned
	end     int64       // last line of the range (grows when \n adds lines)
	pos     int         // scan offset within lno
	prevEnd int         // end of the previous match on lno (-1 none)
	m       regex.Match // candidate awaiting the answer
	matched bool        // some candidate was found (vs "No match")
}

// startSubstConfirm begins a confirmed substitution and shows the prompt for
// the first candidate. It returns an error if nothing in the range matches.
func (e *Engine) startSubstConfirm(re *regex.Regex, repl []rune, global bool, l1, l2 int64) error {
	s := e.scr
	s.subConfirm = &substConfirm{re: re, repl: repl, global: global, lno: l1, end: l2, prevEnd: -1}
	e.beginChange()
	if !e.substConfirmAdvance() {
		return fmt.Errorf("No match on lines %d,%d", l1, l2)
	}
	return nil
}

// substConfirmAdvance scans from the paused position to the next candidate,
// parks the cursor on the match (with the buffer text still unchanged, like
// nvi), and puts the confirm question on the status line. When the range is
// exhausted it completes the command and returns false.
func (e *Engine) substConfirmAdvance() bool {
	s := e.scr
	sc := s.subConfirm
	for sc.lno <= sc.end {
		in := s.lineRunes(sc.lno)
		for {
			m, ok := sc.re.MatchAt(in, sc.pos)
			if !ok {
				break
			}
			if m.End == m.Start && m.Start == sc.prevEnd {
				// An empty match immediately after the previous match is not
				// a candidate (see substituteLine): skip a character.
				if m.Start >= len(in) {
					break
				}
				sc.pos = m.Start + 1
				continue
			}
			sc.m = m
			sc.matched = true
			col := m.Start
			if col >= len(in) {
				col = len(in) - 1 // a $ match sits past EOL (nvi clamps too)
			}
			if col < 0 {
				col = 0
			}
			s.cursor = Pos{Line: sc.lno, Col: col}
			s.msg, s.msgKind = confirmPrompt, MsgInfo
			return true
		}
		sc.lno++
		sc.pos = 0
		sc.prevEnd = -1
	}
	e.finishSubstConfirm()
	return false
}

// substConfirmKey answers the pending prompt: y substitutes, q stops the
// command, anything else (n, Enter, Escape, ...) declines -- nvi's default.
func (e *Engine) substConfirmKey(ev KeyEvent) {
	sc := e.scr.subConfirm
	switch {
	case ev.Key == KeyNone && ev.Rune == 'y' && ev.Mods == 0:
		e.substConfirmApply()
	case ev.Key == KeyNone && ev.Rune == 'q' && ev.Mods == 0:
		e.finishSubstConfirm()
		return
	default:
		sc.prevEnd = sc.m.End
		sc.pos = sc.m.End
	}
	if !sc.global {
		// Without the g flag only the first match on each line is offered.
		sc.lno++
		sc.pos = 0
		sc.prevEnd = -1
	}
	e.substConfirmAdvance()
}

// substConfirmApply performs the accepted replacement in place, so the screen
// shows it before the next prompt (nvi stores the line and restarts), and
// moves the scan position past the inserted text.
func (e *Engine) substConfirmApply() {
	s := e.scr
	sc := s.subConfirm
	in := s.lineRunes(sc.lno)
	m := sc.m
	exp := buildReplacement(sc.repl, in, m, s.opts.Bool("magic"))
	full := make([]rune, 0, len(in)+len(exp)-(m.End-m.Start))
	full = append(full, in[:m.Start]...)
	full = append(full, exp...)
	if m.End <= len(in) {
		full = append(full, in[m.End:]...)
	}
	// A literal newline in the replacement splits the line, as in the
	// unconfirmed path.
	segs := splitRunes(full, '\n')
	s.setLineKnown(sc.lno, in, segs[0])
	for i := 1; i < len(segs); i++ {
		s.appendLine(sc.lno+int64(i-1), segs[i])
	}
	added := int64(len(segs) - 1)
	sc.end += added
	sc.lno += added
	// Resume scanning right after the expansion on its final line; prevEnd
	// there makes a following empty match adjacent, so it is skipped.
	expSegs := splitRunes(exp, '\n')
	if added == 0 {
		sc.pos = m.Start + len(exp)
	} else {
		sc.pos = len(expSegs[len(expSegs)-1])
	}
	sc.prevEnd = sc.pos
	col := m.Start
	if l := s.lineLen(sc.lno); col >= l {
		col = l - 1
	}
	if col < 0 {
		col = 0
	}
	s.cursor = Pos{Line: sc.lno, Col: col}
}

// finishSubstConfirm completes a confirmed substitution: closes the undo
// bracket and clears the prompt. nvi leaves the cursor at the last consulted
// position rather than moving to the last change, so the cursor stays put.
func (e *Engine) finishSubstConfirm() {
	s := e.scr
	if s.subConfirm == nil {
		return
	}
	s.subConfirm = nil
	e.endChange()
	if s.msg == confirmPrompt {
		s.msg, s.msgKind = "", MsgNone
	}
}

// repeatSubst implements & (and :&): repeat the last substitute on the current
// line.
func (e *Engine) repeatSubst() error {
	s := e.scr
	if s.lastPattern == "" {
		return fmt.Errorf("No previous substitution")
	}
	re, err := e.compilePattern("")
	if err != nil {
		return err
	}
	global := strings.ContainsRune(s.lastSubstFlags, 'g')
	lno := s.cursor.Line
	in := s.lineRunes(lno)
	out, _, replaced := substituteLine(re, in, []rune(s.lastSubstRepl), global, s.opts.Bool("magic"))
	if !replaced {
		return fmt.Errorf("No match")
	}
	e.beginChange()
	segs := splitRunes(out, '\n')
	s.setLineKnown(lno, in, segs[0])
	for i := 1; i < len(segs); i++ {
		s.appendLine(lno+int64(i-1), segs[i])
	}
	e.endChange()
	s.cursor = Pos{Line: lno, Col: s.firstNonBlank(lno)}
	return nil
}

// exAmp implements :[range]& -- repeat the last substitute over the range.
func (e *Engine) exAmp(c *exCmd) error {
	l1, l2, err := e.rangeNoCount(c)
	if err != nil {
		return err
	}
	s := e.scr
	if s.lastPattern == "" {
		return fmt.Errorf("No previous substitution")
	}
	re, err := e.compilePattern("")
	if err != nil {
		return err
	}
	global := strings.ContainsRune(s.lastSubstFlags, 'g')
	replRunes := []rune(s.lastSubstRepl) // decode once, not per line
	e.beginChange()
	any := false
	var last int64
	lno := l1
	end := l2
	for lno <= end {
		if e.Interrupted() {
			e.endChange() // balance beginChange; keep the subs already made
			return errInterrupted
		}
		in := s.lineRunes(lno)
		out, _, replaced := substituteLine(re, in, replRunes, global, s.opts.Bool("magic"))
		if replaced {
			any = true
			last = lno
			segs := splitRunes(out, '\n')
			s.setLineKnown(lno, in, segs[0])
			for i := 1; i < len(segs); i++ {
				s.appendLine(lno+int64(i-1), segs[i])
			}
			end += int64(len(segs) - 1)
			lno += int64(len(segs) - 1)
		}
		lno++
	}
	e.endChange()
	if !any {
		return fmt.Errorf("No match")
	}
	s.cursor = Pos{Line: clampLine(s, last), Col: s.firstNonBlank(clampLine(s, last))}
	return nil
}

// exTilde implements :[range]~ -- repeat the last substitution using the last
// regular expression used in any context. govi keeps a single lastPattern that
// both search and substitute update, so this resolves to the same repeat as :&.
func (e *Engine) exTilde(c *exCmd) error { return e.exAmp(c) }

// substituteLine applies re to a single line, replacing the first match or all
// matches (global). It returns the new line runes (which may contain '\n'), the
// number of replacements, and whether anything changed.
func substituteLine(re *regex.Regex, in, repl []rune, global, magic bool) ([]rune, int, bool) {
	var out []rune
	pos := 0
	count := 0
	prevEnd := -1 // end of the previous match, to reject adjacent empty matches
	for {
		m, ok := re.MatchAt(in, pos)
		if !ok {
			break
		}
		if m.End == m.Start && m.Start == prevEnd {
			// An empty match immediately following the previous match is not a
			// valid replacement (matches nvi / POSIX): skip a character and retry
			// so e.g. s/a*/X/g on "aaa" yields "X", not "XX".
			if m.Start >= len(in) {
				break
			}
			out = append(out, in[pos:m.Start+1]...)
			pos = m.Start + 1
			continue
		}
		out = append(out, in[pos:m.Start]...)
		out = append(out, buildReplacement(repl, in, m, magic)...)
		count++
		prevEnd = m.End
		if m.End == m.Start {
			// Empty match: emit one char and advance to avoid looping.
			if m.End < len(in) {
				out = append(out, in[m.End])
			}
			pos = m.End + 1
		} else {
			pos = m.End
		}
		if !global || pos > len(in) {
			break
		}
	}
	if count == 0 {
		return in, 0, false
	}
	if pos <= len(in) {
		out = append(out, in[pos:]...)
	}
	return out, count, true
}

// buildReplacement expands a substitution replacement, handling the whole
// match (& under magic, \& under nomagic -- the other spelling is a literal
// ampersand, per nvi regsub's O_MAGIC checks), \1-\9 (backreferences), and
// \u \l \U \L \E (case).  Any other escaped character is that literal
// character (nvi regsub) -- \n is the letter n, not a newline (that is
// sed/vim).  A literal (^V-quoted) CR or NL character in the replacement
// breaks the line, per nvi's OUTCH nltrans.
func buildReplacement(repl, in []rune, m regex.Match, magic bool) []rune {
	var out []rune
	// case mode: 0 none, 'U' upper-until-E, 'L' lower-until-E; oneShot 'u'/'l'.
	var caseMode rune
	var oneShot rune

	emit := func(r rune) {
		switch {
		case oneShot == 'u':
			r = unicode.ToUpper(r)
			oneShot = 0
		case oneShot == 'l':
			r = unicode.ToLower(r)
			oneShot = 0
		case caseMode == 'U':
			r = unicode.ToUpper(r)
		case caseMode == 'L':
			r = unicode.ToLower(r)
		}
		out = append(out, r)
	}
	emitGroup := func(g int) {
		if g < len(m.Groups) {
			s, en := m.Groups[g][0], m.Groups[g][1]
			if s >= 0 && en >= 0 {
				for _, r := range in[s:en] {
					emit(r)
				}
			}
		}
	}
	// emitRepl emits a character coming from the replacement string itself:
	// a literal CR or NL breaks the line (untouched by case conversion),
	// anything else goes through emit. Group text bypasses this (nvi OUTCH
	// nltrans=0), though a group can never hold a newline anyway.
	emitRepl := func(r rune) {
		if r == '\r' || r == '\n' {
			out = append(out, '\n')
			return
		}
		emit(r)
	}

	for i := 0; i < len(repl); i++ {
		r := repl[i]
		switch r {
		case '&':
			if magic {
				emitGroup(0)
			} else {
				emitRepl('&')
			}
		case '\\':
			if i+1 >= len(repl) {
				emit('\\')
				break
			}
			i++
			n := repl[i]
			switch {
			case n >= '0' && n <= '9':
				emitGroup(int(n - '0'))
			case n == '&' && !magic:
				emitGroup(0)
			case n == 'u', n == 'l':
				oneShot = n
			case n == 'U', n == 'L':
				caseMode = n
			case n == 'E', n == 'e':
				caseMode = 0
			default:
				emitRepl(n) // \& \\ \/ \n etc -> that literal character
			}
		default:
			emitRepl(r)
		}
	}
	return out
}

// expandReplTilde replaces each unescaped ~ in a substitute replacement with
// the previous replacement text prev (historic vi); under nomagic the sense
// flips: plain ~ is literal and \~ expands (nvi's O_MAGIC checks in the
// replacement parse). Other backslash escapes pass through untouched (so \&
// \1 etc. are not disturbed); the result becomes the new "previous".
func expandReplTilde(repl, prev string, magic bool) string {
	rs := []rune(repl)
	var b strings.Builder
	for i := 0; i < len(rs); i++ {
		if rs[i] == '\\' && i+1 < len(rs) {
			if rs[i+1] == '~' && !magic {
				b.WriteString(prev)
			} else {
				b.WriteRune(rs[i])
				b.WriteRune(rs[i+1])
			}
			i++
			continue
		}
		if rs[i] == '~' && magic {
			b.WriteString(prev)
			continue
		}
		b.WriteRune(rs[i])
	}
	return b.String()
}

// exGlobal implements :[range]g/pattern/cmd (default range whole file, default
// cmd print). :g! inverts to non-matching lines, like :v (exVglobal).
func (e *Engine) exGlobal(c *exCmd) error  { return e.global(c, c.force) }
func (e *Engine) exVglobal(c *exCmd) error { return e.global(c, true) }

func (e *Engine) global(c *exCmd, invert bool) error {
	s := e.scr
	l1, l2 := c.addr1, c.addr2
	if c.addrCount == 0 {
		l1, l2 = 1, s.lineCount()
	}
	pattern, cmd := splitGlobal(c.arg)
	if cmd == "" {
		cmd = "p"
	}
	re, err := e.compilePattern(pattern)
	if err != nil {
		return err
	}

	// Collect matching lines first (by current number), then run the command on
	// each. The matches are tracked in s.gMarks so the line-edit primitives keep
	// them in sync no matter where a body command (delete, copy, move) adds or
	// removes lines -- nvi flags each matched line and follows the flag, so a
	// command that inserts elsewhere (e.g. t$) does not mistrack later matches.
	var matches []int64
	for ln := l1; ln <= l2 && ln <= s.lineCount(); ln++ {
		if e.Interrupted() {
			return errInterrupted
		}
		_, ok := re.MatchAt(s.lineRunes(ln), 0)
		if ok != invert {
			matches = append(matches, ln)
		}
	}
	s.gMarks = matches
	s.gLastEdit = 0
	defer func() { s.gMarks = nil }()

	// Run the whole global as one undo group: nvi undoes an entire :g with a
	// single u. beginChange/endChange nest, so the per-line sub-commands collapse
	// into this one change set.
	e.beginChange()
	defer e.endChange()
	for i := range matches {
		if e.Interrupted() {
			return errInterrupted // keep the body commands already run (nvi)
		}
		target := s.gMarks[i]
		if target < 1 || target > s.lineCount() {
			continue // visited line was deleted by an earlier body command
		}
		s.gLastEdit = target
		if err := e.exExecute(fmt.Sprintf("%d%s", target, cmd)); err != nil {
			return err
		}
	}
	// nvi's final cursor is the line of the last insert/delete a body command
	// performed (or the last visited match if none), clamped to the last line
	// when that line is gone (ex.c range_lno fixup).
	if ln := s.gLastEdit; ln != 0 {
		if n := s.lineCount(); ln > n {
			ln = n
		}
		s.cursor.Line = ln
		s.clampCursor()
	}
	return nil
}

// splitFlagsCount splits a substitute's trailing flags into the flag letters and
// an optional repeat count (nvi: [cgr][count][#lp]). An unknown flag letter is a
// usage error -- nvi rejects it and makes NO change, rather than silently
// substituting (so :s/foo//n errors instead of deleting).
func splitFlagsCount(s string) (flags string, count int64, hasCount bool, err error) {
	usage := fmt.Errorf("Usage: [range]s[ubstitute] [/pattern/replace/] [cgr] [count] [#lp]")
	rs := []rune(s)
	i := 0
	var fb strings.Builder
	for i < len(rs) {
		r := rs[i]
		if r == ' ' || r == '\t' {
			i++
			continue
		}
		if r >= '0' && r <= '9' {
			break
		}
		switch r {
		case 'c', 'g', 'r', '#', 'l', 'p':
			fb.WriteRune(r)
			i++
		default:
			return "", 0, false, usage
		}
	}
	for i < len(rs) && (rs[i] == ' ' || rs[i] == '\t') {
		i++
	}
	start := i
	for i < len(rs) && rs[i] >= '0' && rs[i] <= '9' {
		count = count*10 + int64(rs[i]-'0')
		i++
	}
	hasCount = i > start
	// Trailing print flags (#lp) may follow the count.
	for i < len(rs) {
		switch rs[i] {
		case ' ', '\t', '#', 'l', 'p':
			i++
		default:
			return "", 0, false, usage
		}
	}
	return fb.String(), count, hasCount, nil
}

// splitSubst parses pattern/replacement/flags from a substitute argument whose
// first rune is the delimiter.
func splitSubst(arg string) (pattern, repl, flags string, err error) {
	r := []rune(arg)
	if len(r) == 0 {
		return "", "", "", fmt.Errorf("Missing pattern delimiter")
	}
	delim := r[0]
	parts := splitDelim(r[1:], delim, 3)
	pattern = parts[0]
	if len(parts) > 1 {
		repl = parts[1]
	}
	if len(parts) > 2 {
		flags = parts[2]
	}
	return pattern, repl, flags, nil
}

// splitGlobal parses pattern and command from a global argument.
func splitGlobal(arg string) (pattern, cmd string) {
	r := []rune(arg)
	if len(r) == 0 {
		return "", ""
	}
	delim := r[0]
	parts := splitDelim(r[1:], delim, 2)
	pattern = parts[0]
	if len(parts) > 1 {
		cmd = parts[1]
	}
	return pattern, cmd
}

// splitDelim splits runes on unescaped delim into at most max parts; the final
// part keeps any remaining text (including delimiters).
func splitDelim(r []rune, delim rune, max int) []string {
	var parts []string
	var cur []rune
	for i := 0; i < len(r); i++ {
		if r[i] == '\\' && i+1 < len(r) {
			// Keep the escape for later (regex/replacement) interpretation,
			// except an escaped delimiter becomes a literal delimiter.
			if r[i+1] == delim {
				cur = append(cur, delim)
				i++
				continue
			}
			cur = append(cur, r[i], r[i+1])
			i++
			continue
		}
		if r[i] == delim && len(parts) < max-1 {
			parts = append(parts, string(cur))
			cur = nil
			continue
		}
		cur = append(cur, r[i])
	}
	parts = append(parts, string(cur))
	return parts
}

func splitRunes(r []rune, sep rune) [][]rune {
	if !slices.Contains(r, sep) {
		// Common case (replacement has no embedded newline): one segment, and no
		// need to copy r -- the caller passes it straight to a line-edit
		// primitive, which makes its own copy.
		return [][]rune{r}
	}
	var out [][]rune
	var cur []rune
	for _, c := range r {
		if c == sep {
			out = append(out, cur)
			cur = nil
			continue
		}
		cur = append(cur, c)
	}
	out = append(out, cur)
	return out
}

// readFileLines reads path and splits it into lines, dropping a single trailing
// newline (matching how files load into the buffer).
func readFileLines(path string) ([][]rune, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	str := string(b)
	if str == "" {
		return nil, nil
	}
	parts := strings.Split(str, "\n")
	if strings.HasSuffix(str, "\n") {
		parts = parts[:len(parts)-1]
	}
	out := make([][]rune, len(parts))
	for i, p := range parts {
		out[i] = []rune(p)
	}
	return out, nil
}
