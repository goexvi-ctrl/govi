package engine

import (
	"strings"

	"govi/engine/buffer"
	"govi/engine/undo"
)

// Colon command-line history (nvi's cedit option, vi/v_ex.c). While cedit is
// set, every command entered at the vi ':' prompt is appended to a hidden
// shared buffer; typing the cedit character at the prompt opens that buffer
// in a small vi window where <CR> executes the current line (v_ecl, a later
// stage). The history buffer is engine-wide and survives window close, like
// nvi's per-window ccl_sp screen holding a permanent reference to its EXF.

// ceditHistoryInit lazily creates the shared history buffer (nvi v_ecl_init).
// nvi backs it with an unnamed temp file with recovery turned off (F_RCV_ON
// cleared); govi uses a plain in-memory store, which the recovery code
// already ignores (no file name).
func (e *Engine) ceditHistoryInit() {
	if e.cclStore == nil {
		e.cclStore = buffer.NewMem()
		e.cclLog = undo.New(e.cclStore)
	}
}

// ceditLog appends one colon command to the history (nvi v_ecl_log). line is
// the command as typed, without the prompt; the stored line includes the
// leading ':' because nvi's v_tcmd keeps the prompt character at lb[0] and
// v_ecl_log appends the buffer whole (the ex parser strips it again on
// execution). A line identical to the current last history line is skipped.
// Callers gate on the cedit option being set (as v_ex does); logging happens
// before execution, so failing commands are logged too.
func (e *Engine) ceditLog(line string) {
	// Don't log colon commands typed inside the comedit window itself (nvi
	// v_ecl_log returns early when sp->ep == ccl_sp->ep).
	if e.scr.comedit {
		return
	}
	e.ceditHistoryInit()
	entry := []rune(":" + line)
	last := e.cclStore.Lines()
	if last > 0 {
		if prev, err := e.cclStore.Get(last); err == nil && string(prev) == string(entry) {
			return
		}
	}
	// Append through the undo log so the edit generation advances (the
	// comedit window's display cache keys on it) and the append is undoable
	// in the window, as nvi's logged db_append is.
	if last == 0 {
		e.cclLog.Insert(1, entry)
	} else {
		e.cclLog.Append(last, entry)
	}
}

// ceditTriggerKey reports whether ev is the cedit trigger character (the first
// character of the option). Special keys are matched the way colonFilecKey
// matches tab: an <escape> cedit fires on KeyEscape, a tab one on KeyTab.
func (e *Engine) ceditTriggerKey(ev KeyEvent) bool {
	ce := e.scr.opts.Str("cedit")
	if ce == "" {
		return false
	}
	ch := rune(ce[0])
	if ev.Rune == ch {
		return true
	}
	if ev.Mods&ModCtrl != 0 && ev.Key == KeyNone {
		if r, ok := ctrlRune(ev); ok && r == ch {
			return true
		}
	}
	switch {
	case ch == 0x1b && ev.Key == KeyEscape:
		return true
	case ch == '\t' && ev.Key == KeyTab:
		return true
	case ch == '\r' && ev.Key == KeyEnter:
		return true
	}
	return false
}

// ceditOpen opens the colon command-line edit window (nvi v_ecl): a small
// split screen below the current one, attached to the shared history buffer,
// cursor on the last line.
func (e *Engine) ceditOpen() {
	e.ceditHistoryInit()
	parent := e.scr
	ns := e.newSiblingScreen()
	ns.store = e.cclStore
	ns.log = e.cclLog
	ns.comedit = true
	last := e.cclStore.Lines()
	if last < 1 {
		last = 1
	}
	ns.cursor = Pos{Line: last, Col: 0}
	if err := e.splitHorizCcl(ns); err != nil {
		e.scr.msg, e.scr.msgKind = err.Error(), MsgError
		return
	}
	e.cclParent = parent
	// Both screens get the transient status line, as after any split.
	e.setStatusMsg(parent)
	e.setStatusMsg(ns)
}

// ceditExec runs the history line under the cursor in the originating screen
// and closes the window (nvi v_ecl_exec, reached from v_cr).
func (e *Engine) ceditExec() {
	s := e.scr
	if s.store.Lines() == 0 {
		// nvi v_emsg VIM_EMPTY (M_BERR).
		s.msg, s.msgKind = "The file is empty", MsgError
		e.fe.Bell()
		return
	}
	line := string(s.lineRunes(s.cursor.Line))
	if len(line) == 0 {
		s.msg, s.msgKind = "No ex command to execute", MsgError
		e.fe.Bell()
		return
	}
	e.ceditClose()
	// The command runs in the parent's context; the ex parser strips the
	// stored leading ':'. The execution itself is not re-logged (nvi pushes
	// via ex_run_str, which bypasses v_ex's logging).
	e.runColon(strings.TrimSpace(line))
}

// ceditClose closes the comedit window, folding its space back and focusing
// the originating screen (nvi returns to ccl_parent, not to a geometric
// neighbor). If the parent was itself closed while the window was open, the
// focus stays on the screen the discard chose.
func (e *Engine) ceditClose() {
	parent := e.cclParent
	e.cclParent = nil
	e.discardCurrentScreen(false)
	if parent == nil {
		return
	}
	for i, sc := range e.screens {
		if sc == parent {
			e.setCur(i)
			break
		}
	}
}
