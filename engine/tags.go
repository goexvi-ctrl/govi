package engine

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Tag support: a ctags-format "tags" file maps identifiers to a file and an ex
// command (a line number or a /pattern/ search) locating the definition.
// :tag, ^] (tag the word under the cursor), and ^T (pop the tag stack) navigate
// it. This mirrors nvi's ex/ex_tag.c.

type tagLoc struct {
	file string
	line int64
	col  int
}

// tagMatch is one candidate location produced by :tag or :cscope find (nvi's
// TAG within the head TAGQ). A ctags match carries an ex address (a line number
// or /pattern/); a cscope match carries a source line number and the matching
// source-line text, used as a flexible-whitespace search. :tagnext/:tagprev
// step through a group of matches.
type tagMatch struct {
	file   string
	addr   string // ctags: ex address command
	line   int64  // cscope: source line number
	search string // cscope: source-line text ("" => jump by line number)
	cscope bool
}

// gotoTagMatch switches to m's file (when different) and positions the cursor:
// a cscope match searches for its source line, a ctags match applies its ex
// address. force allows leaving a modified buffer.
func (e *Engine) gotoTagMatch(m tagMatch, force bool) error {
	if m.file != "" && m.file != e.scr.name {
		if e.scr.modified && !force {
			return fmt.Errorf("No write since last change")
		}
		if err := e.Open(m.file); err != nil {
			return err
		}
	}
	if m.cscope {
		return e.cscopeSearch(m)
	}
	e.applyTagAddress(m.addr)
	return nil
}

// returnToLoc moves back to a saved tag location, switching files if needed.
func (e *Engine) returnToLoc(loc tagLoc) error {
	if loc.file != "" && loc.file != e.scr.name {
		if err := e.Open(loc.file); err != nil {
			return err
		}
	}
	e.scr.cursor = Pos{Line: clampLine(e.scr, loc.line), Col: loc.col}
	e.scr.clampCursor()
	return nil
}

// exTagNext implements :tagn[ext] -- move to the next match of the current tag
// group (nvi ex_tag_next).
func (e *Engine) exTagNext(c *exCmd) error {
	s := e.scr
	if len(s.tagMatches) == 0 {
		return fmt.Errorf("The tags stack is empty")
	}
	if s.tagMatchIdx+1 >= len(s.tagMatches) {
		return fmt.Errorf("Already at the last tag of this group")
	}
	s.tagMatchIdx++
	return e.gotoTagMatch(s.tagMatches[s.tagMatchIdx], c.force)
}

// exTagPrev implements :tagp[rev] -- move to the previous match of the current
// tag group (nvi ex_tag_prev).
func (e *Engine) exTagPrev(c *exCmd) error {
	s := e.scr
	if len(s.tagMatches) == 0 {
		return fmt.Errorf("The tags stack is empty")
	}
	if s.tagMatchIdx <= 0 {
		return fmt.Errorf("Already at the first tag of this group")
	}
	s.tagMatchIdx--
	return e.gotoTagMatch(s.tagMatches[s.tagMatchIdx], c.force)
}

// exTagTop implements :tagt[op] -- discard the whole tag stack and return to the
// oldest saved location (nvi ex_tag_top).
func (e *Engine) exTagTop(c *exCmd) error {
	s := e.scr
	if len(s.tagStack) == 0 {
		return fmt.Errorf("The tags stack is empty")
	}
	if s.modified && !c.force {
		return fmt.Errorf("No write since last change")
	}
	loc := s.tagStack[0]
	s.tagStack = nil
	s.tagMatches = nil
	s.tagMatchIdx = 0
	return e.returnToLoc(loc)
}

// exTagPop implements :tagp[op] -- pop one tag location off the stack (nvi
// ex_tag_pop), the ex-command form of ^T.
func (e *Engine) exTagPop(c *exCmd) error { return e.tagPop() }

// exTag implements :tag name.
func (e *Engine) exTag(c *exCmd) error {
	name := strings.TrimSpace(c.arg)
	if name == "" {
		return fmt.Errorf("tag: missing tag name")
	}
	if c.newScreen {
		return e.tagJumpNewScreen(name)
	}
	return e.tagJump(name)
}

// tagJumpNewScreen implements :Tag -- open the tag's file in a new split screen
// and position the cursor there (nvi ex_tag with E_NEWSCREEN).
func (e *Engine) tagJumpNewScreen(name string) error {
	file, excmd, err := e.lookupTag(name)
	if err != nil {
		return err
	}
	e.scr.tagStack = append(e.scr.tagStack, tagLoc{
		file: e.scr.name,
		line: e.scr.cursor.Line,
		col:  e.scr.cursor.Col,
	})
	if err := e.editNewScreen(file); err != nil {
		return err
	}
	e.applyTagAddress(excmd)
	return nil
}

