// Command bridge builds the govi editor engine into a C archive (libgovi.a)
// that a native host application links against. It is the embedding proof: the
// macOS AppKit app in gui/macos drives this library and renders the resulting
// character grid in a custom NSView, with the engine running in-process — no
// terminal, and no exec of nvi.
//
// The library is multi-instance: GoviStart returns an opaque handle for one
// editor (one window), and every other call takes that handle as its first
// argument, so the host can open many independent windows. The host calls all
// functions from a single thread (its UI thread): each engine is single-
// threaded by design. The flow each frame is:
//
//  1. GoviStart(path) -> handle, once per window.
//  2. GoviResize(h, rows, cols) whenever the window/cell geometry changes.
//  3. Feed input with GoviKeyRune / GoviKeySpecial / GoviText / GoviInterrupt.
//  4. GoviCompose(h, rows, cols), then read GoviRowText / GoviCursor* to redraw.
//  5. Check GoviTakeBell, GoviShouldQuit, and the *Pending timers to schedule
//     a follow-up GoviFireTimeout / GoviSyncRecovery.
//  6. GoviClose(h) when the window closes.
//
// Strings returned by GoviRowText and GoviTitle are heap-allocated with C
// malloc; the caller must free() them.
package main

// #include <stdlib.h>
import "C"

import (
	"sync"
	"unsafe"

	"govi/engine"
	"govi/frontend/grid"
)

// host implements engine.Frontend. Because the host application pulls the
// composed grid on demand after each input, Render only needs to note that a
// repaint is due; Bell and SetTitle latch state the host reads back.
type host struct {
	bellPending bool
	title       string
}

func (h *host) Render(_ engine.View, _ engine.ChangeSet) {}
func (h *host) Bell()                                    { h.bellPending = true }
func (h *host) SetTitle(s string)                        { h.title = s }

// instance is one editor: an engine, its frontend, the last composed grid, and
// the host-side selection state. Instances are keyed by an integer handle so no
// Go pointer crosses the C boundary.
type instance struct {
	eng    *engine.Engine
	fe     *host
	gr     grid.Grid
	gridOK bool
	rows   int
	cols   int

	bufSelActive bool
	bufSelA      engine.Pos
	bufSelB      engine.Pos

	screenSelActive bool
	screenSelLinear bool // false = axis-aligned rectangle; true = reading-order
	screenSelA      grid.Cell
	screenSelB      grid.Cell
}

// instMu guards insts and nextHandle. The GUI host now runs each editor's input
// on its own serial queue (so a long command does not freeze the UI and a ^C can
// reach Engine.Interrupt), so get may be called from a window's queue thread
// concurrently with GoviStart/GoviClose on the main thread. Each engine is still
// single-threaded; only the handle table is shared.
var (
	instMu     sync.RWMutex
	insts      = map[int64]*instance{}
	nextHandle int64
)

// defaultMode is the GUI's selection-mode default applied to each new engine
// before LoadStartup, so an .exrc :set mode can still override it. The host sets
// it from its Settings via GoviSetDefaultMode.
var defaultMode = "contextual"

// GoviSetDefaultMode sets the selection mode applied to engines created afterward.
//
//export GoviSetDefaultMode
func GoviSetDefaultMode(mode *C.char) { defaultMode = C.GoString(mode) }

// modeCode maps a selection-mode name to the GUI's 0/1/2 code.
func modeCode(name string) C.int {
	switch name {
	case "terminal":
		return 0
	case "gui":
		return 1
	default:
		return 2 // contextual
	}
}

// modeName maps a 0/1/2 code back to a selection-mode name.
func modeName(code C.int) string {
	switch code {
	case 0:
		return "terminal"
	case 1:
		return "gui"
	default:
		return "contextual"
	}
}

// GoviMode returns this editor's selection mode as 0=terminal, 1=gui,
// 2=contextual.
//
//export GoviMode
func GoviMode(h C.longlong) C.int {
	in := get(h)
	if in == nil {
		return 2
	}
	return modeCode(in.eng.StrOption("mode"))
}

