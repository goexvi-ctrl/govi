package engine

import (
	"govi/engine/buffer"
	"govi/engine/mark"
	"govi/engine/register"
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
	regs  *register.Set

	name     string // file path, or "" for an unnamed buffer
	modified bool

	cursor Pos   // 1-based line, 0-based rune column
	top    int64 // first buffer line shown (1-based)
	rows   int   // text rows available for the buffer
	cols   int   // columns available

	mode    Mode
	msg     string
	msgKind MessageKind

	// colon holds the in-progress command line while mode == ModeExColon;
	// cmdPrefix is the prompt character (':', '/', or '?') that determines how
	// the line is executed on Enter.
	colon     []rune
	cmdPrefix rune

	// exTranscript is the scrolling output shown while mode == ModeExText.
	exTranscript []string

	// pendingOutput is multi-line command output (e.g. :set all) shown over the
	// buffer in vi mode until the next keypress.
	pendingOutput []string

	// search and substitute state
	lastPattern    string
	lastSearchDir  searchDir
	lastSubstRepl  string
	lastSubstFlags string

	opts options
	maps mapTable

	// Cursor column maintenance for vertical motions (nvi's RCM). desiredCol is
	// the display column j/k/^F/... try to keep; desiredEOL makes them stick to
	// the end of each line (set by $).
	desiredCol int
	desiredEOL bool

	// showmatch: when a close bracket is typed, matchActive briefly highlights
	// the matching open bracket at matchPos.
	matchActive bool
	matchPos    Pos
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

// lineLen returns the rune length of buffer line lno.
func (s *screen) lineLen(lno int64) int { return len(s.lineRunes(lno)) }

// firstNonBlank returns the column of the first non-blank rune on line lno, or
// 0 if the line is empty or all blanks.
func (s *screen) firstNonBlank(lno int64) int {
	r := s.lineRunes(lno)
	for i, c := range r {
		if c != ' ' && c != '\t' {
			return i
		}
	}
	return 0
}

// Line-edit primitives. All buffer mutations go through these so they are
// recorded for undo and so marks are kept consistent. Callers must bracket a
// logical change with Engine.beginChange/endChange.

func (s *screen) setLine(lno int64, runes []rune) {
	if lno < 1 {
		return
	}
	if s.store.Lines() == 0 {
		s.log.Insert(1, runes)
		return
	}
	s.log.Set(lno, runes)
}

func (s *screen) insertLine(lno int64, runes []rune) {
	s.log.Insert(lno, runes)
	s.marks.LinesInserted(lno, 1)
}

func (s *screen) appendLine(lno int64, runes []rune) {
	s.log.Append(lno, runes)
	s.marks.LinesInserted(lno+1, 1)
}

func (s *screen) deleteLine(lno int64) {
	s.log.Delete(lno)
	s.marks.LinesDeleted(lno, 1)
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

// textCols returns the number of columns available for buffer text after the
// line-number gutter.
func (s *screen) textCols() int {
	w := s.cols - GutterWidth(s.lineCount(), s.opts.Bool("number"))
	if w < 1 {
		w = 1
	}
	return w
}

// displayWidth returns the total display width (columns) of line lno, with tabs
// and control characters expanded per the tabstop.
func (s *screen) displayWidth(lno int64) int {
	col := 0
	for _, r := range s.lineRunes(lno) {
		col += runeWidth(r, col, s.opts.Int("tabstop"))
	}
	return col
}

// displayColOf returns the display column at which rune index col begins on line
// lno.
func (s *screen) displayColOf(lno int64, col int) int {
	runes := s.lineRunes(lno)
	c := 0
	for i := 0; i < col && i < len(runes); i++ {
		c += runeWidth(runes[i], c, s.opts.Int("tabstop"))
	}
	return c
}

// colAtDisplay returns the rune index whose cell span contains display column
// dcol on line lno (clamped to the last rune when dcol is past the line end).
func (s *screen) colAtDisplay(lno int64, dcol int) int {
	runes := s.lineRunes(lno)
	c := 0
	for i, r := range runes {
		w := runeWidth(r, c, s.opts.Int("tabstop"))
		if c+w > dcol {
			return i
		}
		c += w
	}
	if len(runes) == 0 {
		return 0
	}
	return len(runes) - 1
}

// maintainedCol returns the rune column on line lno that vertical motions should
// land on, honoring the sticky-EOL flag.
func (s *screen) maintainedCol(lno int64) int {
	if s.desiredEOL {
		if n := s.lineLen(lno); n > 0 {
			return n - 1
		}
		return 0
	}
	return s.colAtDisplay(lno, s.desiredCol)
}

// screenLines returns the number of physical screen rows line lno occupies when
// wrapped to the text width (at least 1).
func (s *screen) screenLines(lno int64) int {
	return wrapRows(s.displayWidth(lno), s.textCols())
}

// wrapRows returns how many rows of the given width a span of dw display columns
// occupies (at least 1).
func wrapRows(dw, w int) int {
	if w < 1 {
		w = 1
	}
	if dw <= 0 {
		return 1
	}
	return (dw + w - 1) / w
}

// scrollToCursor adjusts top so the cursor's line is fully visible, accounting
// for line wrapping (a long line occupies several screen rows).
func (s *screen) scrollToCursor() {
	if s.rows <= 0 {
		return
	}
	if s.cursor.Line < s.top {
		s.top = s.cursor.Line
	}
	if s.top < 1 {
		s.top = 1
	}
	// Advance top one logical line at a time until the cursor line fits within
	// the visible screen rows.
	for s.top < s.cursor.Line {
		used := 0
		bottom := s.top
		for ln := s.top; ln <= s.lineCount(); ln++ {
			r := s.screenLines(ln)
			if used+r > s.rows {
				break
			}
			used += r
			bottom = ln
		}
		if s.cursor.Line <= bottom {
			break
		}
		s.top++
	}
}
