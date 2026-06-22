package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	file *os.File // open handle backing a paged buffer, if any
	quit bool
}

// New returns an Engine that renders through fe. Call Open and Resize before
// feeding input.
func New(fe Frontend, _ Options) *Engine {
	e := &Engine{fe: fe}
	e.setBuffer(buffer.NewMem(), "")
	return e
}

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

// Open loads path into the active buffer. A missing file starts an empty,
// unsaved buffer named path (vi's "new file"). Large files are paged from disk
// rather than read whole.
func (e *Engine) Open(path string) error {
	store, fh, err := buffer.NewPagedFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			e.setBuffer(buffer.NewMem(), path)
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
	e.setBuffer(store, path)
	e.scr.msg = fmt.Sprintf("%q: %d lines", filepath.Base(path), store.Lines())
	e.scr.msgKind = MsgInfo
	return nil
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

// Resize sets the viewport geometry (text rows and columns) and repaints fully.
func (e *Engine) Resize(rows, cols int) {
	e.scr.rows = rows
	e.scr.cols = cols
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
		e.mapTimeout()
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
	}
	e.fe.Bell()
}

// dispatchKey routes a single keypress to the current mode's handler. Command,
// insert, and replace modes are handled by the vi state machine (vi.go); the
// command line (':' ex commands and '/' '?' searches) is handled here. Map
// expansion happens upstream in handleKeyEvent.
func (e *Engine) dispatchKey(ev KeyEvent) {
	if e.scr.mode == ModeExColon {
		e.cmdlineKey(ev)
		return
	}
	e.vi.key(e, ev)
}

// enterCmdline starts command-line input with the given prompt prefix.
func (e *Engine) enterCmdline(prefix rune) {
	e.scr.mode = ModeExColon
	e.scr.cmdPrefix = prefix
	e.scr.colon = nil
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
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return 0, 0, err
	}
	return n, bytesOut, nil
}