// GoviSetMode sets this editor's selection mode (0/1/2), e.g. when the Settings
// dialog changes while the window is open.
//
//export GoviSetMode
func GoviSetMode(h C.longlong, mode C.int) {
	if in := get(h); in != nil {
		_ = in.eng.SetStrOption("mode", modeName(mode))
	}
}

// GoviInsertActive reports whether the editor is in insert (or replace) mode,
// which mode=contextual uses to decide if a selection captures input.
//
//export GoviInsertActive
func GoviInsertActive(h C.longlong) C.int {
	in := get(h)
	if in == nil {
		return 0
	}
	var active bool
	in.eng.WithView(func(v engine.View) {
		active = v.Mode() == engine.ModeInsert || v.Mode() == engine.ModeReplace
	})
	return boolToC(active)
}

func get(h C.longlong) *instance {
	instMu.RLock()
	defer instMu.RUnlock()
	return insts[int64(h)]
}

func main() {} // required for c-archive builds

// GoviStart creates an editor and opens path (empty path starts a scratch
// buffer). foreground and background are host defaults for new tabs (#RGB,
// #RRGGBB, or a color name; empty means system default). Startup files and
// :set may override them per tab. Returns a handle, or 0 on error.
//
//export GoviStart
func GoviStart(path, foreground, background, cwd *C.char, silent C.int) C.longlong {
	in := &instance{fe: &host{}}
	in.eng = engine.New(in.fe, engine.Options{})
	_ = in.eng.SetStrOption("foreground", cString(foreground))
	_ = in.eng.SetStrOption("background", cString(background))
	// Apply the GUI's selection-mode default before LoadStartup so an .exrc :set
	// mode can still override it.
	_ = in.eng.SetStrOption("mode", defaultMode)
	// Set the working directory before LoadStartup so a project-local ./.nexrc or
	// .exrc (e.g. one that sets background) is read from the directory govi -g was
	// run in, not the app's launch cwd ("/"). The host passes cwd from the
	// validated govi:// payload (or the configured initial dir for new windows).
	if c := C.GoString(cwd); c != "" {
		in.eng.SetLaunchContext(engine.LaunchContext{Cwd: c})
	}
	p := C.GoString(path)
	// silent (-s) skips startup files / EXINIT, matching the terminal frontend.
	if silent == 0 {
		if err := in.eng.LoadStartup(); err != nil {
			return 0
		}
		if in.eng.ShouldQuit() {
			return 0
		}
	}
	in.eng.InitCwd()
	if p != "" {
		if err := in.eng.Open(p); err != nil {
			return 0
		}
	}
	instMu.Lock()
	nextHandle++
	h := nextHandle
	insts[h] = in
	instMu.Unlock()
	return C.longlong(h)
}

// GoviSetCwd sets the editor's working directory (per tab), used for relative
// :e/:r/:w and :!cmd. The host passes the cwd from a validated govi:// launch
// payload; empty editors are left at the engine's default cwd.
//
//export GoviSetCwd
func GoviSetCwd(h C.longlong, cwd *C.char) {
	if in := get(h); in != nil {
		in.eng.SetCwd(C.GoString(cwd))
	}
}

// GoviSetTemporary marks the editor's buffer as backed by a throwaway temp file
// (govi -g with no file): quitting then warns and discards like nvi instead of
// silently writing the soon-deleted temp.
//
//export GoviSetTemporary
func GoviSetTemporary(h C.longlong) {
	if in := get(h); in != nil {
		in.eng.SetTemporary()
	}
}

// GoviIsTemporary reports whether this editor's buffer is a throwaway temp file
// (govi -g with no file), so the host can warn that closing discards it.
//
//export GoviIsTemporary
func GoviIsTemporary(h C.longlong) C.int {
	in := get(h)
	return boolToC(in != nil && in.eng.IsTemporary())
}

