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
