package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"govi/engine/buffer"
	"govi/engine/mark"
	"govi/engine/register"
	"govi/engine/undo"
)

// Options configures a new Engine. It is intentionally empty for now and will
// grow (initial mode, secure mode, option overrides) in later phases.
type Options struct{}

// Engine is the embeddable editor core. A host constructs one with a Frontend,
// opens a file, sets the viewport geometry, and feeds input events; the engine
// drives the Frontend via Render. It has no terminal or GUI dependency.
type Engine struct {
	fe  Frontend
	scr *screen // the active screen; always == screens[cur]
	vi  *vimode

	// screens are the displayed editor screens (split windows), ordered
	// top-to-bottom then left-to-right (nvi's WIN scrq, kept sorted by roff,coff).
	// A single unsplit session has exactly one. cur is the index of the active
	// screen; scr is kept pointing at screens[cur].
	screens []*screen
	cur     int

	// bg holds backgrounded screens (nvi's GS hidden queue gp->hq): screens
	// removed from the display by :bg or a :fg swap, kept editable and
	// restorable by :fg/:Fg.
	bg []*screen

	// cclStore/cclLog hold the colon command-line history collected while the
	// cedit option is set (nvi wp->ccl_sp's buffer, vi/v_ex.c). Created lazily,
	// shared by every comedit window, never discarded -- the history and any
	// in-window edits to it survive window close.
	cclStore buffer.LineStore
	cclLog   *undo.Log

	// termH/termCols are the full terminal geometry last given to Resize: termH
	// is the total number of display rows (text rows of all screens plus one
	// status row per screen), termCols the width.
	termH    int
	termCols int

	mapPending []rune // runes accumulating toward a possible map LHS

	// cscopes are the running cscope subprocess connections (nvi exp->cscq),
	// shared across all screens; cscopeInit records whether the CSCOPE_DIRS
	// environment variable has been consulted yet.
	cscopes    []*cscopeConn
	cscopeInit bool

	lockF   *os.File // dedicated fd holding the advisory edit lock (nvi ep->fd)
	quit    bool
	exitMsg string // set when a signal caused the exit; printed by the host

	// redrawRequested is set by ^L/^R and consumed by the next diff() so the
	// frontend forces a full physical repaint (nvi v_redraw). This recovers the
	// display after another program has written to the tty; ordinary paints only
	// push changed cells and cannot fix corruption the editor did not cause.
	redrawRequested bool

	// interrupted / interruptCh carry the user's ^C (SIGINT) request across the
	// frontend->engine boundary. They are the one piece of engine state a frontend
	// may touch concurrently with the goroutine driving Input (see Interrupt): a
	// long-running command must be able to observe a ^C that lands while it is
	// still executing, and the engine is otherwise single-threaded. CPU-bound
	// loops poll interrupted; blocking operations select on interruptCh.
	interrupted atomic.Bool
	interruptCh chan struct{} // buffered(1)

	wordBoundary WordBoundaryFunc // double-click word selection (GUI hosts)

	recoverPath  string    // this session's recovery file, "" if none yet
	recoverSync  time.Time // last time the recovery file was written
	recoverDirty bool      // changes exist that the recovery file lacks
	recoverKeep  bool      // :preserve ran; keep the file past save/exit (nvi RCV_PRESERVE)

	// Line-oriented ex (Q) output: while exLineMode is set, command output is
	// collected into exOut for a line-at-a-time host instead of the transcript.
	exLineMode bool
	exOut      []string
	exSilent   bool // batch script (nvi -s / SC_EX_SILENT): suppress autoprint etc.

	startup   bool // true while reading EXINIT / exrc startup information
	launchCtx LaunchContext
	cwd       string // per-instance working directory (:cd, relative :read/:write)
}

// New returns an Engine that renders through fe. Call Open and Resize before
// feeding input.
func New(fe Frontend, _ Options) *Engine {
	e := &Engine{
		fe:           fe,
		wordBoundary: DefaultWordBoundary,
		interruptCh:  make(chan struct{}, 1),
	}
	e.setBuffer(buffer.NewMem(), "")
	return e
}

