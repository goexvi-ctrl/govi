package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"govi/engine/regex"
)

// compileCscopePattern compiles a cscope-converted pattern. cscope searches are
// always magic and case-sensitive, independent of the editor's options, and
// need \| alternation for the blank-run expression (nvi passes SEARCH_CSCOPE,
// which forces extended REs and ignores ignorecase/magic).
func compileCscopePattern(p string) (*regex.Regex, error) {
	return regex.Compile(p, regex.Options{Magic: true, Alt: true})
}

// cscopeFind implements "cscope find <type> <pattern>": query every connection,
// collect the matches into the tag-match queue, and jump to the first one (nvi
// cscope_find). force (a trailing '!') lets the file switch happen even with
// unsaved changes.
func (e *Engine) cscopeFind(arg string, force bool) error {
	if len(e.cscopes) == 0 {
		return fmt.Errorf("No cscope connections running")
	}

	num, pattern, err := parseCscopeQuery(arg)
	if err == errCscopeFindUsage {
		// nvi's create_cs_cmd shows the "find" help (csc_help) on a usage error,
		// not an error message.
		return e.cscopeHelp("find")
	}
	if err != nil {
		return err
	}

	// Save the current location so ^T can return here once we jump.
	ret := tagLoc{file: e.scr.name, line: e.scr.cursor.Line, col: e.scr.cursor.Col, tag: pattern}

	var matches []tagMatch
	for _, c := range e.cscopes {
		ms, err := c.find(num, pattern)
		if err != nil {
			return err
		}
		matches = append(matches, ms...)
	}

	if len(matches) == 0 {
		e.scr.msg, e.scr.msgKind = "No matches for query", MsgInfo
		return nil
	}

	e.scr.tagStack = append(e.scr.tagStack, ret)
	e.scr.tagMatches = matches
	e.scr.tagMatchIdx = 0
	return e.gotoTagMatch(matches[0], force)
}

// errCscopeFindUsage signals that "cscope find" was given no/!malformed type or
// no pattern; nvi responds by printing the find help (csc_help), not an error.
var errCscopeFindUsage = errors.New("cscope find usage")

// parseCscopeQuery parses "<type> <pattern>" into the cscope query number and
// the search pattern (nvi create_cs_cmd). The type letter must be one of
// cscopeQueries and be followed by a blank.
func parseCscopeQuery(arg string) (num int, pattern string, err error) {
	r := []rune(arg)
	// Skip leading blanks; need a type char followed by a blank.
	i := 0
	for i < len(r) && (r[i] == ' ' || r[i] == '\t') {
		i++
	}
	if i >= len(r) || i+1 >= len(r) || !(r[i+1] == ' ' || r[i+1] == '\t') {
		return 0, "", errCscopeFindUsage
	}
	typ := r[i]
	idx := strings.IndexRune(cscopeQueries, typ)
	if idx < 0 || typ == ' ' {
		return 0, "", fmt.Errorf("%c: unknown search type: use one of %s", typ, cscopeQueries)
	}
	// Skip blanks to the pattern.
	j := i + 1
	for j < len(r) && (r[j] == ' ' || r[j] == '\t') {
		j++
	}
	if j >= len(r) {
		return 0, "", errCscopeFindUsage
	}
	return idx, string(r[j:]), nil
}

// find sends one query to the connection and returns its matches, resolving each
// result's file name against the connection's search paths (nvi parse/csc_file).
func (c *cscopeConn) find(num int, pattern string) ([]tagMatch, error) {
	if _, err := fmt.Fprintf(c.in, "%d%s\n", num, pattern); err != nil {
		return nil, err
	}

	// Read lines until the "cscope: N lines" count line. Anything before it is a
	// warning/error (e.g. an out-of-date database) and is discarded, like nvi.
	// Some cscope builds report a failed search as "Unable to search database"
	// with no count line, going straight to the ">> " prompt; detect that prompt
	// (which has no trailing newline, so a line read would block) as zero matches.
	var nlines int
	for {
		if c.atPrompt() {
			if err := c.consumePrompt(); err != nil {
				return nil, fmt.Errorf("%s: %v", c.dir, err)
			}
			return nil, nil
		}
		line, err := c.out.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("%s: %v", c.dir, err)
		}
		if n, ok := parseCscopeCount(line); ok {
			nlines = n
			break
		}
	}

	var matches []tagMatch
	for ; nlines > 0; nlines-- {
		line, err := c.out.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("%s: %v", c.dir, err)
		}
		name, lno, search, ok := parseCscopeResult(line)
		if !ok {
			continue
		}
		fname, isOlder := c.resolveFile(name)
		// If the file is newer than the database, or there's no usable search
		// string, jump by line number instead of searching (nvi parse).
		if isOlder || search == "<unknown>" {
			search = ""
		}
		matches = append(matches, tagMatch{
			file:   fname,
			line:   lno,
			search: search,
			cscope: true,
		})
	}

	if err := c.readPrompt(); err != nil {
		return nil, fmt.Errorf("%s: %v", c.dir, err)
	}
	return matches, nil
}