// GoviTempPending reports whether closing would discard a temporary buffer that
// holds content (true even after :w, since the temp is still thrown away). The
// host warns on close/quit when this or GoviModified is set.
//
//export GoviTempPending
func GoviTempPending(h C.longlong) C.int {
	in := get(h)
	return boolToC(in != nil && in.eng.TempDiscardPending())
}

// GoviCwd returns this editor's working directory (malloc'd; caller frees).
//
//export GoviCwd
func GoviCwd(h C.longlong) *C.char {
	if in := get(h); in != nil {
		in.eng.InitCwd()
		return C.CString(in.eng.Cwd())
	}
	return C.CString("")
}

// GoviForegroundSpec returns this tab's foreground color spec (malloc'd;
// caller frees). Empty means system default.
//
//export GoviForegroundSpec
func GoviForegroundSpec(h C.longlong) *C.char {
	if in := get(h); in != nil {
		return C.CString(in.eng.StrOption("foreground"))
	}
	return C.CString("")
}

// GoviBackgroundSpec returns this tab's background color spec (malloc'd;
// caller frees). Empty means system default.
//
//export GoviBackgroundSpec
func GoviBackgroundSpec(h C.longlong) *C.char {
	if in := get(h); in != nil {
		return C.CString(in.eng.StrOption("background"))
	}
	return C.CString("")
}

// GoviClose disposes of an editor and releases its file handle.
//
//export GoviClose
func GoviClose(h C.longlong) {
	instMu.Lock()
	in := insts[int64(h)]
	delete(insts, int64(h))
	instMu.Unlock()
	if in != nil {
		in.eng.Close()
	}
}

// GoviResize sets the viewport geometry. rows is the full window height in
// cells including the status row; cols is the width in cells.
//
//export GoviResize
func GoviResize(h C.longlong, rows, cols C.int) {
	in := get(h)
	if in == nil {
		return
	}
	tr := int(rows) - 1 // the engine reserves the status row itself
	if tr < 1 {
		tr = 1
	}
	in.eng.Resize(tr, int(cols))
}

// GoviKeyRune feeds a typed character with optional modifiers (bit flags
// matching engine.Mod: 1=Ctrl, 2=Alt, 4=Shift).
//
//export GoviKeyRune
func GoviKeyRune(h C.longlong, r C.int, mods C.int) {
	if in := get(h); in != nil {
		in.eng.Input(engine.KeyEvent{Rune: rune(r), Mods: engine.Mod(mods)})
	}
}

// GoviKeySpecial feeds a non-text key. key matches the engine.SpecialKey
// enumeration (1=Escape, 2=Enter, 3=Tab, 4=Backspace, 5=Delete, 6=Up, 7=Down,
// 8=Left, 9=Right, 10=Home, 11=End, 12=PageUp, 13=PageDown, 14=Insert).
//
//export GoviKeySpecial
func GoviKeySpecial(h C.longlong, key C.int, mods C.int) {
	if in := get(h); in != nil {
		in.eng.Input(engine.KeyEvent{Key: engine.SpecialKey(key), Mods: engine.Mod(mods)})
	}
}

// GoviText feeds a run of literal text (e.g. a paste), bypassing map expansion.
//
//export GoviText
func GoviText(h C.longlong, s *C.char) {
	if in := get(h); in != nil {
		in.eng.Input(engine.StringEvent{Text: C.GoString(s)})
	}
}

// GoviInterrupt records the user's ^C out of band. Engine.Interrupt touches only
// the atomic interrupt flag and the interrupt channel, so -- uniquely among the
// engine entry points -- it is safe to call concurrently with a command running
// on another thread. The GUI host calls it from the main thread while a long
// command runs on its serial engine queue, so an in-progress search / :s / :g /
// :! observes the ^C and aborts.
//
// It deliberately does NOT feed an InterruptEvent here: the between-commands
// colon-line cancel is delivered as the ordinary ^C key (GoviKeyRune 3) on the
// engine queue, which must be serialized behind any command already running
// there rather than mutating engine state from this concurrent call.
//
//export GoviInterrupt
func GoviInterrupt(h C.longlong) {
	if in := get(h); in != nil {
		in.eng.Interrupt()
	}
}

