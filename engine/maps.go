package engine

import (
	"fmt"
	"strings"
)

// Maps and abbreviations (nvi's seq machinery, common/seq.c, ex/ex_map.c).
//
// A map replaces a typed key sequence with another in command mode (:map) or
// insert mode (:map!). An abbreviation replaces a just-typed word in insert
// mode (:abbreviate). Map expansion happens in the engine's input path, ahead
// of the vi state machine. Maps are non-remapping (the right-hand side is sent
// literally), which covers ordinary use; full recursive remapping is a later
// refinement.

type mapTable struct {
	command map[string]string // :map  (command mode)
	insert  map[string]string // :map! (insert mode)
	abbrev  map[string]string // :abbreviate (insert mode, on word break)
}

func newMapTable() mapTable {
	return mapTable{
		command: map[string]string{},
		insert:  map[string]string{},
		abbrev:  map[string]string{},
	}
}

// --- ex commands ---

func (e *Engine) exMap(c *exCmd) error {
	lhs, rhs, ok := splitMapArg(c.arg)
	if !ok {
		return nil // listing maps is not yet supported; treat as a no-op query
	}
	table := e.scr.maps.command
	if c.force {
		table = e.scr.maps.insert
	}
	table[string(decodeKeys(lhs))] = rhs
	return nil
}

func (e *Engine) exUnmap(c *exCmd) error {
	lhs := strings.TrimSpace(c.arg)
	if lhs == "" {
		return fmt.Errorf("unmap: missing argument")
	}
	key := string(decodeKeys(lhs))
	if c.force {
		delete(e.scr.maps.insert, key)
	} else {
		delete(e.scr.maps.command, key)
	}
	return nil
}

func (e *Engine) exAbbreviate(c *exCmd) error {
	lhs, rhs, ok := splitMapArg(c.arg)
	if !ok {
		return nil
	}
	e.scr.maps.abbrev[lhs] = rhs
	return nil
}

func (e *Engine) exUnabbreviate(c *exCmd) error {
	lhs := strings.TrimSpace(c.arg)
	if lhs == "" {
		return fmt.Errorf("unabbreviate: missing argument")
	}
	delete(e.scr.maps.abbrev, lhs)
	return nil
}

// splitMapArg splits "lhs rhs" on the first run of whitespace. ok is false when
// there is no rhs (a query/list form).
func splitMapArg(arg string) (lhs, rhs string, ok bool) {
	arg = strings.TrimLeft(arg, " \t")
	i := strings.IndexAny(arg, " \t")
	if i < 0 {
		return arg, "", false
	}
	lhs = arg[:i]
	rhs = strings.TrimLeft(arg[i:], " \t")
	return lhs, rhs, rhs != ""
}

// decodeKeys converts caret notation (^X, ^[, ^?) into the control runes it
// denotes, leaving other characters literal.
func decodeKeys(s string) []rune {
	r := []rune(s)
	var out []rune
	for i := 0; i < len(r); i++ {
		if r[i] == '^' && i+1 < len(r) {
			c := r[i+1]
			switch {
			case c >= '@' && c <= '_':
				out = append(out, c&0x1f)
				i++
				continue
			case c == '?':
				out = append(out, 0x7f)
				i++
				continue
			}
		}
		out = append(out, r[i])
	}
	return out
}

// --- input-path map expansion ---

func (e *Engine) activeMapTable() map[string]string {
	switch e.scr.mode {
	case ModeCommand:
		return e.scr.maps.command
	case ModeInsert, ModeReplace:
		return e.scr.maps.insert
	}
	return nil
}

// handleKeyEvent is the entry point for a key from Input. Plain runes flow
// through map expansion; special and modified keys flush any partial map and
// dispatch directly.
func (e *Engine) handleKeyEvent(ev KeyEvent) {
	if ev.Key != KeyNone || ev.Mods&(ModCtrl|ModAlt) != 0 {
		e.flushMapPending()
		e.dispatchKey(ev)
		return
	}
	e.feedMapRune(ev.Rune)
}