// setBuffer creates the initial screen with default editor settings. Used once
// at New.
func (e *Engine) setBuffer(store buffer.LineStore, name string) {
	e.scr = &screen{
		store:         store,
		log:           undo.New(store),
		marks:         mark.New(),
		regs:          register.New(),
		name:          name,
		cursor:        Pos{Line: 1, Col: 0},
		top:           1,
		mode:          ModeCommand,
		showModeLabel: "Command",
		opts:          defaultOptions(),
		maps:          newMapTable(),
	}
	e.screens = []*screen{e.scr}
	e.cur = 0
	e.vi = newVimode()
}

// replaceBuffer swaps the file being edited while preserving editor-global
// state (options, maps, registers) and geometry, as vi does across :e / :n.
func (e *Engine) replaceBuffer(store buffer.LineStore, name string) {
	e.removeRecovery() // discard the previous file's recovery state
	s := e.scr
	if s.file != nil {
		s.file.Close()
		s.file = nil
	}
	s.store = store
	s.log = undo.New(store)
	s.dlReady = false // new buffer + fresh log (gen restarts at 0): drop the memo
	s.marks = mark.New()
	s.name = name
	s.nameChanged = false
	s.tempFile = false
	s.cursor = Pos{Line: 1, Col: 0}
	s.top = 1
	s.mode = ModeCommand
	s.showModeLabel = "Command"
	s.modified = false
	s.colon = nil
	e.vi = newVimode()
}

// Open loads path into the active buffer. A missing file starts an empty,
// unsaved buffer named path (vi's "new file"). Large files are paged from disk
// rather than read whole.
func (e *Engine) Open(path string) error {
	// Keep the file name exactly as given (relative or absolute), like nvi's
	// FR_NAME: it is what the status line shows and, crucially, what later file
	// operations re-resolve against the *current* directory. Only the filesystem
	// access below uses the resolved path. Storing the absolute path here would
	// make a :w after a :cd write to the original directory instead of the new one.
	name := strings.TrimSpace(path)
	resolved := e.canonicalPath(name)
	// Remember the file we are leaving as the alternate file.
	if e.scr != nil && e.scr.name != "" && e.scr.name != name {
		e.scr.altFile = e.scr.name
	}
	store, fh, err := buffer.NewPagedFile(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			e.replaceBuffer(buffer.NewMem(), name)
			if len(e.scr.argv) > 1 {
				e.scr.showFileCount = true
			}
			e.scr.msg = fmt.Sprintf("%s: new file: line %d", e.scr.name, e.scr.cursor.Line)
			e.scr.msgKind = MsgInfo
			return nil
		}
		return err
	}
	e.releaseLock()              // drop any lock held on the previous file
	e.replaceBuffer(store, name) // closes the previous per-screen file handle
	e.scr.file = fh
	if len(e.scr.argv) > 1 {
		e.scr.showFileCount = true
	}
	chars := int64(0)
	if fi, err := os.Stat(resolved); err == nil {
		chars = fi.Size()
	}
	e.scr.msg = fmt.Sprintf("%s: %d lines, %d characters", e.scr.name, store.Lines(), chars)
	e.scr.msgKind = MsgInfo
	// Advisory file lock (nvi `lock` option / common/exf.c file_lock): take a
	// non-blocking exclusive lock for the edit session on a dedicated fd. If
	// another process holds it, open read-only and warn so the readonly
	// write-guard protects the first session's edits. The lock is keyed on the
	// file, so it interoperates with nvi.
	if e.scr.opts.Bool("lock") && e.acquireLock(resolved) == lockUnavail {
		e.scr.opts.b["readonly"] = true
		e.scr.msg = fmt.Sprintf("%s: already locked, session is read-only", e.scr.name)
		e.scr.msgKind = MsgInfo
	}
	if e.hasRecovery(resolved) {
		e.scr.msg = fmt.Sprintf("%s: recovery file exists; use :recover to restore it", e.scr.name)
		e.scr.msgKind = MsgInfo
	}
	return nil
}

