package engine

import (
	"os"

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

	// dlCache memoizes DisplayLines so a redraw need not recompute every
	// visible row on each input event (a cursor-only move changes no content).
	// It is keyed by 1-based line number and valid only while dlGen matches the
	// undo log's edit generation and the tabstop/list options are unchanged --
	// the three inputs makeDisplayLine depends on. Any edit (including undo/redo)
	// bumps the generation; an option change is caught by dlTab/dlList; both
	// force a rebuild. See displayLine.
	dlCache map[int64]DisplayLine
	dlGen   uint64
	dlTab   int
	dlList  bool
	dlReady bool

	name        string // file path, or "" for an unnamed buffer
	modified    bool
	nameChanged bool // :f renamed the buffer; cleared on write (nvi FR_NAMECHANGE)
	tempFile    bool // backed by a throwaway temp file (govi -g, no file); discarded on exit

	cursor     Pos   // 1-based line, 0-based rune column
	top        int64 // first buffer line shown (1-based)
	rows       int   // full text rows (nvi t_maxrows); the status row sits just below
	mapRows    int   // active scroll-map height (nvi t_rows)
	minMapRows int   // minimum map height after z[count] (nvi t_minrows)
	cols       int   // columns available
	defScroll  int   // ^D/^U half-page size (nvi defscroll); 0 = derive from rows
	winUserSet bool  // window option was set explicitly (survives resizes)

	// Split-screen placement in the display (nvi SCR roff/coff). A screen
	// occupies display rows [roff, roff+rows) for text and row roff+rows for its
	// own status/colon/message line; columns [coff, coff+cols). For a single
	// (unsplit) screen roff and coff are 0.
	roff int
	coff int

	// file is the paged-file handle backing this screen's buffer, if any (nvi's
	// per-screen EXF). nil for an in-memory buffer.
	file *os.File

	// Per-screen file-list and navigation state (nvi keeps argv/cargv, the
	// alternate name, and the tag stack in each SCR). A new split screen starts
	// with an empty argument list and no alternate/tag history.
	argv          []string // file argument list
	argIdx        int      // index of the current file in argv
	showFileCount bool     // next :f/^G shows "N files to edit" (nvi SC_STATUS_CNT)
	altFile       string   // alternate file (^^ / #), the previously edited file
	tagStack      []tagLoc // tag jump stack for ^T

	// tagMatches holds the candidate locations of the most recent :tag or
	// :cscope find that produced one or more results (nvi's head TAGQ);
	// :tagnext/:tagprev step through them and tagMatchIdx is the current one.
	tagMatches  []tagMatch
	tagMatchIdx int

	mode          Mode
	showModeLabel string // showmode text: Command, Insert, Append, Change, Replace
	msg           string
	msgKind       MessageKind

	// colon holds the in-progress command line while mode == ModeExColon;
	// cmdPrefix is the prompt character (':', '/', '?', or '!') that determines
	// how the line is executed on Enter.
	colon          []rune
	cmdPrefix      rune
	cmdLiteralNext bool // ^V: insert next character literally
	cmdHexMode     bool // ^X: collecting a hexadecimal character code
	cmdHexBuf      []rune

	// filterL1/filterL2 hold the line range for a vi ! filter (v_filter in
	// vi/v_ex.c); the status line shows "!" while the command is typed.
	filterL1, filterL2 int64

	// exTranscript is the scrolling output shown while mode == ModeExText.
	exTranscript []string

	// exInput, when non-nil, means the ex input commands (a/i/c) are collecting
	// lines until a sole "." is entered.
	exInput *exInputState

	// pendingOutput is multi-line command output (e.g. :set all) shown over the
	// buffer in vi mode until dismissed; pendingPage selects the visible page.
	pendingOutput []string
	pendingPage   int

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

	// lastAtBuf is the buffer most recently run with @<buf>; @@ repeats it
	// (nvi sp->at_lbuf / SC_AT_SET).
	lastAtBuf rune

	// gMarks tracks the line numbers a running :g/:v still has to visit. The
	// line-edit primitives keep them in sync across inserts/deletes the same way
	// marks are, so a body command that adds or moves lines (t/copy/m) cannot
	// mistrack later matches (nvi flags each matched line in its recno record).
	// A visited/deleted entry is set to -1. nil when no global is running.
	gMarks []int64
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

// statusCols is the width available for the status/message line, falling back to
// the columns option (then 80) before the first Resize.
func (s *screen) statusCols() int {
	if s.cols > 0 {
		return s.cols
	}
	if c := s.opts.Int("columns"); c > 0 {
		return c
	}
	return 80
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

// dirty reports unsaved buffer changes, including edits still inside an open
// beginChange/endChange bracket (e.g. text typed in insert mode before ESC).
func (s *screen) dirty() bool { return s.modified || s.log.Pending() }

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

// setLineKnown is setLine for callers that already hold the current content of
// lno (before), letting the undo log skip a redundant store read. before must
// be the live content of lno; the caller may keep using it afterward.
func (s *screen) setLineKnown(lno int64, before, runes []rune) {
	if lno < 1 {
		return
	}
	if s.store.Lines() == 0 {
		s.log.Insert(1, runes)
		return
	}
	s.log.SetKnown(lno, before, runes)
}

func (s *screen) insertLine(lno int64, runes []rune) {
	s.log.Insert(lno, runes)
	s.marks.LinesInserted(lno, 1)
	s.gMarksInserted(lno)
}

func (s *screen) appendLine(lno int64, runes []rune) {
	s.log.Append(lno, runes)
	s.marks.LinesInserted(lno+1, 1)
	s.gMarksInserted(lno + 1)
}

func (s *screen) deleteLine(lno int64) {
	s.log.Delete(lno)
	s.marks.LinesDeleted(lno, 1)
	s.gMarksDeleted(lno)
}

// gMarksInserted/gMarksDeleted mirror mark.Set's line fixups for the transient
// :g match list (see gMarks). They are no-ops when no global is running.
func (s *screen) gMarksInserted(at int64) {
	for i, ln := range s.gMarks {
		if ln >= at {
			s.gMarks[i] = ln + 1
		}
	}
}

func (s *screen) gMarksDeleted(at int64) {
	for i, ln := range s.gMarks {
		switch {
		case ln == at:
			s.gMarks[i] = -1
		case ln > at:
			s.gMarks[i] = ln - 1
		}
	}
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

// dlCacheCap bounds the memo so extensive navigation of a very large file
// (which never edits, so never resets the cache via the generation) cannot grow
// it without limit. On overflow the cache is cleared; the next paint simply
// refills the visible rows.
const dlCacheCap = 1 << 16

// displayLine returns the DisplayLine for line lno, memoized across input events.
// DisplayLine is a pure function of the line's runes, the tabstop, and the list
// option; the cache is reset whenever any of those change (edit generation,
// tabstop, or list), so a returned DisplayLine is always current.
func (s *screen) displayLine(lno int64) DisplayLine {
	tab := s.opts.Int("tabstop")
	list := s.opts.Bool("list")
	gen := s.log.Gen()
	if !s.dlReady || s.dlGen != gen || s.dlTab != tab || s.dlList != list {
		if s.dlCache == nil {
			s.dlCache = make(map[int64]DisplayLine)
		} else {
			clear(s.dlCache)
		}
		s.dlGen = gen
		s.dlTab = tab
		s.dlList = list
		s.dlReady = true
	} else if dl, ok := s.dlCache[lno]; ok {
		return dl
	}
	if len(s.dlCache) >= dlCacheCap {
		clear(s.dlCache)
	}
	dl := makeDisplayLine(s.lineRunes(lno), tab, list)
	s.dlCache[lno] = dl
	return dl
}

// displayWidth returns the total display width (columns) of line lno, with tabs
// and control characters expanded per the tabstop and list option.
func (s *screen) displayWidth(lno int64) int {
	return DisplayLineWidth(s.displayLine(lno))
}

// displayColOf returns the display column at which rune index col begins on line
// lno.
func (s *screen) displayColOf(lno int64, col int) int {
	runes := s.lineRunes(lno)
	list := s.opts.Bool("list")
	tab := s.opts.Int("tabstop")
	c := 0
	for i := 0; i < col && i < len(runes); i++ {
		c += runeWidth(runes[i], c, tab, list)
	}
	return c
}

// displayCursorColOf returns the display column where the cursor should appear
// for rune index col on line lno (see CursorDisplayColumn).
func (s *screen) displayCursorColOf(lno int64, col int) int {
	return CursorDisplayColumn(s.displayLine(lno), col, s.mode)
}

// colAtDisplay returns the rune index whose cell span contains display column
// dcol on line lno (clamped to the last rune when dcol is past the line end).
func (s *screen) colAtDisplay(lno int64, dcol int) int {
	runes := s.lineRunes(lno)
	list := s.opts.Bool("list")
	tab := s.opts.Int("tabstop")
	c := 0
	for i, r := range runes {
		w := runeWidth(r, c, tab, list)
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

// applyWindowOption re-derives the window option and the vi map after a
// geometry change (nvi f_lines): a default or no-longer-fitting window tracks
// the new text-row count; an explicitly set smaller value survives.
func (s *screen) applyWindowOption() {
	if w := s.opts.Int("window"); !s.winUserSet || w <= 0 || w > s.rows {
		s.opts.i["window"] = s.rows
		s.winUserSet = false
	}
	s.mapRows = s.windowVal()
	s.minMapRows = s.mapRows
	// nvi f_lines: a geometry change also re-derives the related scroll
	// value (displayed by :set all; vi's ^D/^U use defscroll, not this).
	s.opts.i["scroll"] = s.windowVal() / 2
}

// windowVal is the effective window option value (nvi O_WINDOW after
// f_window's clamp): at most the text rows, at least 1. The vi map default
// and the ^F/^B paging distance derive from it.
func (s *screen) windowVal() int {
	w := s.opts.Int("window")
	if w <= 0 || w > s.rows {
		return s.rows
	}
	return w
}

// effectiveMapRows returns the active map height, defaulting to the full screen.
func (s *screen) effectiveMapRows() int {
	if s.mapRows <= 0 || s.mapRows > s.rows {
		return s.rows
	}
	return s.mapRows
}

// scrollToCursor adjusts top so the cursor's line is fully visible, accounting
// for line wrapping (a long line occupies several screen rows). In a reduced
// z[count] map (nvi IS_SMALL), the map grows line-by-line as the cursor moves
// down instead of scrolling top while the rest of the screen stays blank.
func (s *screen) scrollToCursor() {
	if s.rows <= 0 {
		return
	}
	if s.top < 1 {
		s.top = 1
	}

	// Small map: expand toward full height until the cursor fits (vs_refresh.c).
	if s.minMapRows < s.rows {
		// Cursor above the viewport: bring its line to the top.
		if s.cursor.Line < s.top {
			s.top = s.cursor.Line
			return
		}
		for {
			if s.screenRowOf(s.cursor.Line, s.top) < s.effectiveMapRows() {
				return
			}
			if s.mapRows < s.rows {
				s.mapRows++
				continue
			}
			break
		}
		if newTop := s.topForBottom(s.cursor.Line); newTop > s.top {
			s.top = newTop
		}
		return
	}

	s.scrollFull()
}

// scrollFull repositions a full-height viewport around the cursor, following
// nvi's vs_refresh.c section 6: an on-screen cursor doesn't scroll; a cursor
// within half a text screen of an edge scrolls minimally to that edge; a farther
// jump puts the file boundary at the edge when close to it, otherwise centers
// the cursor line. This is why a forward jump to an off-screen line (e.g. 20G)
// lands mid-screen rather than on the bottom row.
func (s *screen) scrollFull() {
	top := s.top
	c := s.cursor.Line
	bottom := s.bottomLine(top)
	if c >= top && c <= bottom {
		return // already visible
	}
	half := s.rows / 2
	if half < 1 {
		half = 1
	}

	if c > bottom {
		// Below the screen.
		if s.rowsBetween(bottom, c, half) < half {
			s.top = s.topForBottom(c) // scroll the cursor to the bottom row
			return
		}
		// Far below: snap the last line to the bottom if near EOF, else center.
		if last := s.lineCount(); s.rowsBetween(c, last+1, s.rows) < half {
			s.top = s.topForBottom(last)
		} else {
			s.top = s.topForMiddle(c)
		}
		return
	}

	// Above the screen.
	if s.rowsBetween(c, top, half) < half {
		s.top = c // scroll the cursor to the top row
	} else if s.rowsBetween(1, c, half) < half {
		s.top = 1 // near SOF: snap the first line to the top
	} else {
		s.top = s.topForMiddle(c)
	}
	if s.top < 1 {
		s.top = 1
	}
}

// rowsBetween returns the number of screen rows spanned by buffer lines [a, b)
// (a <= b), saturating at cap (nvi vs_sm_nlines with a capped count). Used to
// measure how far the cursor sits from a viewport edge or a file boundary.
func (s *screen) rowsBetween(a, b int64, cap int) int {
	n := 0
	for ln := a; ln < b; ln++ {
		n += s.screenLines(ln)
		if n >= cap {
			return cap
		}
	}
	return n
}

// bottomLine returns the last buffer line visible when the viewport starts at
// top, accounting for line wrapping within the active map.
func (s *screen) bottomLine(top int64) int64 {
	mapH := s.effectiveMapRows()
	if mapH <= 0 {
		return top
	}
	used := 0
	last := top
	for ln := top; ln <= s.lineCount(); ln++ {
		r := s.screenLines(ln)
		if used > 0 && used+r > mapH {
			break
		}
		used += r
		last = ln
		if used >= mapH {
			break
		}
	}
	return last
}

// halfPage is the ^D/^U scroll size (nvi defscroll): an explicit count sticks in
// defScroll, otherwise it is half the window rounded up, matching nvi's
// (O_WINDOW+1)/2 default.
func (s *screen) halfPage() int {
	if s.defScroll > 0 {
		return s.defScroll
	}
	if s.rows <= 0 {
		return 1
	}
	return (s.rows + 1) / 2
}

// topForBottom returns the topmost line such that lines [top, bottom] fit within
// the active map when wrapped (bottom shown at the map's last row).
func (s *screen) topForBottom(bottom int64) int64 {
	return s.topForBottomRows(bottom, s.effectiveMapRows())
}

func (s *screen) topForBottomRows(bottom int64, mapH int) int64 {
	used := 0
	top := bottom
	for ln := bottom; ln >= 1; ln-- {
		r := s.screenLines(ln)
		if used+r > mapH {
			break
		}
		used += r
		top = ln
	}
	return top
}

// topForMiddle returns the topmost line such that line target's first screen row
// sits near the middle of the map when wrapped (nvi P_MIDDLE / vs_sm_fill).
func (s *screen) topForMiddle(target int64) int64 {
	return s.topForMiddleRows(target, s.effectiveMapRows())
}

func (s *screen) topForMiddleRows(target int64, mapH int) int64 {
	if mapH <= 0 {
		return 1
	}
	mid := mapH / 2
	used := 0
	top := target
	for ln := target - 1; ln >= 1; ln-- {
		r := s.screenLines(ln)
		if used+r > mid {
			break
		}
		used += r
		top = ln
	}
	if top < 1 {
		top = 1
	}
	return top
}

// screenRowOf returns the 0-based screen row where line's first row appears when
// top is the first buffer line shown.
func (s *screen) screenRowOf(line, top int64) int {
	row := 0
	for ln := top; ln < line; ln++ {
		row += s.screenLines(ln)
	}
	return row
}