// GoviFireTimeout delivers a timeout event, resolving an ambiguous map prefix
// or clearing a showmatch flash.
//
//export GoviFireTimeout
func GoviFireTimeout(h C.longlong) {
	if in := get(h); in != nil {
		in.eng.Input(engine.TimeoutEvent{})
	}
}

// GoviShouldQuit reports whether a quit command was issued (host should close
// the window).
//
//export GoviShouldQuit
func GoviShouldQuit(h C.longlong) C.int {
	in := get(h)
	return boolToC(in != nil && in.eng.ShouldQuit())
}

// GoviClearQuit resets the quit flag after the host aborts closing a window.
//
//export GoviClearQuit
func GoviClearQuit(h C.longlong) {
	if in := get(h); in != nil {
		in.eng.ClearQuit()
	}
}

// GoviModified reports whether the buffer has unsaved changes.
//
//export GoviModified
func GoviModified(h C.longlong) C.int {
	in := get(h)
	if in == nil {
		return 0
	}
	mod := false
	in.eng.WithView(func(v engine.View) { mod = v.Modified() })
	return boolToC(mod)
}

// GoviSave writes the buffer to path (NULL or "" uses the current file).
// Returns 0 on success, 1 on error.
//
//export GoviSave
func GoviSave(h C.longlong, path *C.char) C.int {
	in := get(h)
	if in == nil {
		return 1
	}
	p := ""
	if path != nil {
		p = C.GoString(path)
	}
	if err := in.eng.Save(p); err != nil {
		return 1
	}
	return 0
}

// GoviSaveAs renames the buffer to path and writes it there (:f path then :w).
// Returns 0 on success, 1 on error.
//
//export GoviSaveAs
func GoviSaveAs(h C.longlong, path *C.char) C.int {
	in := get(h)
	if in == nil || path == nil {
		return 1
	}
	if err := in.eng.SaveAs(C.GoString(path)); err != nil {
		return 1
	}
	return 0
}

// GoviMapPending reports whether input is buffered awaiting more keys.
//
//export GoviMapPending
func GoviMapPending(h C.longlong) C.int {
	in := get(h)
	return boolToC(in != nil && in.eng.MapPending())
}

// GoviMatchPending reports whether a showmatch bracket flash is active.
//
//export GoviMatchPending
func GoviMatchPending(h C.longlong) C.int {
	in := get(h)
	return boolToC(in != nil && in.eng.MatchPending())
}

// GoviMatchTimeMS returns the showmatch flash duration in milliseconds.
//
//export GoviMatchTimeMS
func GoviMatchTimeMS(h C.longlong) C.int {
	if in := get(h); in != nil {
		return C.int(in.eng.MatchTime() * 100)
	}
	return 0
}

// GoviNeedsRecoverySync reports whether the recovery file should be flushed.
//
//export GoviNeedsRecoverySync
func GoviNeedsRecoverySync(h C.longlong) C.int {
	in := get(h)
	return boolToC(in != nil && in.eng.NeedsRecoverySync())
}

// GoviSyncRecovery flushes pending changes to the recovery file.
//
//export GoviSyncRecovery
func GoviSyncRecovery(h C.longlong) {
	if in := get(h); in != nil {
		in.eng.SyncRecovery()
	}
}

// GoviTakeBell returns 1 and clears the flag if a bell occurred since the last
// call, so the host can play NSBeep.
//
//export GoviTakeBell
func GoviTakeBell(h C.longlong) C.int {
	in := get(h)
	if in == nil || !in.fe.bellPending {
		return 0
	}
	in.fe.bellPending = false
	return 1
}

// GoviTitle returns the desired window title (malloc'd; caller frees).
//
//export GoviTitle
func GoviTitle(h C.longlong) *C.char {
	if in := get(h); in != nil {
		return C.CString(in.fe.title)
	}
	return C.CString("")
}

