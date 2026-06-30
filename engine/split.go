package engine

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"govi/engine/buffer"
	"govi/engine/mark"
	"govi/engine/undo"
)

// Split-screen support. This mirrors nvi's multi-window subsystem (vi/vs_split.c,
// vi/v_screen.c). Screens are stacked in the display and kept sorted by their
// row/column offset; the active screen is e.screens[e.cur] (== e.scr). Registers
// and maps are shared across split screens (nvi keeps them in the WIN/GS); each
// screen has its own buffer, marks, cursor, viewport, and a copied set of
// options.

// setCur makes screen i the active one, keeping e.scr in step.
func (e *Engine) setCur(i int) {
	if i < 0 || i >= len(e.screens) {
		return
	}
	e.cur = i
	e.scr = e.screens[i]
}

// newSiblingScreen creates an empty screen that shares the active screen's
// registers and maps and copies its options (nvi copies the option array to a
// new screen but shares cut buffers and the map sequences). Geometry is filled
// in later by splitHoriz; the buffer is filled in by the caller.
func (e *Engine) newSiblingScreen() *screen {
	parent := e.scr
	store := buffer.NewMem()
	ns := &screen{
		store:         store,
		log:           undo.New(store),
		marks:         mark.New(),
		regs:          parent.regs, // shared cut buffers (nvi WIN cutq)
		maps:          parent.maps, // shared maps (nvi GS seq); struct of maps
		opts:          parent.opts.clone(),
		cursor:        Pos{Line: 1, Col: 0},
		top:           1,
		mode:          ModeCommand,
		showModeLabel: "Command",
	}
	return ns
}

// splitHoriz divides the active screen in half and places ns (already loaded
// with its buffer and cursor) in the freed half, then makes ns the active
// screen. It follows vi/vs_split.c: half = total/2; if the cursor is in the
// bottom half of the current screen the new screen takes the top, otherwise the
// current screen keeps the top and the new screen takes the bottom. In govi a
// screen's display height is rows+1 (text rows plus its own status row).
func (e *Engine) splitHoriz(ns *screen) error {
	s := e.scr
	disp := s.rows + 1 // total display rows of the screen being split
	if disp < 4 {
		return fmt.Errorf("Screen must be larger than %d lines to split", 3)
	}
	half := disp / 2

	// 0-based screen row of the cursor within the current screen.
	curRow := s.screenRowOf(s.cursor.Line, s.top)
	splitup := curRow+1 >= half // cursor is in the bottom half

	ns.cols = s.cols
	ns.coff = s.coff
	if splitup {
		// New screen takes the top (disp-half rows); old becomes the bottom.
		ns.rows = (disp - half) - 1
		ns.roff = s.roff
		s.rows = half - 1
		s.roff += disp - half
	} else {
		// Old screen keeps the top (disp-half rows); new takes the bottom.
		s.rows = (disp - half) - 1
		ns.rows = half - 1
		ns.roff = s.roff + (disp - half)
	}

	for _, sc := range []*screen{s, ns} {
		sc.mapRows = sc.rows
		sc.minMapRows = sc.rows
		sc.defScroll = 0
		sc.clampCursor()
		sc.scrollToCursor()
	}

	e.insertScreen(ns) // sorts the screen list and focuses ns
	return nil
}

// vsplitVert divides the active screen vertically, placing ns (already loaded
// with its buffer) in a new screen to the right and making it active. It follows
// vi/vs_split.c vs_vsplit: the screen is split in half by columns with one column
// sacrificed as a divider, and the new screen always goes to the right.
func (e *Engine) vsplitVert(ns *screen) error {
	s := e.scr
	if s.cols/2 <= minScreenCols {
		return fmt.Errorf("Screen must be larger than %d columns to split", minScreenCols*2)
	}
	cols := s.cols / 2
	ns.cols = s.cols - cols - 1 // right part, less the divider column
	ns.coff = s.coff + cols + 1
	ns.roff = s.roff
	ns.rows = s.rows
	s.cols = cols // left part
	s.cursor.Col = 0

	for _, sc := range []*screen{s, ns} {
		sc.mapRows = sc.rows
		sc.minMapRows = sc.rows
		sc.defScroll = 0
		sc.clampCursor()
		sc.scrollToCursor()
	}
	e.insertScreen(ns)
	return nil
}

// minScreenCols mirrors nvi's MINIMUM_SCREEN_COLS: a vertical split needs each
// half to be wider than this.
const minScreenCols = 20

