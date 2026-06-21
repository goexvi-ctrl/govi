package engine

import (
	"govi/engine/buffer"
	"govi/engine/mark"
	"govi/engine/undo"
)

// screen is the per-view editor state: the buffer being edited plus everything
// about how this view onto it is positioned and presented. It corresponds to
// nvi's SCR (common/screen.h). One Engine drives one screen in Phase 2; split
// screens and multiple buffers arrive in later phases.
type screen struct {
	store buffer.LineStore
	log   *undo.Log
	marks *mark.Set

	name     string // file path, or "" for an unnamed buffer
	modified bool

	cursor Pos   // 1-based line, 0-based rune column
	top    int64 // first buffer line shown (1-based)
	rows   int   // text rows available for the buffer
	cols   int   // columns available

	mode    Mode
	msg     string
	msgKind MessageKind

	// colon holds the in-progress ex command line while mode == ModeExColon.
	colon []rune
}

// lineCount returns the number of lines in the buffer, treating an empty buffer
// as a single empty line so the cursor always has somewhere to be (vi shows one
// blank line for an empty file).
func (s *screen) lineCount() int64 {
	if n := s.store.Lines(); n > 0 {
		return n
	}
	return 1
}

// lineRunes returns the runes of buffer line lno, or an empty slice for the
// phantom line of an empty buffer / out-of-range request.
func (s *screen) lineRunes(lno int64) []rune {
	if lno < 1 || lno > s.store.Lines() {
		return nil
	}
	r, err := s.store.Get(lno)
	if err != nil {
		return nil
	}
	return r
}

// clampCursor keeps the cursor within the buffer and within its line. maxCol is
// the largest legal column: in command mode the cursor rests on the last rune
// (len-1), not past it.
func (s *screen) clampCursor() {
	n := s.lineCount()
	if s.cursor.Line < 1 {
		s.cursor.Line = 1
	}
	if s.cursor.Line > n {
		s.cursor.Line = n
	}
	llen := len(s.lineRunes(s.cursor.Line))
	max := llen - 1
	if s.mode == ModeInsert || s.mode == ModeReplace {
		max = llen // insert mode may sit just past the end
	}
	if max < 0 {
		max = 0
	}
	if s.cursor.Col < 0 {
		s.cursor.Col = 0
	}
	if s.cursor.Col > max {
		s.cursor.Col = max
	}
}

// scrollToCursor adjusts top so the cursor line is visible.
func (s *screen) scrollToCursor() {
	if s.rows <= 0 {
		return
	}
	if s.cursor.Line < s.top {
		s.top = s.cursor.Line
	}
	if s.cursor.Line > s.top+int64(s.rows)-1 {
		s.top = s.cursor.Line - int64(s.rows) + 1
	}
	if s.top < 1 {
		s.top = 1
	}
}