// OpenArgs sets the file argument list and opens the first file. With no
// arguments it leaves the initial empty buffer.
func (e *Engine) OpenArgs(args []string) error {
	e.scr.argv = args
	e.scr.argIdx = 0
	if len(args) == 0 {
		return nil
	}
	return e.Open(args[0])
}

// Close releases any file handles held by the engine's screens.
func (e *Engine) Close() error {
	e.releaseLock()
	e.cscopeReset() // terminate any running cscope subprocesses
	var firstErr error
	for _, s := range e.screens {
		if s.file != nil {
			if err := s.file.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
			s.file = nil
		}
	}
	return firstErr
}

// acquireLock takes the advisory edit lock on resolved using a dedicated fd
// (nvi keeps a separate lock fd, ep->fd). On success the fd is held in e.lockF
// until released; on LOCK_UNAVAIL/LOCK_FAILED the fd is closed and the result is
// returned so the caller can fall back to read-only.
func (e *Engine) acquireLock(resolved string) lockResult {
	e.releaseLock()
	f, err := os.Open(resolved)
	if err != nil {
		return lockFailed
	}
	if r := tryFileLock(f); r == lockSuccess {
		e.lockF = f
		return r
	} else {
		f.Close()
		return r
	}
}

// releaseLock drops the advisory edit lock (closing its fd releases the flock).
func (e *Engine) releaseLock() {
	if e.lockF != nil {
		e.lockF.Close()
		e.lockF = nil
	}
}

// relockAfterWrite re-takes the advisory lock after a temp-file+rename write.
// The rename replaces the inode the lock was held on with a new one, so without
// this the lock would silently evaporate on the first :w (nvi writes in place
// and keeps its lock). Best-effort: only when we already hold the lock and are
// writing the buffer's own file; a failure leaves the session unlocked rather
// than erroring a successful write.
func (e *Engine) relockAfterWrite(resolved string) {
	if e.lockF == nil {
		return
	}
	e.acquireLock(resolved)
}

// ShouldQuit reports whether a quit command has been issued; the host event
// loop should stop feeding input and tear down once this is true.
func (e *Engine) ShouldQuit() bool { return e.quit }

// ClearQuit resets the quit flag after the host cancels a close (:q with unsaved
// changes, or a dismissed save sheet).
func (e *Engine) ClearQuit() { e.quit = false }

// RunEx executes an ex command programmatically (e.g. from a GUI menu or host
// configuration), as if typed on the colon line without the leading ':'.
func (e *Engine) RunEx(cmd string) error { return e.exExecute(cmd) }

// RunStartupEx runs a command-line startup ex command (-c/+cmd, -t tag) after
// the initial file is opened, reporting failure on the status line the way
// nvi runs c_option, rather than returning the error to the host.
func (e *Engine) RunStartupEx(cmd string) { e.runColon(cmd) }

// curView builds the read-only View over the editor state: a plain single-screen
// view when unsplit, or a renderView enumerating every screen when split. All
// Render calls and WithView use it.
func (e *Engine) curView() View {
	if len(e.screens) <= 1 {
		return view{s: e.scr}
	}
	return renderView{view: view{s: e.scr}, all: e.screens, cur: e.cur}
}

// WithView calls fn with the read-only View of the current editor state. A host
// that pulls state on demand (rather than reacting to Render) uses this to lay
// out a frame between inputs. The View is valid only for the duration of the
// call; the engine is quiescent while fn runs (it must not feed input from fn).
func (e *Engine) WithView(fn func(View)) { fn(e.curView()) }

// MapPending reports whether input is buffered awaiting more keys to resolve a
// possible map. A host can use this to arm a key-timeout (sending a
// TimeoutEvent) so an ambiguous map prefix does not hang.
func (e *Engine) MapPending() bool { return len(e.mapPending) > 0 }

// MatchPending reports whether a showmatch bracket flash is active, so the host
// can arm a timer (matchtime tenths of a second) to clear it.
func (e *Engine) MatchPending() bool { return e.scr.matchActive }

// MatchTime returns the showmatch flash duration in tenths of a second.
func (e *Engine) MatchTime() int { return e.scr.opts.Int("matchtime") }