// GoviCompose lays out the current view into a rows x cols grid cached for the
// row/cursor accessors.
//
//export GoviCompose
func GoviCompose(h C.longlong, rows, cols C.int) {
	in := get(h)
	if in == nil {
		return
	}
	in.rows, in.cols = int(rows), int(cols)
	var sel *grid.Selection
	if in.bufSelActive {
		sel = &grid.Selection{A: in.bufSelA, B: in.bufSelB}
	}
	in.eng.WithView(func(v engine.View) {
		in.gr = grid.ComposeSel(v, int(rows), int(cols), sel)
		if in.screenSelActive {
			ss := &grid.ScreenSelection{A: in.screenSelA, B: in.screenSelB}
			if in.screenSelLinear {
				// View-aware so the line-number gutter is not highlighted on a
				// multi-row selection's continuation rows.
				grid.ApplyScreenLinearSelView(&in.gr, v, int(rows), int(cols), ss)
			} else {
				grid.ApplyScreenSel(&in.gr, ss)
			}
		}
	})
	in.gridOK = true
}

// GoviRows / GoviCols return the cached grid dimensions.
//
//export GoviRows
func GoviRows(h C.longlong) C.int {
	if in := get(h); in != nil && in.gridOK {
		return C.int(in.gr.Rows)
	}
	return 0
}

//export GoviCols
func GoviCols(h C.longlong) C.int {
	if in := get(h); in != nil && in.gridOK {
		return C.int(in.gr.Cols)
	}
	return 0
}

// GoviRowText returns row y of the cached grid as a UTF-8 string with blank
// cells rendered as spaces and trailing blanks trimmed (malloc'd; caller frees).
//
//export GoviRowText
func GoviRowText(h C.longlong, y C.int) *C.char {
	in := get(h)
	if in == nil || !in.gridOK || int(y) < 0 || int(y) >= in.gr.Rows {
		return C.CString("")
	}
	runes := make([]rune, in.gr.Cols)
	last := -1
	for x := 0; x < in.gr.Cols; x++ {
		r := in.gr.At(x, int(y)).Rune
		if r == 0 {
			r = ' '
		} else {
			last = x
		}
		runes[x] = r
	}
	return C.CString(string(runes[:last+1]))
}

// GoviRowStyle returns a string the same length as GoviRowText's row in which
// each character is '1' where that cell is highlighted (selection / reverse
// video) and '0' otherwise (malloc'd; caller frees).
//
//export GoviRowStyle
func GoviRowStyle(h C.longlong, y C.int) *C.char {
	in := get(h)
	if in == nil || !in.gridOK || int(y) < 0 || int(y) >= in.gr.Rows {
		return C.CString("")
	}
	flags := make([]byte, in.gr.Cols)
	last := -1
	for x := 0; x < in.gr.Cols; x++ {
		if in.gr.At(x, int(y)).Style&engine.StyleReverse != 0 {
			flags[x] = '1'
			last = x
		} else {
			flags[x] = '0'
		}
	}
	return C.CString(string(flags[:last+1]))
}

// GoviCellToPos maps a screen cell (x, y) to the buffer caret it sits on,
// writing the 1-based line into *line and the rune index into *col.
//
//export GoviCellToPos
func GoviCellToPos(h C.longlong, x, y C.int, line *C.longlong, col *C.int) {
	in := get(h)
	if in == nil {
		return
	}
	in.eng.WithView(func(v engine.View) {
		p := grid.Locate(v, in.rows, in.cols, int(x), int(y))
		*line, *col = C.longlong(p.Line), C.int(p.Col)
	})
}

// GoviWordRange writes the caret range a double-click at cell (x, y) selects
// (word under the cursor) into the out-params.
//
//export GoviWordRange
func GoviWordRange(h C.longlong, x, y C.int, l1 *C.longlong, c1 *C.int, l2 *C.longlong, c2 *C.int) {
	in := get(h)
	if in == nil {
		return
	}
	in.eng.WithView(func(v engine.View) {
		p := grid.Locate(v, in.rows, in.cols, int(x), int(y))
		a, b := in.eng.WordRange(p.Line, p.Col)
		*l1, *c1 = C.longlong(a.Line), C.int(a.Col)
		*l2, *c2 = C.longlong(b.Line), C.int(b.Col)
	})
}

