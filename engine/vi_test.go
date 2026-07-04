package engine

import (
	"strings"
	"testing"
)

// drive feeds a key script to the engine. ESC is '\x1b', Enter is '\r',
// Backspace is '\x7f'; all other runes are sent as plain key events.
func drive(e *Engine, keys string) {
	for _, r := range keys {
		switch r {
		case '\x1b':
			e.Input(KeyEvent{Key: KeyEscape})
		case '\r', '\n':
			e.Input(KeyEvent{Key: KeyEnter})
		case '\x7f':
			e.Input(KeyEvent{Key: KeyBackspace})
		default:
			e.Input(KeyEvent{Rune: r})
		}
	}
}

func bufText(e *Engine) string {
	n := e.scr.store.Lines()
	rows := make([]string, 0, n)
	for i := int64(1); i <= n; i++ {
		ln, _ := e.scr.store.Get(i)
		rows = append(rows, string(ln))
	}
	return strings.Join(rows, "\n")
}

// viCase runs a key script against initial content and checks the result.
func viCase(t *testing.T, name, initial, keys, want string) {
	t.Helper()
	e, _, _ := newTestEngine(t, initial)
	drive(e, keys)
	if got := bufText(e); got != want {
		t.Errorf("%s: keys %q on %q\n got %q\nwant %q", name, keys, initial, got, want)
	}
}

func TestHomeKeyCommandMode(t *testing.T) {
	// nvi maps khome to '^': first nonblank, not column 0 (that is vim).
	e, _, _ := newTestEngine(t, "  foo bar\n")
	drive(e, "$")
	e.Input(KeyEvent{Key: KeyHome})
	curAt(t, e, 1, 2, "Home to first nonblank")
	// End stays '$' (nvi kend).
	e.Input(KeyEvent{Key: KeyEnd})
	curAt(t, e, 1, 8, "End to last char")
}

func TestDelKeyCommandMode(t *testing.T) {
	// ^? (the DEL/Backspace key) is not a command key: nvi reports it.
	e, _, _ := newTestEngine(t, "hello\n")
	e.Input(KeyEvent{Rune: 0x7f})
	if e.scr.msg != "^? isn't a vi command" || e.scr.msgKind != MsgError {
		t.Fatalf("msg=%q kind=%v, want %q/MsgError", e.scr.msg, e.scr.msgKind, "^? isn't a vi command")
	}
	if e.scr.cursor.Col != 0 {
		t.Fatalf("^? moved the cursor to col %d; it should be inert", e.scr.cursor.Col)
	}
	// In insert mode the same key erases.
	e2, _, _ := newTestEngine(t, "ab\n")
	drive(e2, "A")
	e2.Input(KeyEvent{Rune: 0x7f})
	if got := bufText(e2); got != "a" {
		t.Fatalf("insert-mode ^? erase: got %q, want %q", got, "a")
	}
}

func TestViMotionsAndDelete(t *testing.T) {
	viCase(t, "x", "hello\n", "x", "ello")
	viCase(t, "3x", "hello\n", "3x", "lo")
	viCase(t, "dw", "hello world\n", "dw", "world")
	viCase(t, "dw-last-word", "foo bar\n", "wdw", "foo ")
	viCase(t, "dw-eol-keeps-newline", "foo\nbar\n", "dw", "\nbar")
	viCase(t, "d$", "hello world\n", "wd$", "hello ")
	viCase(t, "D", "hello world\n", "wD", "hello ")
	viCase(t, "dd", "a\nb\nc\n", "dd", "b\nc")
	viCase(t, "2dd", "a\nb\nc\nd\n", "2dd", "c\nd")
	viCase(t, "dj", "a\nb\nc\n", "dj", "c")
	viCase(t, "dG", "a\nb\nc\n", "dG", "")
	viCase(t, "de", "hello world\n", "de", " world")
	viCase(t, "df-inclusive", "a,b,c\n", "df,", "b,c")
	viCase(t, "dt", "a,b,c\n", "dt,", ",b,c")
}