// insertScreen adds ns to the displayed-screen list, re-sorts the list into
// display order (top-to-bottom, then left-to-right), and makes ns active.
func (e *Engine) insertScreen(ns *screen) {
	e.screens = append(e.screens, ns)
	sort.SliceStable(e.screens, func(i, j int) bool {
		a, b := e.screens[i], e.screens[j]
		if a.roff != b.roff {
			return a.roff < b.roff
		}
		return a.coff < b.coff
	})
	for i, sc := range e.screens {
		if sc == ns {
			e.setCur(i)
			break
		}
	}
}

// removeScreen drops s from the displayed-screen list.
func (e *Engine) removeScreen(s *screen) {
	out := make([]*screen, 0, len(e.screens))
	for _, sc := range e.screens {
		if sc != s {
			out = append(out, sc)
		}
	}
	e.screens = out
}

// switchScreen implements vi-mode ^W: move to the next screen in display order,
// wrapping back to the first (vi/v_screen.c v_screen). Both the screen left and
// the screen entered get the transient status line (nvi sets SC_STATUS on both).
func (e *Engine) switchScreen() {
	if len(e.screens) <= 1 {
		e.scr.msg, e.scr.msgKind = "No other screen to switch to", MsgError
		return
	}
	old := e.scr
	e.setCur((e.cur + 1) % len(e.screens))
	e.setStatusMsg(old)
	e.setStatusMsg(e.scr)
}

// setStatusMsg sets a screen's transient status line (the long msgq_status form
// shown right after a split or switch). The next command on the active screen
// clears it (vi.go commandKey), reverting to the persistent modeline -- matching
// nvi's SC_STATUS -> vs_modeline flip.
func (e *Engine) setStatusMsg(s *screen) {
	s.msg = e.screenStatusMsg(s)
	s.msgKind = MsgInfo
}

// screenStatusMsg builds the transient per-screen status nvi shows right after a
// split or screen switch (common/msg.c msgq_status without MSTAT_SHOWLAST),
// truncated to the screen width the nvi way.
func (e *Engine) screenStatusMsg(s *screen) string {
	name := s.name
	if name == "" {
		name = "[No file]"
	}
	var b strings.Builder
	b.WriteString(name)
	nameEnd := len([]rune(b.String())) // index of the ':' after the name (nvi np)
	b.WriteString(": ")
	needSep := false
	if s.name == "" && s.dirty() {
		b.WriteString("new file")
		needSep = true
	} else {
		if s.nameChanged {
			b.WriteString("name changed")
			needSep = true
		}
		if needSep {
			b.WriteString(", ")
		}
		if s.dirty() {
			b.WriteString("modified")
		} else {
			b.WriteString("unmodified")
		}
		needSep = true
	}
	if s.opts.Bool("readonly") {
		if needSep {
			b.WriteString(", ")
		}
		b.WriteString("readonly")
		needSep = true
	}
	if needSep {
		b.WriteString(": ")
	}
	if n := s.lineCount(); n <= 1 && s.lineLen(1) == 0 {
		b.WriteString("empty file")
	} else {
		b.WriteString("line ")
		b.WriteString(strconv.FormatInt(s.cursor.Line, 10))
	}
	return nviTruncateStatus([]rune(b.String()), nameEnd, s.cols)
}

// nviTruncateStatus shortens an over-long status line the way common/msg.c does
// for a split modeline (MSTAT_TRUNCATE): keep a path tail starting at a '/' that
// fits cols-3 columns, prefixed with "...".
func nviTruncateStatus(r []rune, nameEnd, cols int) string {
	if cols <= 0 || len(r) <= cols {
		return string(r)
	}
	s := 0
	for s < nameEnd && (r[s] != '/' || (len(r)-s) > cols-3) {
		s++
	}
	if s >= nameEnd {
		keep := cols - 5
		if keep < 0 {
			keep = 0
		}
		if keep > len(r) {
			keep = len(r)
		}
		return "... " + string(r[len(r)-keep:])
	}
	return "..." + string(r[s:])
}

// horizNeighbors returns the screens whose full bottom/top border touches s's
// top/bottom border (sharing the same column span). For a horizontal stack these
// are the screens directly above and below s.
func (e *Engine) horizNeighbors(s *screen) (above, below *screen) {
	for _, t := range e.screens {
		if t == s || t.coff != s.coff || t.cols != s.cols {
			continue
		}
		if t.roff+t.rows+1 == s.roff {
			above = t
		}
		if t.roff == s.roff+s.rows+1 {
			below = t
		}
	}
	return
}

// vertNeighbors returns the screens whose full right/left border touches s's
// left/right border (sharing the same row span). For a vertical split these are
// the screens directly to the left and right of s.
func (e *Engine) vertNeighbors(s *screen) (left, right *screen) {
	for _, t := range e.screens {
		if t == s || t.roff != s.roff || t.rows != s.rows {
			continue
		}
		if t.coff+t.cols+1 == s.coff {
			left = t
		}
		if t.coff == s.coff+s.cols+1 {
			right = t
		}
	}
	return
}