// GoviLineRange writes the caret range a triple-click at cell (x, y) selects
// (the whole logical line) into the out-params.
//
//export GoviLineRange
func GoviLineRange(h C.longlong, x, y C.int, l1 *C.longlong, c1 *C.int, l2 *C.longlong, c2 *C.int) {
	in := get(h)
	if in == nil {
		return
	}
	in.eng.WithView(func(v engine.View) {
		p := grid.Locate(v, in.rows, in.cols, int(x), int(y))
		a, b := in.eng.LineSelectRange(p.Line)
		*l1, *c1 = C.longlong(a.Line), C.int(a.Col)
		*l2, *c2 = C.longlong(b.Line), C.int(b.Col)
	})
}

// GoviSetSelection sets (active != 0) or clears the buffer caret range [a, b)
// drawn highlighted on the next GoviCompose. Clears any screen selection.
//
//export GoviSetSelection
func GoviSetSelection(h C.longlong, active C.int, l1 C.longlong, c1 C.int, l2 C.longlong, c2 C.int) {
	in := get(h)
	if in == nil {
		return
	}
	in.bufSelActive = active != 0
	in.bufSelA = engine.Pos{Line: int64(l1), Col: int(c1)}
	in.bufSelB = engine.Pos{Line: int64(l2), Col: int(c2)}
	if active != 0 {
		in.screenSelActive = false
	}
}

// GoviSetScreenSelection sets or clears a screen-cell selection. active: 0=none,
// 1=axis-aligned rectangle (Option-drag), 2=reading-order (overlay/ex drag).
// Corners may be given in either order. Clears any buffer selection.
//
//export GoviSetScreenSelection
func GoviSetScreenSelection(h C.longlong, active, linear C.int, ax, ay, bx, by C.int) {
	in := get(h)
	if in == nil {
		return
	}
	in.screenSelActive = active != 0
	in.screenSelLinear = linear != 0
	in.screenSelA = grid.Cell{X: int(ax), Y: int(ay)}
	in.screenSelB = grid.Cell{X: int(bx), Y: int(by)}
	if active != 0 {
		in.bufSelActive = false
	}
}

// GoviScreenRangeText returns rectangular screen selection text (malloc'd;
// caller frees).
//
//export GoviScreenRangeText
func GoviScreenRangeText(h C.longlong, ax, ay, bx, by C.int) *C.char {
	in := get(h)
	if in == nil || !in.gridOK {
		return C.CString("")
	}
	sel := grid.ScreenSelection{
		A: grid.Cell{X: int(ax), Y: int(ay)},
		B: grid.Cell{X: int(bx), Y: int(by)},
	}
	return C.CString(grid.ScreenRangeText(in.gr, sel))
}

// GoviScreenLinearRangeText returns reading-order screen selection text
// (malloc'd; caller frees).
//
//export GoviScreenLinearRangeText
func GoviScreenLinearRangeText(h C.longlong, ax, ay, bx, by C.int) *C.char {
	in := get(h)
	if in == nil || !in.gridOK {
		return C.CString("")
	}
	sel := grid.ScreenSelection{
		A: grid.Cell{X: int(ax), Y: int(ay)},
		B: grid.Cell{X: int(bx), Y: int(by)},
	}
	return C.CString(grid.ScreenLinearRangeText(in.gr, sel))
}

// GoviScreenToBuffer maps screen cell (x, y) to a buffer caret. Returns 1 and
// writes *line/*col when the cell is editable buffer text; 0 otherwise.
//
//export GoviScreenToBuffer
func GoviScreenToBuffer(h C.longlong, x, y C.int, line *C.longlong, col *C.int) C.int {
	in := get(h)
	if in == nil {
		return 0
	}
	var p engine.Pos
	var ok bool
	in.eng.WithView(func(v engine.View) {
		p, ok = grid.ScreenToBuffer(v, in.rows, in.cols, int(x), int(y))
	})
	if !ok {
		return 0
	}
	*line, *col = C.longlong(p.Line), C.int(p.Col)
	return 1
}