// tagJumpWord implements ^]: jump to the tag named by the word under the cursor.
func (e *Engine) tagJumpWord() error {
	word := e.scr.wordAt(e.scr.cursor.Line, e.scr.cursor.Col)
	if word == "" {
		return fmt.Errorf("Cursor not in a word")
	}
	return e.tagJump(word)
}

func (e *Engine) tagJump(name string) error {
	matches, err := e.lookupTagAll(name)
	if err != nil {
		return err
	}
	if e.scr.modified {
		return fmt.Errorf("No write since last change")
	}
	// Push the current location for ^T, then make the matches the active group so
	// :tagnext/:tagprev can step through them.
	e.scr.tagStack = append(e.scr.tagStack, tagLoc{
		file: e.scr.name,
		line: e.scr.cursor.Line,
		col:  e.scr.cursor.Col,
	})
	e.scr.tagMatches = matches
	e.scr.tagMatchIdx = 0
	return e.gotoTagMatch(matches[0], true) // modified already checked above
}

// applyTagAddress positions the cursor per a tag's ex command: a line number or
// a /pattern/ (or ?pattern?) search.
func (e *Engine) applyTagAddress(excmd string) {
	excmd = strings.TrimSpace(excmd)
	if excmd == "" {
		return
	}
	if n, err := strconv.ParseInt(excmd, 10, 64); err == nil {
		e.gotoLine(n)
		return
	}
	if len(excmd) >= 2 && (excmd[0] == '/' || excmd[0] == '?') {
		pat := excmd[1 : len(excmd)-1]
		// ctags wraps the line in ^...$ and escapes nothing else; search for it.
		e.scr.cursor = Pos{Line: 1, Col: 0}
		if err := e.startSearch(pat, searchFwd); err != nil {
			e.scr.msg, e.scr.msgKind = err.Error(), MsgError
		}
	}
}

// tagPop implements ^T: return to the location saved by the most recent tag
// jump.
func (e *Engine) tagPop() error {
	if len(e.scr.tagStack) == 0 {
		return fmt.Errorf("The tags stack is empty")
	}
	if e.scr.modified {
		return fmt.Errorf("No write since last change")
	}
	loc := e.scr.tagStack[len(e.scr.tagStack)-1]
	e.scr.tagStack = e.scr.tagStack[:len(e.scr.tagStack)-1]
	// Leaving the current tag group; :tagnext/:tagprev no longer apply.
	e.scr.tagMatches = nil
	e.scr.tagMatchIdx = 0
	return e.returnToLoc(loc)
}

// lookupTag scans the tags file(s) for name, returning the first match's target
// file and ex command.
func (e *Engine) lookupTag(name string) (file, excmd string, err error) {
	matches, err := e.lookupTagAll(name)
	if err != nil {
		return "", "", err
	}
	return matches[0].file, matches[0].addr, nil
}

// lookupTagAll scans the tags file(s) for name, returning every matching entry
// (across all files), so :tagnext/:tagprev can step through tags with more than
// one definition (nvi ctag_slist).
func (e *Engine) lookupTagAll(name string) ([]tagMatch, error) {
	tl := e.scr.opts.Int("taglength")
	var matches []tagMatch
	for _, tagsPath := range strings.Fields(e.scr.opts.Str("tags")) {
		f, ferr := os.Open(e.resolvePath(tagsPath))
		if ferr != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if line == "" || line[0] == '!' {
				continue
			}
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) < 3 {
				continue
			}
			if sigTag(parts[0], tl) == sigTag(name, tl) {
				matches = append(matches, tagMatch{file: parts[1], addr: stripTagComment(parts[2])})
			}
		}
		f.Close()
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("%s: tag not found", name)
	}
	return matches, nil
}

// sigTag returns the significant prefix of a tag name under taglength: when
// tl > 0 only the first tl runes are significant; tl == 0 means all are.
func sigTag(s string, tl int) string {
	if tl > 0 {
		r := []rune(s)
		if len(r) > tl {
			return string(r[:tl])
		}
	}
	return s
}

// stripTagComment removes a trailing ";\"" extension-field comment that modern
// ctags append to the ex command.
func stripTagComment(s string) string {
	if i := strings.Index(s, ";\""); i >= 0 {
		return s[:i]
	}
	return s
}
