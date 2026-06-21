package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govi/engine/buffer"
	"govi/engine/mark"
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
		name:   name,
		cursor: Pos{Line: 1, Col: 0},
		top:    1,
		mode:   ModeCommand,
	}
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
		for _, r := range v.Text {
			e.key(KeyEvent{Rune: r})
		}
	case KeyEvent:
		e.key(v)
	case InterruptEvent:
		e.interrupt()
	default:
		// SuspendEvent, TimeoutEvent: nothing to do in Phase 2.
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

// key dispatches a single keypress according to the current mode. The command
// handling here is a Phase 2 placeholder: it supports just enough motion and
// colon handling to prove the embedding seam. Phase 3 replaces it with the
// engine/vi vikeys dispatch and Phase 4 with engine/ex.
func (e *Engine) key(ev KeyEvent) {
	switch e.scr.mode {
	case ModeExColon:
		e.colonKey(ev)
	default:
		e.commandKey(ev)
	}
}

func (e *Engine) commandKey(ev KeyEvent) {
	s := e.scr
	s.msg, s.msgKind = "", MsgNone

	if ev.Mods&ModCtrl != 0 {
		switch ev.Rune {
		case 'f': // page down
			s.cursor.Line += int64(max(1, s.rows-2))
		case 'b': // page up
			s.cursor.Line -= int64(max(1, s.rows-2))
		}
		return
	}

	switch ev.Key {
	case KeyLeft:
		s.cursor.Col--
		return
	case KeyRight:
		s.cursor.Col++
		return
	case KeyDown:
		s.cursor.Line++
		return
	case KeyUp:
		s.cursor.Line--
		return
	case KeyPageDown:
		s.cursor.Line += int64(max(1, s.rows-2))
		return
	case KeyPageUp:
		s.cursor.Line -= int64(max(1, s.rows-2))
		return
	case KeyHome:
		s.cursor.Col = 0
		return
	case KeyEnd:
		s.cursor.Col = len(s.lineRunes(s.cursor.Line)) - 1
		return
	}

	switch ev.Rune {
	case 'h':
		s.cursor.Col--
	case 'l', ' ':
		s.cursor.Col++
	case 'j':
		s.cursor.Line++
	case 'k':
		s.cursor.Line--
	case '0':
		s.cursor.Col = 0
	case '$':
		s.cursor.Col = len(s.lineRunes(s.cursor.Line)) - 1
	case 'G':
		s.cursor.Line = s.lineCount()
	case ':':
		s.mode = ModeExColon
		s.colon = nil
	}
}

func (e *Engine) colonKey(ev KeyEvent) {
	s := e.scr
	switch {
	case ev.Key == KeyEnter || ev.Rune == '\r' || ev.Rune == '\n':
		cmd := strings.TrimSpace(string(s.colon))
		s.mode = ModeCommand
		s.colon = nil
		e.runColon(cmd)
	case ev.Key == KeyEscape:
		s.mode = ModeCommand
		s.colon = nil
	case ev.Key == KeyBackspace || ev.Rune == 0x7f || ev.Rune == '\b':
		if len(s.colon) == 0 {
			s.mode = ModeCommand // backspacing past the ':' leaves colon mode
		} else {
			s.colon = s.colon[:len(s.colon)-1]
		}
	default:
		if ev.Rune != 0 {
			s.colon = append(s.colon, ev.Rune)
		}
	}
}

// runColon handles a tiny set of ex commands so the editor can be saved and
// exited. The real ex parser and command table arrive in Phase 4.
func (e *Engine) runColon(cmd string) {
	s := e.scr
	switch cmd {
	case "":
		// no-op
	case "q", "q!":
		if cmd == "q" && s.modified {
			s.msg, s.msgKind = "No write since last change (use :q! to override)", MsgError
			return
		}
		e.quit = true
	case "w":
		e.write()
	case "wq", "x", "x!", "wq!":
		if e.write() {
			e.quit = true
		}
	default:
		s.msg, s.msgKind = fmt.Sprintf("The %q command is not yet implemented", cmd), MsgError
	}
}

// write saves the buffer to its file, atomically via a temp file + rename.
// Returns true on success.
func (e *Engine) write() bool {
	s := e.scr
	if s.name == "" {
		s.msg, s.msgKind = "No current filename", MsgError
		return false
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.name), ".govi-*")
	if err != nil {
		s.msg, s.msgKind = err.Error(), MsgError
		return false
	}
	tmpName := tmp.Name()
	w := tmp
	n := s.store.Lines()
	var bytesOut int64
	for i := int64(1); i <= n; i++ {
		line, _ := s.store.Get(i)
		b := []byte(string(line))
		w.Write(b)
		w.Write([]byte{'\n'})
		bytesOut += int64(len(b)) + 1
	}
	if err := w.Close(); err != nil {
		os.Remove(tmpName)
		s.msg, s.msgKind = err.Error(), MsgError
		return false
	}
	if err := os.Rename(tmpName, s.name); err != nil {
		os.Remove(tmpName)
		s.msg, s.msgKind = err.Error(), MsgError
		return false
	}
	s.modified = false
	s.msg = fmt.Sprintf("%q: %d lines, %d bytes", filepath.Base(s.name), n, bytesOut)
	s.msgKind = MsgInfo
	return true
}