// GoviSelectionEditRange writes the buffer caret range [a, b) covering the
// current screen selection's text, for cut/paste-over/delete/replace-on-type.
// Returns 1 only when the selection is a reading-order (linear) selection lying
// entirely on buffer rows; 0 for block selections or any selection touching a
// non-buffer cell (status, ~, gutter endpoint, overlay, ex transcript).
//
//export GoviSelectionEditRange
func GoviSelectionEditRange(h C.longlong, l1 *C.longlong, c1 *C.int, l2 *C.longlong, c2 *C.int) C.int {
	in := get(h)
	if in == nil || !in.screenSelActive || !in.screenSelLinear {
		return 0
	}
	sel := grid.ScreenSelection{A: in.screenSelA, B: in.screenSelB}
	var a, b engine.Pos
	var ok bool
	in.eng.WithView(func(v engine.View) {
		a, b, ok = grid.SelectionEditRange(v, in.rows, in.cols, sel)
	})
	if !ok {
		return 0
	}
	*l1, *c1 = C.longlong(a.Line), C.int(a.Col)
	*l2, *c2 = C.longlong(b.Line), C.int(b.Col)
	return 1
}

// GoviScroll scrolls the viewport by delta lines (positive = toward the end of
// the file) for wheel/trackpad scrolling, without moving the cursor.
//
//export GoviScroll
func GoviScroll(h C.longlong, delta C.int) {
	if in := get(h); in != nil {
		in.eng.ScrollLines(int(delta))
	}
}

// GoviMoveCursor positions the cursor caret (click-to-position).
//
//export GoviMoveCursor
func GoviMoveCursor(h C.longlong, line C.longlong, col C.int) {
	if in := get(h); in != nil {
		in.eng.MoveCursorTo(int64(line), int(col))
	}
}

// GoviRangeText returns the text in the caret range [a, b) (malloc'd; caller
// frees). Backs copy/cut.
//
//export GoviRangeText
func GoviRangeText(h C.longlong, l1 C.longlong, c1 C.int, l2 C.longlong, c2 C.int) *C.char {
	if in := get(h); in != nil {
		return C.CString(in.eng.RangeText(pos(l1, c1), pos(l2, c2)))
	}
	return C.CString("")
}

// GoviDeleteRange deletes the caret range [a, b). Backs cut and
// delete-over-selection.
//
//export GoviDeleteRange
func GoviDeleteRange(h C.longlong, l1 C.longlong, c1 C.int, l2 C.longlong, c2 C.int) {
	if in := get(h); in != nil {
		in.eng.DeleteRange(pos(l1, c1), pos(l2, c2))
	}
}

// GoviReplaceType deletes [a, b) and enters insert mode with text as the first
// run (GUI replace-on-type).
//
//export GoviReplaceType
func GoviReplaceType(h C.longlong, l1 C.longlong, c1 C.int, l2 C.longlong, c2 C.int, text *C.char) {
	if in := get(h); in != nil {
		in.eng.ReplaceSelectionType(pos(l1, c1), pos(l2, c2), C.GoString(text))
	}
}

// GoviReplaceText deletes [a, b) and inserts text in command mode (GUI paste
// over a selection).
//
//export GoviReplaceText
func GoviReplaceText(h C.longlong, l1 C.longlong, c1 C.int, l2 C.longlong, c2 C.int, text *C.char) {
	if in := get(h); in != nil {
		in.eng.ReplaceSelectionText(pos(l1, c1), pos(l2, c2), C.GoString(text))
	}
}