// TestViEscapeCancelsPartialCommand covers nvi's <ESC> behavior in command mode:
// it abandons a pending operator, count, or register so the following key starts
// a fresh command (nvi v_cmd esc: handling). Regression for the audit finding
// that govi left a pending operator/count in place across <ESC>.
func TestViEscapeCancelsPartialCommand(t *testing.T) {
	// Operator then ESC: the operator is cancelled, the next key is its own
	// command (w just moves; the line is unchanged).
	viCase(t, "d<esc>w", "alpha beta gamma\n", "d\x1bw", "alpha beta gamma")
	// Count then ESC: the count is discarded, so x deletes a single char.
	viCase(t, "5<esc>x", "alpha beta gamma\n", "5\x1bx", "lpha beta gamma")
	// Count + operator then ESC: both are cancelled.
	viCase(t, "2d<esc>w", "alpha beta gamma\n", "2d\x1bw", "alpha beta gamma")
	// Register selection then ESC: cancelled, so the following dd is a plain
	// line delete into the unnamed register (not into "a).
	viCase(t, `"a<esc>dd`, "one\ntwo\n", "\"a\x1bdd", "two")
	// A cancelled operator must not leak into the next operator: d<esc>dd is a
	// normal line delete, not d+d (which would also delete a line but via the
	// wrong path) -- verify the result is a clean single-line delete.
	viCase(t, "d<esc>dd", "one\ntwo\nthree\n", "d\x1bdd", "two\nthree")
}

func TestViChange(t *testing.T) {
	viCase(t, "cw", "hello world\n", "cwbye\x1b", "bye world")
	viCase(t, "cc", "old line\nkeep\n", "ccnew\x1b", "new\nkeep")
	viCase(t, "C", "hello world\n", "wCthere\x1b", "hello there")
	viCase(t, "s", "abc\n", "sX\x1b", "Xbc")
}

func TestViInsert(t *testing.T) {
	viCase(t, "i", "bc\n", "iA\x1b", "Abc")
	viCase(t, "a", "ac\n", "ab\x1b", "abc")
	viCase(t, "A", "ab\n", "Ac\x1b", "abc")
	viCase(t, "I", "  bc\n", "IA\x1b", "  Abc")
	viCase(t, "o", "a\nc\n", "ob\x1b", "a\nb\nc")
	viCase(t, "O", "b\nc\n", "Oa\x1b", "a\nb\nc")
}

func TestViInsertNewline(t *testing.T) {
	// Move to column 5 (the 'w'), insert "foo\nbar" splitting the line.
	viCase(t, "split", "helloworld\n", "5lifoo\rbar\x1b", "hellofoo\nbarworld")
}

func TestViYankPut(t *testing.T) {
	viCase(t, "yyp", "one\ntwo\n", "yyp", "one\none\ntwo")
	viCase(t, "yyP", "one\ntwo\n", "yyP", "one\none\ntwo")
	viCase(t, "dd-p", "a\nb\nc\n", "ddp", "b\na\nc")
	viCase(t, "x-p-swap", "ab\n", "xp", "ba")
	viCase(t, "char-yank-put", "abc\n", "ylp", "aabc")
}

func TestViNamedRegister(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello\nworld\n")
	drive(e, `"ayy`) // yank line 1 into register a
	drive(e, "j")    // to line 2
	drive(e, `"ap`)  // put register a below
	if got := bufText(e); got != "hello\nworld\nhello" {
		t.Fatalf("named register put: got %q", got)
	}
}

func TestViJoinAndTilde(t *testing.T) {
	viCase(t, "J", "foo\nbar\n", "J", "foo bar")
	viCase(t, "J-no-space-before-paren", "foo\n)x\n", "J", "foo)x")
	viCase(t, "tilde", "abc\n", "~", "Abc")
	viCase(t, "3tilde", "abcd\n", "3~", "ABCd")
}

func TestViReplace(t *testing.T) {
	viCase(t, "r", "abc\n", "rX", "Xbc")
	viCase(t, "3r", "abcd\n", "3rX", "XXXd")
	viCase(t, "R", "abcdef\n", "RXYZ\x1b", "XYZdef")
}

func TestViUndoToggle(t *testing.T) {
	// nvi semantics: 'u' toggles undo/redo because the undo is itself the last
	// change. 'uu' is a no-op (undo then redo).
	e, _, _ := newTestEngine(t, "hello\n")
	drive(e, "x")
	if bufText(e) != "ello" {
		t.Fatalf("after x: %q", bufText(e))
	}
	drive(e, "u")
	if bufText(e) != "hello" {
		t.Fatalf("after u (undo): %q", bufText(e))
	}
	drive(e, "u")
	if bufText(e) != "ello" {
		t.Fatalf("after uu (redo via toggle): %q", bufText(e))
	}
	drive(e, "u")
	if bufText(e) != "hello" {
		t.Fatalf("after uuu (undo again): %q", bufText(e))
	}
}

