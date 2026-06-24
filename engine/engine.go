package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	scr *screen
	vi  *vimode

	mapPending []rune // runes accumulating toward a possible map LHS

	argv          []string // file argument list
	argIdx        int      // index of the current file in argv
	showFileCount bool     // next :f/^G shows "N files to edit" (nvi SC_STATUS_CNT)
	altFile  string   // alternate file (^^ / #), the previously edited file
	tagStack []tagLoc // tag jump stack for ^T

	file *os.File // open handle backing a paged buffer, if any
	quit bool

	wordBoundary WordBoundaryFunc // double-click word selection (GUI hosts)

	recoverPath  string    // this session's recovery file, "" if none yet
	recoverSync  time.Time // last time the recovery file was written
	recoverDirty bool      // changes exist that the recovery file lacks

	// Line-oriented ex (Q) output: while exLineMode is set, command output is
	// collected into exOut for a line-at-a-time host instead of the transcript.
	exLineMode bool
	exOut      []string
}

// New returns an Engine that renders through fe. Call Open and Resize before
// feeding input.
func New(fe Frontend, _ Options) *Engine {
	e := &Engine{fe: fe, wordBoundary: DefaultWordBoundary}
	e.setBuffer(buffer.NewMem(), "")
	return e
}

// setBuffer creates the initial screen with default editor settings. Used once
// at New.
func (e *Engine) setBuffer(store buffer.LineStore, name string) {
	e.scr = &screen{
		store:  store,
		log:    undo.New(store),
		marks:  mark.New(),
		regs:   register.New(),
		name:   name,
		cursor: Pos{Line: 1, Col: 0},
		top:    1,
		mode:   ModeCommand,
		opts:   defaultOptions(),
		maps:   newMapTable(),
	}
	e.vi = newVimode()
}

// replaceBuffer swaps the file being edited while preserving editor-global
// state (options, maps, registers) and geometry, as vi does across :e / :n.
func (e *Engine) replaceBuffer(store buffer.LineStore, name string) {
	e.removeRecovery() // discard the previous file's recovery state
	s := e.scr
	s.store = store
	s.log = undo.New(store)
	s.marks = mark.New()
	s.name = name
	s.nameChanged = false
	s.cursor = Pos{Line: 1, Col: 0}
	s.top = 1
	s.mode = ModeCommand
	s.modified = false
	s.colon = nil
	e.vi = newVimode()
}

// Open loads path into the active buffer. A missing file starts an empty,
// unsaved buffer named path (vi's "new file"). Large files are paged from disk
// rather than read whole.
func (e *Engine) Open(path string) error {
	// Remember the file we are leaving as the alternate file.
	if e.scr != nil && e.scr.name != "" && e.scr.name != path {
		e.altFile = e.scr.name
	}
	store, fh, err := buffer.NewPagedFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			e.replaceBuffer(buffer.NewMem(), path)
			if len(e.argv) > 1 {
				e.showFileCount = true
			}
			e.scr.msg = fmt.Sprintf("%q: new file", filepath.Base(path))
			e.scr.msgKind = MsgInfo
			return nil
		}
		return err
	}
	if e.file != nil {
		e.file.Close()
	}
	e.file = fh
	e.replaceBuffer(store, path)
	if len(e.argv) > 1 {
		e.showFileCount = true
	}
	e.scr.msg = fmt.Sprintf("%q: %d lines", filepath.Base(path), store.Lines())
	e.scr.msgKind = MsgInfo
	if e.hasRecovery(path) {
		e.scr.msg = fmt.Sprintf("%q: recovery file exists; use :recover to restore it", filepath.Base(path))
		e.scr.msgKind = MsgInfo
	}
	return nil
}

// OpenArgs sets the file argument list and opens the first file. With no
// arguments it leaves the initial empty buffer.
func (e *Engine) OpenArgs(args []string) error {
	e.argv = args
	e.argIdx = 0
	if len(args) == 0 {
		return nil
	}
	return e.Open(args[0])
}