// GoviEndPos writes the caret at the very end of the buffer (last line, past
// its last rune) into *line/*col. Backs Select All.
//
//export GoviEndPos
func GoviEndPos(h C.longlong, line *C.longlong, col *C.int) {
	in := get(h)
	if in == nil {
		return
	}
	in.eng.WithView(func(v engine.View) {
		last := v.LineCount()
		*line, *col = C.longlong(last), C.int(len(v.Line(last).Text))
	})
}

// GoviExActive reports whether the editor is in line-oriented ex (Q) mode, in
// which the window shows a scrolling transcript rather than the buffer.
//
//export GoviExActive
func GoviExActive(h C.longlong) C.int {
	in := get(h)
	return boolToC(in != nil && in.eng.ExActive())
}

// GoviOverlayActive reports whether command output (:!, :viusage, etc.) is shown
// as a full-window overlay.
//
//export GoviOverlayActive
func GoviOverlayActive(h C.longlong) C.int {
	in := get(h)
	if in == nil {
		return 0
	}
	var active bool
	in.eng.WithView(func(v engine.View) {
		active = v.PendingOutput() != nil
	})
	return boolToC(active)
}

// GoviTopLine returns the first visible buffer line (the viewport top).
//
//export GoviTopLine
func GoviTopLine(h C.longlong) C.longlong {
	in := get(h)
	if in == nil {
		return 1
	}
	var top int64 = 1
	in.eng.WithView(func(v engine.View) { top = v.Viewport().Top })
	return C.longlong(top)
}

// GoviLineCount returns the number of buffer lines.
//
//export GoviLineCount
func GoviLineCount(h C.longlong) C.longlong {
	in := get(h)
	if in == nil {
		return 0
	}
	var n int64
	in.eng.WithView(func(v engine.View) { n = v.LineCount() })
	return C.longlong(n)
}

// GoviLineText returns the text of buffer line `line` (malloc'd; caller frees).
// Backs spell checking, which works on whole logical lines.
//
//export GoviLineText
func GoviLineText(h C.longlong, line C.longlong) *C.char {
	in := get(h)
	if in == nil {
		return C.CString("")
	}
	out := ""
	in.eng.WithView(func(v engine.View) {
		if line >= 1 && int64(line) <= v.LineCount() {
			out = string(v.Line(int64(line)).Text)
		}
	})
	return C.CString(out)
}

// GoviPosToCell maps a buffer caret (line, col) to its screen cell, writing the
// cell into *x/*y and 1/0 into *visible. The inverse of GoviCellToPos; backs
// anchoring spelling underlines to buffer positions.
//
//export GoviPosToCell
func GoviPosToCell(h C.longlong, line C.longlong, col C.int, x *C.int, y *C.int, visible *C.int) {
	in := get(h)
	if in == nil {
		return
	}
	in.eng.WithView(func(v engine.View) {
		cx, cy, ok := grid.CellOf(v, in.rows, in.cols, engine.Pos{Line: int64(line), Col: int(col)})
		*x, *y = C.int(cx), C.int(cy)
		*visible = boolToC(ok)
	})
}

// GoviCursorX / GoviCursorY / GoviCursorVisible expose the cursor cell.
//
//export GoviCursorX
func GoviCursorX(h C.longlong) C.int {
	if in := get(h); in != nil && in.gridOK {
		return C.int(in.gr.CursorX)
	}
	return 0
}

//export GoviCursorY
func GoviCursorY(h C.longlong) C.int {
	if in := get(h); in != nil && in.gridOK {
		return C.int(in.gr.CursorY)
	}
	return 0
}

//export GoviCursorVisible
func GoviCursorVisible(h C.longlong) C.int {
	in := get(h)
	return boolToC(in != nil && in.gridOK && in.gr.CursorVisible)
}

// GoviFree frees a string returned by this library.
//
//export GoviFree
func GoviFree(p *C.char) { C.free(unsafe.Pointer(p)) }

func pos(l C.longlong, c C.int) engine.Pos { return engine.Pos{Line: int64(l), Col: int(c)} }

func boolToC(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

func cString(p *C.char) string {
	if p == nil {
		return ""
	}
	return C.GoString(p)
}