func TestViUndoDotWalksBack(t *testing.T) {
	// '.' repeats the last command; after 'u' it repeats the undo, walking
	// back through history. 'u..' undoes three changes.
	e, _, _ := newTestEngine(t, "hello\n")
	drive(e, "xxx") // -> "lo" (three deletions)
	if bufText(e) != "lo" {
		t.Fatalf("after xxx: %q", bufText(e))
	}
	drive(e, "u") // undo 3rd x -> "llo"
	if bufText(e) != "llo" {
		t.Fatalf("after u: %q", bufText(e))
	}
	drive(e, ".") // undo 2nd x -> "ello"
	if bufText(e) != "ello" {
		t.Fatalf("after u.: %q", bufText(e))
	}
	drive(e, ".") // undo 1st x -> "hello"
	if bufText(e) != "hello" {
		t.Fatalf("after u..: %q", bufText(e))
	}
}

func TestViUndoRedoDirection(t *testing.T) {
	// The full nvi model: with 3 changes, u u u . . u . . walks undo then redo.
	// Make three single-char deletions producing distinct buffer states.
	e, _, _ := newTestEngine(t, "abcd\n")
	states := []string{"abcd"}
	drive(e, "x") // bcd
	states = append(states, bufText(e))
	drive(e, "x") // cd
	states = append(states, bufText(e))
	drive(e, "x") // d
	states = append(states, bufText(e))
	// states: [abcd, bcd, cd, d]; current = d (3 changes applied)

	want := func(label, w string) {
		if got := bufText(e); got != w {
			t.Fatalf("%s: got %q, want %q", label, got, w)
		}
	}
	drive(e, "u")
	want("u (undo 3)", states[2]) // cd
	drive(e, "u")
	want("u (redo 3)", states[3]) // d
	drive(e, "u")
	want("u (undo 3)", states[2]) // cd
	drive(e, ".")
	want("u.. (undo 2)", states[1]) // bcd
	drive(e, ".")
	want("u... (undo 1)", states[0]) // abcd
	drive(e, "u")
	want("u (redo 1)", states[1]) // bcd
	drive(e, ".")
	want(". (redo 2)", states[2]) // cd
	drive(e, ".")
	want(". (redo 3)", states[3]) // d
}

func TestViDotRepeat(t *testing.T) {
	viCase(t, "x-dot", "hello\n", "x..", "lo")
	viCase(t, "dw-dot", "a b c d\n", "dw.", "c d")
	viCase(t, "insert-dot", "\n", "ix\x1b.", "xx")
	viCase(t, "count-dot-override", "aaaaaaa\n", "x3.", "aaa")
}

func TestViCounts(t *testing.T) {
	e, _, _ := newTestEngine(t, "abcdefgh\n")
	drive(e, "3l")
	if e.scr.cursor.Col != 3 {
		t.Fatalf("3l -> col %d, want 3", e.scr.cursor.Col)
	}
	drive(e, "2h")
	if e.scr.cursor.Col != 1 {
		t.Fatalf("2h -> col %d, want 1", e.scr.cursor.Col)
	}
}

func TestViWordMotions(t *testing.T) {
	e, _, _ := newTestEngine(t, "foo bar baz\n")
	drive(e, "w")
	if e.scr.cursor.Col != 4 {
		t.Fatalf("w -> col %d, want 4", e.scr.cursor.Col)
	}
	drive(e, "w")
	if e.scr.cursor.Col != 8 {
		t.Fatalf("ww -> col %d, want 8", e.scr.cursor.Col)
	}
	drive(e, "b")
	if e.scr.cursor.Col != 4 {
		t.Fatalf("b -> col %d, want 4", e.scr.cursor.Col)
	}
	drive(e, "e")
	if e.scr.cursor.Col != 6 {
		t.Fatalf("e -> col %d, want 6", e.scr.cursor.Col)
	}
}

// TestViPrevContextMark covers CORNERS A-1: absolute motions (G / ? n % { } etc.),
// searches, and non-relative ex addresses record the pre-jump position under the
// previous-context mark, so '' and `` return to it and toggle. Operator targets
// (y/pat, dG) must not set it. Verified against nvi via the goterm A-1 probe.
func TestViPrevContextMark(t *testing.T) {
	body := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n15\n"
	lineAfter := func(keys string) int64 {
		e, _, _ := newTestEngine(t, body)
		drive(e, keys)
		return e.scr.cursor.Line
	}
	cases := []struct {
		name string
		keys string
		want int64
	}{
		{"G-sets-ctx", "10G''", 1},        // 10G records line 1; '' returns
		{"quote-toggles", "10G''''", 10},  // '' is itself absolute, so it toggles
		{"search-sets-ctx", "/12\r''", 1}, // / records line 1
		{"search-toggles", "/12\r''''", 12},
		{"n-sets-ctx", "/1\rn''", 10},         // / -> line 10, n -> line 11 (''=10)
		{"ex-addr-sets-ctx", ":13\r''", 1},    // :13 (number address) records line 1
		{"ex-addr-toggles", ":13\r''''", 13},  // '' toggles back to 13
		{"ex-dot-inert", ":.\r''", 1},         // :. is relative: '' unset -> no move
		{"H-after-G", "10GH''", 10},           // H records the pre-jump line 10
		{"op-y-no-set", "10G1Gy/5\r''", 10},   // yank target must not overwrite ''
		{"backtick-line", "5G`'''", 5},        // `mark reference aliases 'mark
	}
	for _, c := range cases {
		if got := lineAfter(c.keys); got != c.want {
			t.Errorf("%s: %q -> line %d, want %d", c.name, c.keys, got, c.want)
		}
	}
}

