package tcell

import (
	"strings"
	"testing"

	"govi/engine"
)

// TestInputKeySplitsMergedControl reproduces the scripted-input stream
// ":set cedit=<^V><esc><cr>" arriving in one read: tcell merges "<esc><cr>"
// into Alt+Ctrl+M (input.go inpStateEsc turns a control byte into letter+Ctrl),
// and inputKey must resolve it to a plain Escape followed by Enter -- the
// events the bytes would have produced alone. Before the fix the remainder
// arrived as Ctrl+M, which the colon editor appended as a literal ^M, leaving
// the command line open and swallowing the following command.
func TestInputKeySplitsMergedControl(t *testing.T) {
	eng, sim := setup(t, "one\ntwo\n", 80, 24)
	fe, err := NewWithScreen(sim)
	if err != nil {
		t.Fatal(err)
	}
	fe.Attach(eng)

	feed := func(evs ...engine.Event) {
		for _, ev := range evs {
			fe.inputKey(ev)
		}
	}
	// Type :set cedit= then ^V, then the merged Alt+Ctrl+M for "<esc><cr>".
	feed(engine.KeyEvent{Rune: ':'})
	for _, r := range "set cedit=" {
		feed(engine.KeyEvent{Rune: r})
	}
	feed(engine.KeyEvent{Rune: 'v', Mods: engine.ModCtrl})
	feed(engine.KeyEvent{Rune: 'm', Mods: engine.ModCtrl | engine.ModAlt})

	// The option took effect iff the cedit trigger now opens the history
	// window: a split appears, putting the parent's status line mid-screen.
	feed(engine.KeyEvent{Rune: ':'})
	feed(engine.KeyEvent{Key: engine.KeyEscape})
	rows := rowsOf(t, sim)
	joined := strings.Join(rows[:len(rows)-1], "\n")
	if !strings.Contains(joined, "unmodified: line 1") {
		t.Fatalf("no comedit split after merged <esc><cr>; screen:\n%s", strings.Join(rows, "\n"))
	}
}