// RefreshInterval returns the minimum interval between screen repaints during
// fast terminal input (govi extension). Zero disables throttling.
func (e *Engine) RefreshInterval() time.Duration {
	d, err := parseRefresh(e.scr.opts.Str("refresh"))
	if err != nil {
		return defaultRefresh
	}
	return d
}

// Resize sets the viewport geometry and repaints fully. rows is the number of
// text rows for a single full-screen session (terminal height minus the one
// status row); cols the width. The engine tracks the full terminal height
// (rows+1) so it can lay out split screens, each of which carries its own status
// row.
func (e *Engine) Resize(rows, cols int) {
	e.termH = rows + 1
	e.termCols = cols
	if len(e.screens) <= 1 {
		s := e.scr
		s.roff, s.coff = 0, 0
		s.rows = rows
		s.cols = cols
		s.applyWindowOption()
		s.defScroll = 0 // re-derive the half-page size from the new height
		// Keep the columns/lines options in step with the terminal geometry.
		s.opts.i["columns"] = cols
		s.opts.i["lines"] = rows + 1
		s.clampCursor()
		s.scrollToCursor()
	} else {
		e.relayout()
	}
	e.fe.Render(e.curView(), ChangeSet{Full: true})
}

// relayout redistributes the full terminal height across the currently displayed
// (horizontally stacked) screens, in proportion to their previous heights, and
// gives every screen the full terminal width. Exact nvi resize parity is a later
// refinement; this keeps a split layout valid across a terminal resize.
func (e *Engine) relayout() {
	n := len(e.screens)
	tot := 0
	for _, s := range e.screens {
		tot += s.rows + 1
	}
	if tot <= 0 {
		tot = 1
	}
	roff := 0
	for i, s := range e.screens {
		var disp int
		if i == n-1 {
			disp = e.termH - roff // remainder to the last screen
		} else {
			disp = (s.rows + 1) * e.termH / tot
			if disp < 2 {
				disp = 2
			}
		}
		if disp < 2 {
			disp = 2
		}
		s.roff = roff
		s.coff = 0
		s.cols = e.termCols
		s.rows = disp - 1
		s.applyWindowOption()
		s.defScroll = 0
		s.opts.i["columns"] = e.termCols
		s.opts.i["lines"] = e.termH
		s.clampCursor()
		s.scrollToCursor()
		roff += disp
	}
}

// snapshot of the presentation-relevant fields, used to compute a ChangeSet.
type snap struct {
	cursor Pos
	top    int64
	mode   Mode
	msg    string
	kind   MessageKind
}

func (e *Engine) snap() snap {
	s := e.scr
	return snap{cursor: s.cursor, top: s.top, mode: s.mode, msg: s.msg, kind: s.msgKind}
}

// Input feeds one event to the engine and repaints as needed.
func (e *Engine) Input(ev Event) {
	// Discard a leftover interrupt when the command FINISHES, not on entry
	// (nvi's CLR_INTERRUPT). The frontend records a ^C out of band
	// (Engine.Interrupt) on a separate goroutine and can poll it ahead of the
	// main loop (see frontend/tcell forwardInterrupts), so a ^C typed to abort
	// *this* command may already be set before Input begins. Clearing on entry --
	// as an earlier version did -- swallowed such a ^C and let a long search /
	// :s / :g / :! run to completion with no feedback. Deferring the clear lets
	// this command's interruptible loops observe the flag first, then drops only
	// an interrupt left over once the command is done, so it cannot leak into the
	// next command.
	defer e.clearInterrupt()

	before := e.snap()

	// A showmatch bracket flash is cleared by the next real key (which is then
	// processed normally, so the cursor returns and the keystroke takes effect).
	if e.scr.matchActive {
		switch ev.(type) {
		case KeyEvent, StringEvent:
			e.scr.matchActive = false
		}
	}

	// A pending output overlay (e.g. :set all, :exusage) is paged by keypress.
	if e.handlePendingOutput(ev) {
		return
	}

	switch v := ev.(type) {
	case ResizeEvent:
		e.Resize(v.Rows, v.Cols)
		return
	case StringEvent:
		// Pasted/literal text is dispatched directly, bypassing map expansion.
		for _, r := range v.Text {
			e.dispatchRune(r)
		}
	case KeyEvent:
		e.handleKeyEvent(v)
	case InterruptEvent:
		e.flushMapPending()
		e.interrupt()
	case TimeoutEvent:
		if e.scr.matchActive {
			e.scr.matchActive = false // showmatch flash elapsed
		} else {
			e.mapTimeout()
		}
	default:
		// SuspendEvent: nothing to do.
	}

	e.scr.clampCursor()
	e.scr.scrollToCursor()
	e.fe.Render(e.curView(), e.diff(before))
}