// closeCurrentScreen discards the active screen and folds its display space into
// a neighbor, then makes that neighbor active (vi/vs_split.c vs_discard/vs_join,
// horizontal axis). It must only be called when more than one screen is
// displayed.
func (e *Engine) closeCurrentScreen() {
	if len(e.screens) <= 1 {
		return
	}
	s := e.scr
	// nvi vs_join checks the vertical axis (left/right neighbors) before the
	// horizontal axis (above/below).
	left, right := e.vertNeighbors(s)
	above, below := e.horizNeighbors(s)

	var target *screen
	switch {
	case left != nil: // VERT_PRECEDE: the screen on the left grows rightward
		left.cols += s.cols + 1
		target = left
	case right != nil: // VERT_FOLLOW: the screen on the right moves left and grows
		right.coff = s.coff
		right.cols += s.cols + 1
		target = right
	case above != nil: // HORIZ_PRECEDE: the screen above grows downward
		above.rows += s.rows + 1
		target = above
	case below != nil: // HORIZ_FOLLOW: the screen below moves up and grows
		below.roff = s.roff
		below.rows += s.rows + 1
		target = below
	}

	if s.file != nil {
		s.file.Close()
		s.file = nil
	}
	e.removeScreen(s)

	if target == nil {
		// No full-border neighbor (only reachable with vertical splits, handled
		// later); redistribute to avoid leaving a gap.
		e.relayout()
		if e.cur >= len(e.screens) {
			e.setCur(len(e.screens) - 1)
		} else {
			e.setCur(e.cur)
		}
		return
	}

	target.mapRows = target.rows
	target.minMapRows = target.rows
	target.defScroll = 0
	target.clampCursor()
	target.scrollToCursor()
	for i, sc := range e.screens {
		if sc == target {
			e.setCur(i)
			break
		}
	}
}

// finishQuit ends an edit of the active screen: when other screens are
// displayed it closes just this one (folding its space into a neighbor), as nvi
// does for :q in a split; otherwise it marks the whole editor to exit. Callers
// must have already done any write and dirty/force checks.
func (e *Engine) finishQuit() {
	if len(e.screens) > 1 {
		e.closeCurrentScreen()
		return
	}
	e.removeRecovery()
	e.quit = true
}

// editNewScreen opens name in a new horizontally split screen (the E_NEWSCREEN
// path of nvi's :edit/:Edit). An empty name re-edits the current screen's file
// in the new screen.
func (e *Engine) editNewScreen(name string) error {
	return e.newScreenEdit(name, (*Engine).splitHoriz)
}

// vsplitNewScreen opens name in a new vertically split screen (nvi :vsplit).
func (e *Engine) vsplitNewScreen(name string) error {
	return e.newScreenEdit(name, (*Engine).vsplitVert)
}

// newScreenEdit loads name into a new sibling screen and places it with the
// given split function, then sets both screens' transient status line.
func (e *Engine) newScreenEdit(name string, split func(*Engine, *screen) error) error {
	name = strings.TrimSpace(name)
	if name == "" {
		name = e.scr.name
	}
	parent := e.scr
	ns := e.newSiblingScreen()
	e.loadScreenFile(ns, name)
	if err := split(e, ns); err != nil {
		// Splitting failed (screen too small): drop the new screen and report.
		if ns.file != nil {
			ns.file.Close()
		}
		return err
	}
	// Both screens show the transient status line after the split (nvi sets
	// SC_STATUS); it reverts to the modeline on the next command.
	e.setStatusMsg(parent)
	if ns.msg == "" { // keep a load error if loadScreenFile reported one
		e.setStatusMsg(ns)
	}
	return nil
}

// loadScreenFile loads name into screen s, paging from disk when possible. It
// does not touch engine-global lock/recovery/argv state (a later stage moves
// those per-screen); a missing file starts an empty "new file" buffer.
func (e *Engine) loadScreenFile(s *screen, name string) {
	name = strings.TrimSpace(name)
	s.name = name
	if name == "" {
		return // keep the empty buffer from newSiblingScreen
	}
	resolved := e.canonicalPath(name)
	store, fh, err := buffer.NewPagedFile(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			s.msg = fmt.Sprintf("%s: new file: line 1", name)
			s.msgKind = MsgInfo
		} else {
			s.msg = err.Error()
			s.msgKind = MsgError
		}
		return
	}
	s.store = store
	s.file = fh
	s.log = undo.New(store)
	s.dlReady = false
}
