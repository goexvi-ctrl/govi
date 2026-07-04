package engine

import "testing"

// cclLines returns the colon history buffer's lines as strings.
func cclLines(e *Engine) []string {
	if e.cclStore == nil {
		return nil
	}
	var out []string
	for i := int64(1); i <= e.cclStore.Lines(); i++ {
		r, _ := e.cclStore.Get(i)
		out = append(out, string(r))
	}
	return out
}

func setCedit(e *Engine, s string) { e.scr.opts.s["cedit"] = s }

func TestCeditLogsColonCommands(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\ntwo\n")
	setCedit(e, "\x1b")

	typeColon(e, "set nu")
	typeColon(e, "set nonu")

	got := cclLines(e)
	want := []string{":set nu", ":set nonu"}
	if len(got) != len(want) {
		t.Fatalf("history = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("history[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCeditLogSkipsConsecutiveDuplicates(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\n")
	setCedit(e, "\x1b")

	typeColon(e, "set nu")
	typeColon(e, "set nu")
	if got := cclLines(e); len(got) != 1 || got[0] != ":set nu" {
		t.Fatalf("history = %q, want one :set nu", got)
	}

	// A non-consecutive repeat is logged again (nvi only compares against
	// the last line).
	typeColon(e, "set nonu")
	typeColon(e, "set nu")
	if got := cclLines(e); len(got) != 3 {
		t.Fatalf("history = %q, want 3 entries", got)
	}
}

func TestCeditNoLogWhenUnset(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\n")
	typeColon(e, "set nu")
	if e.cclStore != nil {
		t.Fatalf("history created with cedit unset: %q", cclLines(e))
	}
}

func TestCeditLogsFailingCommand(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\n")
	setCedit(e, "\x1b")
	typeColon(e, "bogus")
	if got := cclLines(e); len(got) != 1 || got[0] != ":bogus" {
		t.Fatalf("history = %q, want [:bogus]", got)
	}
}

func TestCeditLogsBareColon(t *testing.T) {
	// nvi's v_ecl_log appends the TEXT buffer whole; a bare ":<CR>" has just
	// the prompt at lb[0], so a lone ":" is logged.
	e, _, _ := newTestEngine(t, "one\n")
	setCedit(e, "\x1b")
	typeColon(e, "")
	if got := cclLines(e); len(got) != 1 || got[0] != ":" {
		t.Fatalf("history = %q, want [:]", got)
	}
}

func TestCeditGlobalBodyNotLogged(t *testing.T) {
	// Only the :g command itself goes through the vi colon prompt; its body
	// commands run via exExecute directly (as in nvi, where they bypass
	// v_ex's prompt loop).
	e, _, _ := newTestEngine(t, "one\ntwo\nthree\n")
	setCedit(e, "\x1b")
	typeColon(e, "g/o/s/o/O/")
	got := cclLines(e)
	if len(got) != 1 || got[0] != ":g/o/s/o/O/" {
		t.Fatalf("history = %q, want just the :g line", got)
	}
}

// findComedit returns the displayed comedit screen, or nil.
func findComedit(e *Engine) *screen {
	for _, sc := range e.screens {
		if sc.comedit {
			return sc
		}
	}
	return nil
}

// openCedit types ":" then the ESC trigger.
func openCedit(e *Engine) {
	e.Input(KeyEvent{Rune: ':'})
	e.Input(KeyEvent{Key: KeyEscape})
}

func TestCeditTriggerOpensWindow(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\ntwo\n")
	setCedit(e, "\x1b")
	typeColon(e, "set nu")
	typeColon(e, "set nonu")

	openCedit(e)
	if len(e.screens) != 2 {
		t.Fatalf("screens = %d, want 2", len(e.screens))
	}
	w := findComedit(e)
	if w == nil {
		t.Fatal("no comedit screen")
	}
	if e.scr != w {
		t.Fatal("comedit window should be focused")
	}
	if w.store != e.cclStore || w.log != e.cclLog {
		t.Fatal("window must attach the shared history buffer")
	}
	// Always below the parent (nvi vs_split ccl=1 never splits up).
	parent := e.cclParent
	if parent == nil || w.roff <= parent.roff {
		t.Fatalf("window roff %d not below parent %+v", w.roff, parent)
	}
	// Cursor on the last history line.
	if w.cursor.Line != 2 || w.cursor.Col != 0 {
		t.Fatalf("cursor = %+v, want line 2 col 0", w.cursor)
	}
	if got := string(w.lineRunes(2)); got != ":set nonu" {
		t.Fatalf("last line = %q", got)
	}
}

func TestCeditWindowAlwaysBelowAndCapped(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\ntwo\nthree\nfour\nfive\nsix\nseven\n")
	e.Resize(40, 80)
	setCedit(e, "\x1b")
	drive(e, "G") // cursor in the bottom half: a normal split would go on top
	openCedit(e)
	w := findComedit(e)
	if w == nil {
		t.Fatal("no comedit window")
	}
	if w.roff <= e.cclParent.roff {
		t.Fatal("ccl window must open below")
	}
	// nvi caps the ccl window at 6 display rows -> 5 text rows.
	if w.rows != 5 {
		t.Fatalf("window rows = %d, want 5", w.rows)
	}
}

func TestCeditPartialTextLoggedNotExecuted(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\n")
	setCedit(e, "\x1b")
	e.Input(KeyEvent{Rune: ':'})
	for _, r := range "xyzzy" {
		e.Input(KeyEvent{Rune: r})
	}
	e.Input(KeyEvent{Key: KeyEscape})
	if len(e.screens) != 2 {
		t.Fatalf("screens = %d, want 2 (window open)", len(e.screens))
	}
	got := cclLines(e)
	if len(got) != 1 || got[0] != ":xyzzy" {
		t.Fatalf("history = %q, want [:xyzzy]", got)
	}
	// xyzzy was never executed: no error message on the parent (it carries
	// the transient split status instead).
	if e.cclParent.msgKind == MsgError {
		t.Fatalf("partial text was executed: %q", e.cclParent.msg)
	}
}

func TestCeditCRExecutesInParent(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\n")
	setCedit(e, "\x1b")
	typeColon(e, "set nu")
	parent := e.scr
	openCedit(e)
	e.Input(KeyEvent{Key: KeyEnter})
	if len(e.screens) != 1 {
		t.Fatalf("screens = %d, want 1 (window closed)", len(e.screens))
	}
	if e.scr != parent {
		t.Fatal("parent screen must be focused after <CR>")
	}
	if !e.scr.opts.Bool("number") {
		t.Fatal(":set nu from the history did not run in the parent")
	}
	if e.cclParent != nil {
		t.Fatal("cclParent must be cleared on close")
	}
}

func TestCeditEditThenExecPersists(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\n")
	setCedit(e, "\x1b")
	typeColon(e, "set nu")
	openCedit(e)
	// Replace the line with :set nonu, then execute it.
	drive(e, "cc")
	for _, r := range ":set nonu" {
		e.Input(KeyEvent{Rune: r})
	}
	e.Input(KeyEvent{Key: KeyEscape})
	e.Input(KeyEvent{Key: KeyEnter})
	if len(e.screens) != 1 {
		t.Fatal("window should have closed")
	}
	if e.scr.opts.Bool("number") {
		t.Fatal("edited command :set nonu did not run")
	}
	// The edit persists in the shared history across close/reopen.
	openCedit(e)
	if got := string(e.scr.lineRunes(1)); got != ":set nonu" {
		t.Fatalf("history line = %q, want the edited text", got)
	}
}

func TestCeditEmptyBufferAndEmptyLineMessages(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\n")
	setCedit(e, "\x1b")
	openCedit(e) // no history yet
	e.Input(KeyEvent{Key: KeyEnter})
	if len(e.screens) != 2 {
		t.Fatal("window must stay open on the empty-buffer error")
	}
	if msg, k := e.scr.msg, e.scr.msgKind; k != MsgError || msg != "The file is empty" {
		t.Fatalf("msg = %q/%v, want The file is empty", msg, k)
	}
	// Add an empty line and try to execute it.
	drive(e, "o")
	e.Input(KeyEvent{Key: KeyEscape})
	e.Input(KeyEvent{Key: KeyEnter})
	if len(e.screens) != 2 {
		t.Fatal("window must stay open on the empty-line error")
	}
	if msg, k := e.scr.msg, e.scr.msgKind; k != MsgError || msg != "No ex command to execute" {
		t.Fatalf("msg = %q/%v, want No ex command to execute", msg, k)
	}
}

func TestCeditCtrlWRefused(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\n")
	setCedit(e, "\x1b")
	typeColon(e, "set nu")
	openCedit(e)
	e.Input(KeyEvent{Rune: 'w', Mods: ModCtrl})
	if e.scr != findComedit(e) {
		t.Fatal("^W must not leave the comedit window")
	}
	want := "Enter <CR> to execute a command, :q to exit"
	if msg, k := e.scr.msg, e.scr.msgKind; k != MsgError || msg != want {
		t.Fatalf("msg = %q/%v, want %q", msg, k, want)
	}
}

func TestCeditQuitSilentDespiteEdits(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\n")
	setCedit(e, "\x1b")
	typeColon(e, "set nu")
	parent := e.scr
	openCedit(e)
	drive(e, "x") // dirty the history buffer
	typeColon(e, "q")
	if len(e.screens) != 1 || e.scr != parent {
		t.Fatal(":q must close the window and focus the parent")
	}
	if e.scr.msgKind == MsgError {
		t.Fatalf(":q raised %q; comedit quits silently", e.scr.msg)
	}
	if e.ShouldQuit() {
		t.Fatal(":q in the window must not exit the editor")
	}
}

func TestCeditInWindowColonCommandsNotLogged(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\n")
	setCedit(e, "\x1b")
	typeColon(e, "set nu")
	openCedit(e)
	typeColon(e, "set list") // runs on the window screen; must not be logged
	typeColon(e, "q")
	got := cclLines(e)
	if len(got) != 1 || got[0] != ":set nu" {
		t.Fatalf("history = %q, want just [:set nu]", got)
	}
}

func TestCeditLiteralNextInsertsTrigger(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\n")
	setCedit(e, "\x1b")
	e.Input(KeyEvent{Rune: ':'})
	e.Input(KeyEvent{Rune: 'v', Mods: ModCtrl})
	e.Input(KeyEvent{Key: KeyEscape})
	if len(e.screens) != 1 {
		t.Fatal("^V-quoted trigger must insert, not open the window")
	}
	if e.scr.mode != ModeExColon || len(e.scr.colon) != 1 || e.scr.colon[0] != 0x1b {
		t.Fatalf("colon = %q, want a literal ESC", string(e.scr.colon))
	}
}

func TestCeditEscapeCancelsWhenUnset(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\n")
	e.Input(KeyEvent{Rune: ':'})
	e.Input(KeyEvent{Key: KeyEscape})
	if len(e.screens) != 1 || e.scr.mode != ModeCommand {
		t.Fatal("ESC with cedit unset must just cancel the colon line")
	}
}

func TestCeditTabSharedWithFilec(t *testing.T) {
	e, _, _ := newTestEngine(t, "one\n")
	setCedit(e, "\t") // filec defaults to tab too
	typeColon(e, "set nu")

	// Tab on an empty colon line: cedit wins, the window opens.
	e.Input(KeyEvent{Rune: ':'})
	e.Input(KeyEvent{Key: KeyTab})
	if len(e.screens) != 2 || findComedit(e) == nil {
		t.Fatal("tab on an empty colon line must open the history window")
	}
	typeColon(e, "q")

	// Tab after text: file completion wins, no window.
	e.Input(KeyEvent{Rune: ':'})
	for _, r := range "e nosuchprefix" {
		e.Input(KeyEvent{Rune: r})
	}
	e.Input(KeyEvent{Key: KeyTab})
	if len(e.screens) != 1 {
		t.Fatal("tab after text must fall through to file completion")
	}
	if e.scr.mode != ModeExColon {
		t.Fatal("still on the colon line after completion attempt")
	}
	e.Input(KeyEvent{Key: KeyEscape})
}

func TestCeditSearchPromptsNotLogged(t *testing.T) {
	// The / and ? prompts never log (nvi passes TXT_CEDIT only for ':').
	e, _, _ := newTestEngine(t, "one\ntwo\n")
	setCedit(e, "\x1b")
	e.Input(KeyEvent{Rune: '/'})
	for _, r := range "two" {
		e.Input(KeyEvent{Rune: r})
	}
	e.Input(KeyEvent{Key: KeyEnter})
	if e.cclStore != nil {
		t.Fatalf("search logged to colon history: %q", cclLines(e))
	}
}
