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
	s.lastSubstRepl = repl
	s.lastSubstFlags = flags
	replRunes := []rune(repl) // decode once, not per line
	e.beginChange()
	any := false
	var lastLine int64
	lno := l1
	end := l2
	for lno <= end {
		in := s.lineRunes(lno)
		out, n, replaced := substituteLine(re, in, replRunes, global)
		if replaced {
			any = true
			lastLine = lno
			// out may contain newlines (from \n in the replacement): split it
			// into one or more buffer lines.
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
	out, _, replaced := substituteLine(re, in, []rune(s.lastSubstRepl), global)
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
		in := s.lineRunes(lno)
		out, _, replaced := substituteLine(re, in, replRunes, global)
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
func substituteLine(re *regex.Regex, in, repl []rune, global bool) ([]rune, int, bool) {
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
		out = append(out, buildReplacement(repl, in, m)...)
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

// buildReplacement expands a substitution replacement, handling & (whole match),
// \1-\9 (backreferences), \u \l \U \L \E (case), \n (newline), and \\ / \&
// escapes.
func buildReplacement(repl, in []rune, m regex.Match) []rune {
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

	for i := 0; i < len(repl); i++ {
		r := repl[i]
		switch r {
		case '&':
			emitGroup(0)
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
			case n == 'n':
				out = append(out, '\n')
			case n == 't':
				emit('\t')
			case n == 'u', n == 'l':
				oneShot = n
			case n == 'U', n == 'L':
				caseMode = n
			case n == 'E', n == 'e':
				caseMode = 0
			default:
				emit(n) // \& \\ \/ etc -> literal
			}
		default:
			emit(r)
		}
	}
	return out
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
		_, ok := re.MatchAt(s.lineRunes(ln), 0)
		if ok != invert {
			matches = append(matches, ln)
		}
	}
	s.gMarks = matches
	defer func() { s.gMarks = nil }()

	// Run the whole global as one undo group: nvi undoes an entire :g with a
	// single u. beginChange/endChange nest, so the per-line sub-commands collapse
	// into this one change set.
	e.beginChange()
	defer e.endChange()
	for i := range matches {
		target := s.gMarks[i]
		if target < 1 || target > s.lineCount() {
			continue // visited line was deleted by an earlier body command
		}
		if err := e.exExecute(fmt.Sprintf("%d%s", target, cmd)); err != nil {
			return err
		}
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
