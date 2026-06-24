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

// exTag implements :tag name.
func (e *Engine) exTag(c *exCmd) error {
	name := strings.TrimSpace(c.arg)
	if name == "" {
		return fmt.Errorf("tag: missing tag name")
	}
	return e.tagJump(name)
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
	file, excmd, err := e.lookupTag(name)
	if err != nil {
		return err
	}
	if e.scr.modified {
		return fmt.Errorf("No write since last change")
	}
	// Push the current location for ^T.
	e.tagStack = append(e.tagStack, tagLoc{
		file: e.scr.name,
		line: e.scr.cursor.Line,
		col:  e.scr.cursor.Col,
	})
	if file != e.scr.name {
		if err := e.Open(file); err != nil {
			return err
		}
	}
	e.applyTagAddress(excmd)
	return nil
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
	if len(e.tagStack) == 0 {
		return fmt.Errorf("Tags stack empty")
	}
	loc := e.tagStack[len(e.tagStack)-1]
	e.tagStack = e.tagStack[:len(e.tagStack)-1]
	if e.scr.modified {
		return fmt.Errorf("No write since last change")
	}
	if loc.file != "" && loc.file != e.scr.name {
		if err := e.Open(loc.file); err != nil {
			return err
		}
	}
	e.scr.cursor = Pos{Line: clampLine(e.scr, loc.line), Col: loc.col}
	e.scr.clampCursor()
	return nil
}

// lookupTag scans the tags file(s) for name, returning the target file and ex
// command.
func (e *Engine) lookupTag(name string) (file, excmd string, err error) {
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
			if parts[0] == name {
				f.Close()
				return parts[1], stripTagComment(parts[2]), nil
			}
		}
		f.Close()
	}
	return "", "", fmt.Errorf("%s: tag not found", name)
}

// stripTagComment removes a trailing ";\"" extension-field comment that modern
// ctags append to the ex command.
func stripTagComment(s string) string {
	if i := strings.Index(s, ";\""); i >= 0 {
		return s[:i]
	}
	return s
}
