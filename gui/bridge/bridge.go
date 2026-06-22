// Command bridge builds the govi editor engine into a C archive (libgovi.a)
// that a native host application links against. It is the embedding proof: the
// macOS AppKit app in gui/macos drives this library and renders the resulting
// character grid in a custom NSView, with the engine running in-process — no
// terminal, and no exec of nvi.
//
// The host calls all functions from a single thread (its UI thread): the engine
// is single-threaded by design. The flow each frame is:
//
//  1. GoviStart once to create the engine and open a file.
//  2. GoviResize whenever the window/cell geometry changes.
//  3. Feed input with GoviKeyRune / GoviKeySpecial / GoviText / GoviInterrupt.
//  4. GoviCompose(rows, cols), then read GoviRowText / GoviCursor* to redraw.
//  5. Check GoviTakeBell, GoviShouldQuit, and the *Pending timers to schedule
//     a follow-up GoviFireTimeout / GoviSyncRecovery.
//
// Strings returned by GoviRowText and GoviTitle are heap-allocated with C
// malloc; the caller must free() them.
package main

// #include <stdlib.h>
import "C"

import (
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
func (h *host) Bell()                                     { h.bellPending = true }
func (h *host) SetTitle(s string)                         { h.title = s }

var (
	eng    *engine.Engine
	fe     *host
	curGr  grid.Grid
	gridOK bool
)

func main() {} // required for c-archive builds

// GoviStart creates the engine and opens path (empty path starts a scratch
// buffer). Returns 0 on success, 1 on error.
//
//export GoviStart
func GoviStart(path *C.char) C.int {
	fe = &host{}
	eng = engine.New(fe, engine.Options{})
	p := C.GoString(path)
	if p != "" {
		if err := eng.Open(p); err != nil {
			return 1
		}
	}
	return 0
}

// GoviResize sets the viewport geometry. rows is the full window height in
// cells including the status row; cols is the width in cells.
//
//export GoviResize
func GoviResize(rows, cols C.int) {
	if eng == nil {
		return
	}
	// The engine reserves the status row itself, so it is given text rows.
	tr := int(rows) - 1
	if tr < 1 {
		tr = 1
	}
	eng.Resize(tr, int(cols))
}

// GoviKeyRune feeds a typed character with optional modifiers (bit flags
// matching engine.Mod: 1=Ctrl, 2=Alt, 4=Shift).
//
//export GoviKeyRune
func GoviKeyRune(r C.int, mods C.int) {
	if eng == nil {
		return
	}
	eng.Input(engine.KeyEvent{Rune: rune(r), Mods: engine.Mod(mods)})
}

// GoviKeySpecial feeds a non-text key. key matches the engine.SpecialKey
// enumeration (1=Escape, 2=Enter, 3=Tab, 4=Backspace, 5=Delete, 6=Up, 7=Down,
// 8=Left, 9=Right, 10=Home, 11=End, 12=PageUp, 13=PageDown, 14=Insert).
//
//export GoviKeySpecial
func GoviKeySpecial(key C.int, mods C.int) {
	if eng == nil {
		return
	}
	eng.Input(engine.KeyEvent{Key: engine.SpecialKey(key), Mods: engine.Mod(mods)})
}

// GoviText feeds a run of literal text (e.g. a paste), bypassing map expansion.
//
//export GoviText
func GoviText(s *C.char) {
	if eng == nil {
		return
	}
	eng.Input(engine.StringEvent{Text: C.GoString(s)})
}

// GoviInterrupt delivers the interrupt event (cancel current command/input).
//
//export GoviInterrupt
func GoviInterrupt() {
	if eng != nil {
		eng.Input(engine.InterruptEvent{})
	}
}

// GoviFireTimeout delivers a timeout event, resolving an ambiguous map prefix
// or clearing a showmatch flash.
//
//export GoviFireTimeout
func GoviFireTimeout() {
	if eng != nil {
		eng.Input(engine.TimeoutEvent{})
	}
}

// GoviShouldQuit reports whether a quit command was issued (host should exit).
//
//export GoviShouldQuit
func GoviShouldQuit() C.int {
	if eng != nil && eng.ShouldQuit() {
		return 1
	}
	return 0
}

// GoviMapPending reports whether input is buffered awaiting more keys.
//
//export GoviMapPending
func GoviMapPending() C.int { return boolToC(eng != nil && eng.MapPending()) }

// GoviMatchPending reports whether a showmatch bracket flash is active.
//
//export GoviMatchPending
func GoviMatchPending() C.int { return boolToC(eng != nil && eng.MatchPending()) }

// GoviMatchTimeMS returns the showmatch flash duration in milliseconds.
//
//export GoviMatchTimeMS
func GoviMatchTimeMS() C.int {
	if eng == nil {
		return 0
	}
	return C.int(eng.MatchTime() * 100)
}

// GoviNeedsRecoverySync reports whether the recovery file should be flushed.
//
//export GoviNeedsRecoverySync
func GoviNeedsRecoverySync() C.int { return boolToC(eng != nil && eng.NeedsRecoverySync()) }

// GoviSyncRecovery flushes pending changes to the recovery file.
//
//export GoviSyncRecovery
func GoviSyncRecovery() {
	if eng != nil {
		eng.SyncRecovery()
	}
}

// GoviTakeBell returns 1 and clears the flag if a bell occurred since the last
// call, so the host can play NSBeep.
//
//export GoviTakeBell
func GoviTakeBell() C.int {
	if fe == nil || !fe.bellPending {
		return 0
	}
	fe.bellPending = false
	return 1
}

// GoviTitle returns the desired window title (malloc'd; caller frees).
//
//export GoviTitle
func GoviTitle() *C.char {
	if fe == nil {
		return C.CString("")
	}
	return C.CString(fe.title)
}

// GoviCompose lays out the current view into a rows x cols grid cached for the
// row/cursor accessors.
//
//export GoviCompose
func GoviCompose(rows, cols C.int) {
	if eng == nil {
		gridOK = false
		return
	}
	eng.WithView(func(v engine.View) {
		curGr = grid.Compose(v, int(rows), int(cols))
	})
	gridOK = true
}

// GoviRows / GoviCols return the cached grid dimensions.
//
//export GoviRows
func GoviRows() C.int {
	if !gridOK {
		return 0
	}
	return C.int(curGr.Rows)
}

//export GoviCols
func GoviCols() C.int {
	if !gridOK {
		return 0
	}
	return C.int(curGr.Cols)
}

// GoviRowText returns row y of the cached grid as a UTF-8 string with blank
// cells rendered as spaces and trailing blanks trimmed (malloc'd; caller frees).
//
//export GoviRowText
func GoviRowText(y C.int) *C.char {
	if !gridOK || int(y) < 0 || int(y) >= curGr.Rows {
		return C.CString("")
	}
	runes := make([]rune, curGr.Cols)
	last := -1
	for x := 0; x < curGr.Cols; x++ {
		r := curGr.At(x, int(y)).Rune
		if r == 0 {
			r = ' '
		} else {
			last = x
		}
		runes[x] = r
	}
	return C.CString(string(runes[:last+1]))
}

// GoviCursorX / GoviCursorY / GoviCursorVisible expose the cursor cell.
//
//export GoviCursorX
func GoviCursorX() C.int {
	if !gridOK {
		return 0
	}
	return C.int(curGr.CursorX)
}

//export GoviCursorY
func GoviCursorY() C.int {
	if !gridOK {
		return 0
	}
	return C.int(curGr.CursorY)
}

//export GoviCursorVisible
func GoviCursorVisible() C.int { return boolToC(gridOK && curGr.CursorVisible) }

// GoviFree frees a string returned by this library (for hosts without a direct
// free()).
//
//export GoviFree
func GoviFree(p *C.char) { C.free(unsafe.Pointer(p)) }

func boolToC(b bool) C.int {
	if b {
		return 1
	}
	return 0
}
