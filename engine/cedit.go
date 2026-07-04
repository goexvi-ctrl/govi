package engine

import (
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