func (e *Engine) diff(b snap) ChangeSet {
	a := e.snap()
	sync := e.redrawRequested
	e.redrawRequested = false
	return ChangeSet{
		Sync:           sync,
		CursorMoved:    a.cursor != b.cursor,
		Scrolled:       a.top != b.top,
		ModeChanged:    a.mode != b.mode,
		MessageChanged: a.msg != b.msg || a.kind != b.kind,
	}
}

func (e *Engine) interrupt() {
	s := e.scr
	if s.subConfirm != nil {
		e.finishSubstConfirm() // an interrupt at the confirm prompt quits (nvi)
	}
	if s.mode == ModeExColon {
		s.mode = ModeCommand
		s.colon = nil
		s.resetColonEdit()
		s.filterL1, s.filterL2 = 0, 0
	}
	e.fe.Bell()
}

// dispatchKey routes a single keypress to the current mode's handler. Command,
// insert, and replace modes are handled by the vi state machine (vi.go); the
// command line (':' ex commands and '/' '?' searches) is handled here. Map
// expansion happens upstream in handleKeyEvent.
func (e *Engine) dispatchKey(ev KeyEvent) {
	// While an ex a/i/c command is collecting input, every key feeds the
	// collector regardless of the visual mode it was started from.
	if e.scr.exInput != nil {
		e.exInputKey(ev)
		return
	}
	// While a :s///c confirmation is pending, every key answers the prompt.
	if e.scr.subConfirm != nil {
		e.substConfirmKey(ev)
		return
	}
	switch e.scr.mode {
	case ModeExColon:
		e.cmdlineKey(ev)
	case ModeExText:
		e.exModeKey(ev)
	default:
		e.vi.key(e, ev)
	}
}

// enterCmdline starts command-line input with the given prompt prefix.
func (e *Engine) enterCmdline(prefix rune) {
	e.scr.mode = ModeExColon
	e.scr.cmdPrefix = prefix
	e.scr.colon = nil
	e.scr.resetColonEdit()
	e.scr.filterL1, e.scr.filterL2 = 0, 0
}

func (e *Engine) cmdlineKey(ev KeyEvent) {
	s := e.scr
	e.colonEditKey(ev, colonEditOpts{
		leaveOnEmptyBackspace: true,
		onEnter: func(line string) {
			prefix := s.cmdPrefix
			s.mode = ModeCommand
			s.colon = nil
			e.runCmdline(prefix, line)
		},
		onEscape: func() {
			s.mode = ModeCommand
			s.colon = nil
			s.filterL1, s.filterL2 = 0, 0
			e.vi.searchOp = 0 // cancel a deferred operator-search (d/pat aborted)
		},
	})
}

// runCmdline dispatches a completed command line by its prompt prefix.
func (e *Engine) runCmdline(prefix rune, line string) {
	switch prefix {
	case '/':
		if err := e.runSearchLine(line, searchFwd); err != nil {
			e.scr.msg, e.scr.msgKind = err.Error(), MsgError
		}
	case '?':
		if err := e.runSearchLine(line, searchBack); err != nil {
			e.scr.msg, e.scr.msgKind = err.Error(), MsgError
		}
	case '!':
		l1, l2 := e.scr.filterL1, e.scr.filterL2
		e.scr.filterL1, e.scr.filterL2 = 0, 0
		cmd := strings.TrimSpace(line)
		if cmd == "" {
			e.scr.msg, e.scr.msgKind = "Usage: [range]!command", MsgError
			return
		}
		if err := e.filterLines(l1, l2, cmd); err != nil {
			e.scr.msg, e.scr.msgKind = err.Error(), MsgError
		}
	default:
		// While cedit is set, log the command to the colon history before
		// running it (nvi v_ex calls v_ecl_log before pushing the command, so
		// failing commands are kept too). Only this vi ':' prompt path logs:
		// startup files, ex line mode, :g bodies, and direct exExecute calls
		// never come through here, matching nvi's placement in v_ex.
		if e.scr.opts.Str("cedit") != "" {
			e.ceditLog(line)
		}
		e.runColon(strings.TrimSpace(line))
	}
}