func (e *Engine) feedMapRune(r rune) {
	if len(e.activeMapTable()) == 0 {
		e.dispatchRune(r)
		return
	}
	e.mapPending = append(e.mapPending, r)
	e.resolveMap(false)
}

// resolveMap consumes mapPending: while the buffer is a strict prefix of some
// map it waits (unless forced by a timeout); otherwise it expands the longest
// map that is a prefix of the buffer, or, if none, flushes the leading rune to
// the editor and retries.
func (e *Engine) resolveMap(forced bool) {
	for len(e.mapPending) > 0 {
		table := e.activeMapTable()
		if len(table) == 0 {
			e.flushMapPending()
			return
		}
		s := string(e.mapPending)
		if isStrictPrefixOfSome(table, s) && !forced {
			return // ambiguous: wait for more input or a timeout
		}
		if lhs := longestPrefixMap(table, s); lhs != "" {
			rhs := table[lhs]
			e.mapPending = []rune(s[len(lhs):])
			for _, c := range decodeKeys(rhs) {
				e.dispatchRune(c)
			}
			forced = false
			continue
		}
		first := e.mapPending[0]
		e.mapPending = e.mapPending[1:]
		e.dispatchRune(first)
		forced = false
	}
}

// longestPrefixMap returns the longest map LHS that is a prefix of s, or "".
func longestPrefixMap(table map[string]string, s string) string {
	best := ""
	for lhs := range table {
		if len(lhs) <= len(s) && strings.HasPrefix(s, lhs) && len(lhs) > len(best) {
			best = lhs
		}
	}
	return best
}

func (e *Engine) flushMapPending() {
	pend := e.mapPending
	e.mapPending = nil
	for _, c := range pend {
		e.dispatchRune(c)
	}
}

// mapTimeout resolves a pending ambiguous map when the input wait elapses.
func (e *Engine) mapTimeout() { e.resolveMap(true) }

func isStrictPrefixOfSome(table map[string]string, s string) bool {
	for lhs := range table {
		if len(lhs) > len(s) && strings.HasPrefix(lhs, s) {
			return true
		}
	}
	return false
}

// dispatchRune converts a rune (possibly a control code from a map RHS) into a
// key event and dispatches it to the mode handler.
func (e *Engine) dispatchRune(r rune) {
	switch {
	case r == 0x1b:
		e.dispatchKey(KeyEvent{Key: KeyEscape})
	case r == '\r' || r == '\n':
		e.dispatchKey(KeyEvent{Key: KeyEnter})
	case r == 0x08:
		e.dispatchKey(KeyEvent{Key: KeyBackspace}) // ^H
	case r == 0x7f:
		e.dispatchKey(KeyEvent{Rune: 0x7f}) // ^? (DEL): rejected in command mode
	case r == '\t':
		e.dispatchKey(KeyEvent{Rune: '\t'})
	case r < 0x20:
		e.dispatchKey(KeyEvent{Rune: r + 0x60, Mods: ModCtrl}) // ^A (1) -> 'a'+Ctrl
	default:
		e.dispatchKey(KeyEvent{Rune: r})
	}
}

// --- abbreviations ---

// maybeExpandAbbrev is called in insert mode when a word-breaking key is typed.
// If the word immediately before the cursor is an abbreviation, it is replaced.
func (e *Engine) maybeExpandAbbrev() {
	s := e.scr
	if len(s.maps.abbrev) == 0 {
		return
	}
	line := s.lineRunes(s.cursor.Line)
	col := clampIdx(s.cursor.Col, len(line))
	start := col
	for start > 0 && isWordRune(line[start-1]) {
		start--
	}
	if start == col {
		return
	}
	word := string(line[start:col])
	rhs, ok := s.maps.abbrev[word]
	if !ok {
		return
	}
	repl := decodeKeys(rhs)
	nl := append(append(cloneR(line[:start]), repl...), line[col:]...)
	s.setLine(s.cursor.Line, nl)
	s.cursor.Col = start + len(repl)
}