// parseCscopeCount parses a "cscope: N lines" status line.
func parseCscopeCount(line string) (int, bool) {
	line = strings.TrimRight(line, "\r\n")
	i := strings.Index(line, "cscope: ")
	if i < 0 {
		return 0, false
	}
	rest := strings.TrimSuffix(line[i+len("cscope: "):], " lines")
	if rest == line[i+len("cscope: "):] { // suffix " lines" absent
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(rest))
	if err != nil {
		return 0, false
	}
	return n, true
}

// parseCscopeResult parses one result line "<file> <context> <lineno> <pattern>"
// (nvi parse). The pattern is the remainder after the line number.
func parseCscopeResult(line string) (file string, lno int64, search string, ok bool) {
	line = strings.TrimRight(line, "\r\n")
	file, rest, ok := nextField(line)
	if !ok {
		return "", 0, "", false
	}
	_, rest, ok = nextField(rest) // context (unused)
	if !ok {
		return "", 0, "", false
	}
	lnoStr, rest, ok := nextField(rest)
	if !ok {
		return "", 0, "", false
	}
	n, err := strconv.ParseInt(lnoStr, 10, 64)
	if err != nil {
		return "", 0, "", false
	}
	// The rest of the line, blanks trimmed from the front, is the search pattern.
	return file, n, strings.TrimLeft(rest, " \t"), true
}

// nextField splits off the first whitespace-delimited field of s and the
// remainder (after the single separating run of blanks).
func nextField(s string) (field, rest string, ok bool) {
	s = strings.TrimLeft(s, " \t")
	if s == "" {
		return "", "", false
	}
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, "", true
	}
	return s[:i], s[i:], true
}

// resolveFile finds name under the connection's search paths and reports whether
// the file is older than the database (nvi csc_file). If not found, the name is
// returned unchanged with isOlder false.
func (c *cscopeConn) resolveFile(name string) (path string, isOlder bool) {
	for _, p := range c.paths {
		cand := filepath.Join(p, name)
		if info, err := os.Stat(cand); err == nil {
			return cand, info.ModTime().Before(c.mtime)
		}
	}
	return name, false
}

// cscopeSearch positions the cursor at a cscope match in the already-loaded file
// (nvi cscope_search): search for the converted source-line pattern, or, when
// there is no pattern, go to the recorded line number. The cursor lands on the
// first non-blank of the line.
func (e *Engine) cscopeSearch(m tagMatch) error {
	s := e.scr
	var lno int64
	if m.search == "" {
		if m.line < 1 || m.line > s.lineCount() {
			return fmt.Errorf("%s: the tag's line number is past the end of the file", m.file)
		}
		lno = m.line
	} else {
		re, err := compileCscopePattern(cscopeConv(m.search))
		if err != nil {
			return fmt.Errorf("%s: search pattern not found", m.search)
		}
		pos, ok := e.searchFrom(re, Pos{Line: 1, Col: -1}, searchFwd)
		if !ok {
			return fmt.Errorf("%s: search pattern not found", m.search)
		}
		lno = pos.Line
	}
	s.cursor = Pos{Line: lno, Col: s.firstNonBlank(lno)}
	s.clampCursor()
	return nil
}

// cscopeReSpace is the regex (in vi "magic" BRE syntax) that each blank in a
// cscope source line stands for: any run of blanks and C comments (nvi
// re_cscope_conv's CSCOPE_RE_SPACE). [[:blank:]] is space or tab.
const cscopeReSpace = `\([[:blank:]]\|/\*\([^*]\|\*/\)*\*/\)*`

// cscopeConv converts a cscope source-line pattern into a vi-magic regular
// expression anchored to the whole line, with each blank matching an arbitrary
// run of whitespace/comments (nvi re_cscope_conv). Regex metacharacters in the
// literal text are escaped.
func cscopeConv(pattern string) string {
	var b strings.Builder
	b.WriteByte('^')
	b.WriteString(cscopeReSpace)
	for _, r := range pattern {
		if r == ' ' || r == '\t' {
			b.WriteString(cscopeReSpace)
			continue
		}
		// Escape the characters that are special in vi-magic BRE so the source
		// text matches literally. The remaining ERE metacharacters ( ) | + ? { }
		// are literal when unescaped in BRE, so they are left as-is.
		if strings.ContainsRune(`\^.[]$*`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteString(cscopeReSpace)
	b.WriteByte('$')
	return b.String()
}