// Close releases any file handle held by the engine.
func (e *Engine) Close() error {
	if e.file != nil {
		err := e.file.Close()
		e.file = nil
		return err
	}
	return nil
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

// WithView calls fn with the read-only View of the current editor state. A host
// that pulls state on demand (rather than reacting to Render) uses this to lay
// out a frame between inputs. The View is valid only for the duration of the
// call; the engine is quiescent while fn runs (it must not feed input from fn).
func (e *Engine) WithView(fn func(View)) { fn(view{e.scr}) }

// MapPending reports whether input is buffered awaiting more keys to resolve a
// possible map. A host can use this to arm a key-timeout (sending a
// TimeoutEvent) so an ambiguous map prefix does not hang.
func (e *Engine) MapPending() bool { return len(e.mapPending) > 0 }

// MatchPending reports whether a showmatch bracket flash is active, so the host
// can arm a timer (matchtime tenths of a second) to clear it.
func (e *Engine) MatchPending() bool { return e.scr.matchActive }

// MatchTime returns the showmatch flash duration in tenths of a second.
func (e *Engine) MatchTime() int { return e.scr.opts.Int("matchtime") }

// Resize sets the viewport geometry (text rows and columns) and repaints fully.
func (e *Engine) Resize(rows, cols int) {
	e.scr.rows = rows
	e.scr.mapRows = rows
	e.scr.minMapRows = rows
	e.scr.cols = cols
	// Keep the columns/lines options in step with the terminal geometry.
	e.scr.opts.i["columns"] = cols
	e.scr.opts.i["lines"] = rows + 1
	e.scr.clampCursor()
	e.scr.scrollToCursor()
	e.fe.Render(view{e.scr}, ChangeSet{Full: true})
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
	before := e.snap()

	// A showmatch bracket flash is cleared by the next real key (which is then
	// processed normally, so the cursor returns and the keystroke takes effect).
	if e.scr.matchActive {
		switch ev.(type) {
		case KeyEvent, StringEvent:
			e.scr.matchActive = false
		}
	}

	// A pending output overlay (e.g. :set all) is dismissed by the next key.
	if e.scr.pendingOutput != nil {
		switch ev.(type) {
		case KeyEvent, StringEvent, InterruptEvent:
			e.scr.pendingOutput = nil
			e.mapPending = nil
			e.fe.Render(view{e.scr}, ChangeSet{Full: true})
			return
		}
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
	e.fe.Render(view{e.scr}, e.diff(before))
}

func (e *Engine) diff(b snap) ChangeSet {
	a := e.snap()
	return ChangeSet{
		CursorMoved:    a.cursor != b.cursor,
		Scrolled:       a.top != b.top,
		ModeChanged:    a.mode != b.mode,
		MessageChanged: a.msg != b.msg || a.kind != b.kind,
	}
}

func (e *Engine) interrupt() {
	s := e.scr
	if s.mode == ModeExColon {
		s.mode = ModeCommand
		s.colon = nil
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
	e.scr.filterL1, e.scr.filterL2 = 0, 0
}

func (e *Engine) cmdlineKey(ev KeyEvent) {
	s := e.scr
	switch {
	case ev.Key == KeyEnter || ev.Rune == '\r' || ev.Rune == '\n':
		line := string(s.colon)
		prefix := s.cmdPrefix
		s.mode = ModeCommand
		s.colon = nil
		e.runCmdline(prefix, line)
	case ev.Key == KeyEscape:
		s.mode = ModeCommand
		s.colon = nil
		s.filterL1, s.filterL2 = 0, 0
	case ev.Key == KeyBackspace || ev.Rune == 0x7f || ev.Rune == '\b':
		if len(s.colon) == 0 {
			s.mode = ModeCommand // backspacing past the prompt leaves the line
		} else {
			s.colon = s.colon[:len(s.colon)-1]
		}
	default:
		if ev.Rune != 0 {
			s.colon = append(s.colon, ev.Rune)
		}
	}
}

// runCmdline dispatches a completed command line by its prompt prefix.
func (e *Engine) runCmdline(prefix rune, line string) {
	switch prefix {
	case '/':
		if err := e.startSearch(line, searchFwd); err != nil {
			e.scr.msg, e.scr.msgKind = err.Error(), MsgError
		}
	case '?':
		if err := e.startSearch(line, searchBack); err != nil {
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
		e.runColon(strings.TrimSpace(line))
	}
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