// writeRange writes buffer lines [l1,l2] to path. When appendMode is true the
// lines are appended to path (nvi ":[range]w >> file", LF_APPEND) and path is
// created if absent; otherwise path is replaced atomically via temp+rename.
// Used for partial-range and appended writes; the whole-file write goes through
// writeFile.
func (e *Engine) writeRange(path string, l1, l2 int64, appendMode bool) (lines, bytes int64, err error) {
	s := e.scr
	// The range is addressed against lineCount(), which reports a phantom blank
	// line for an emptied store; clamp to the real line count so writing an
	// emptied buffer produces 0 bytes, not a spurious "\n" (like writeFile).
	if n := s.store.Lines(); l2 > n {
		l2 = n
	}
	var f *os.File
	var tmpName string
	if appendMode {
		f, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			return 0, 0, err
		}
	} else {
		f, err = os.CreateTemp(filepath.Dir(path), ".govi-*")
		if err != nil {
			return 0, 0, err
		}
		tmpName = f.Name()
	}
	var bytesOut, nOut int64
	for i := l1; i <= l2; i++ {
		if e.Interrupted() {
			f.Close()
			if tmpName != "" {
				os.Remove(tmpName)
			}
			return 0, 0, errInterrupted
		}
		line, _ := s.store.Get(i)
		b := []byte(string(line))
		f.Write(b)
		f.Write([]byte{'\n'})
		bytesOut += int64(len(b)) + 1
		nOut++
	}
	if err := f.Close(); err != nil {
		if tmpName != "" {
			os.Remove(tmpName)
		}
		return 0, 0, err
	}
	if !appendMode {
		if info, err := os.Stat(path); err == nil {
			mode := info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid)
			if err := os.Chmod(tmpName, mode); err != nil {
				os.Remove(tmpName)
				return 0, 0, err
			}
		}
		if err := os.Rename(tmpName, path); err != nil {
			os.Remove(tmpName)
			return 0, 0, err
		}
	}
	return nOut, bytesOut, nil
}

// writeFile saves the buffer to path, atomically via a temp file + rename, and
// returns the line and byte counts written.
func (e *Engine) writeFile(path string) (lines, bytes int64, err error) {
	s := e.scr
	tmp, err := os.CreateTemp(filepath.Dir(path), ".govi-*")
	if err != nil {
		return 0, 0, err
	}
	tmpName := tmp.Name()
	n := s.store.Lines()
	var bytesOut int64
	for i := int64(1); i <= n; i++ {
		if e.Interrupted() {
			// ^C mid-write: discard the partial temp file so the original is left
			// untouched (nvi leaves the file unmodified on an interrupted write).
			tmp.Close()
			os.Remove(tmpName)
			return 0, 0, errInterrupted
		}
		line, _ := s.store.Get(i)
		b := []byte(string(line))
		tmp.Write(b)
		tmp.Write([]byte{'\n'})
		bytesOut += int64(len(b)) + 1
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return 0, 0, err
	}
	// Atomic rename replaces the directory entry with the temp inode, so copy
	// the prior file's mode onto the temp (nvi preserves mode by truncating in
	// place via open; we use temp+rename for atomicity).
	if info, err := os.Stat(path); err == nil {
		mode := info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid)
		if err := os.Chmod(tmpName, mode); err != nil {
			os.Remove(tmpName)
			return 0, 0, err
		}
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return 0, 0, err
	}
	return n, bytesOut, nil
}