// TestViInsertCtrlD covers CORNERS A-2: insert-mode ^D erases autoindent (nvi
// txt_dent / K_CNTRLD), with the 0^D and ^^D forms, and is a literal control
// character past the indent or with no autoindent. cc/S keep the first line's
// indent as autoindent characters. Verified against nvi via the goterm A-2 probe.
func TestViInsertCtrlD(t *testing.T) {
	const D = "\x04" // ^D
	cases := []struct {
		name    string
		initial string
		keys    string
		want    string
	}{
		// o opens an 8-col autoindented line; ^D dedents one shiftwidth to 4.
		{"dedent-one", "        x\n", "A\r" + D + "done\x1b", "        x\n    done"},
		{"dedent-two", "        x\n", "A\r" + D + D + "done\x1b", "        x\ndone"},
		// 0^D and ^^D erase all the indent; ^^D reinstates it on the next line.
		{"zero-ctrld", "        x\n", "A\r0" + D + "done\x1b", "        x\ndone"},
		{"carat-ctrld", "        x\n", "A\r^" + D + "abc\rdef\x1b", "        x\nabc\n        def"},
		// Past the indent, or with no autoindent chars, ^D is a literal (0x04).
		{"past-indent", "        x\n", "A\rxy" + D + "z\x1b", "        x\n        xy\x04z"},
		{"plain-a-literal", "        x\n", "A" + D + "z\x1b", "        x\x04z"},
		// cc keeps the first line's indent as autoindent; ^D then dedents it.
		{"cc-keeps-indent", "        x\n", "ccfoo\x1b", "        foo"},
		{"cc-then-ctrld", "        x\n", "cc" + D + "foo\x1b", "    foo"},
	}
	for _, c := range cases {
		e, _, _ := newTestEngine(t, c.initial)
		if err := e.exExecute("set ai sw=4 ts=8"); err != nil {
			t.Fatalf("%s: set: %v", c.name, err)
		}
		drive(e, c.keys)
		if got := bufText(e); got != c.want {
			t.Errorf("%s: keys %q\n got %q\nwant %q", c.name, c.keys, got, c.want)
		}
	}
}

// TestViEmptyRegisterPut covers CORNERS B-4: an unknown register name (such as
// the "- small-delete buffer, which nvi does not implement) or an empty named
// register pastes nothing and reports the buffer as empty, rather than falling
// back to the unnamed register. Verified against nvi.
func TestViEmptyRegisterPut(t *testing.T) {
	// A small delete fills the unnamed register; "-p must NOT paste it.
	e, _, _ := newTestEngine(t, "two three\n")
	drive(e, `dw"-p`)
	if got := bufText(e); got != "three" {
		t.Errorf(`"-p pasted %q, want "three" (nothing pasted)`, got)
	}
	if e.scr.msg != "Buffer - is empty" {
		t.Errorf("message = %q, want %q", e.scr.msg, "Buffer - is empty")
	}
	// An empty named register reports itself, too.
	e2, _, _ := newTestEngine(t, "abc\n")
	drive(e2, `"zp`)
	if got := bufText(e2); got != "abc" {
		t.Errorf(`"zp changed buffer to %q`, got)
	}
	if e2.scr.msg != "Buffer z is empty" {
		t.Errorf("message = %q, want %q", e2.scr.msg, "Buffer z is empty")
	}
}

func TestViGotoLine(t *testing.T) {
	e, _, _ := newTestEngine(t, "a\nb\nc\nd\ne\n")
	drive(e, "G")
	if e.scr.cursor.Line != 5 {
		t.Fatalf("G -> line %d, want 5", e.scr.cursor.Line)
	}
	drive(e, "2G")
	if e.scr.cursor.Line != 2 {
		t.Fatalf("2G -> line %d, want 2", e.scr.cursor.Line)
	}
}
