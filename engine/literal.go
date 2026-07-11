package engine

// isLinefeedEvent reports the <newline> key (a raw \n byte, distinct from
// <Enter>/CR): terminal hosts deliver it as Ctrl+'j' (tcell parses 0x0A as
// KeyCtrlJ), GUI hosts as a bare '\n' rune. Terminals that paste multi-line
// text with \n line endings (e.g. iPadSSH; Terminal.app converts to \r) send
// one for every line break, and nvi's key table (K_NL) treats it as a line
// end everywhere text is being input.
func isLinefeedEvent(ev KeyEvent) bool {
	if ev.Key != KeyNone {
		return false
	}
	if ev.Rune == '\n' && ev.Mods&ModCtrl == 0 {
		return true
	}
	return ev.Rune == 'j' && ev.Mods&ModCtrl != 0
}

// isNewlineEvent reports a line-ending key in text input: <Enter>/CR or the
// <newline> key (nvi makes K_CR and K_NL equivalent in vi/v_txt.c and ex).
func isNewlineEvent(ev KeyEvent) bool {
	if ev.Key == KeyEnter || ev.Rune == '\r' {
		return true
	}
	return isLinefeedEvent(ev)
}

// ctrlRune maps a control-modified key event to its ASCII control code
// (^A -> SOH, ^@ -> NUL). The bool is false when the event is not a Ctrl key.
func ctrlRune(ev KeyEvent) (rune, bool) {
	if ev.Mods&ModCtrl == 0 || ev.Key != KeyNone {
		return 0, false
	}
	if ev.Rune == 0 {
		return 0, true
	}
	return ev.Rune & 0x1f, true
}

// literalRune returns the rune to insert after ^V (VLNEXT). Control-modified
// keys map to their ASCII control code (^A -> 1, ^@ -> NUL); special keys map
// to the character they would insert literally. The bool is false when the event
// carries no quotable character.
func literalRune(ev KeyEvent) (rune, bool) {
	if r, ok := ctrlRune(ev); ok {
		return r, true
	}
	switch ev.Key {
	case KeyEnter:
		return '\r', true
	case KeyTab:
		return '\t', true
	case KeyEscape:
		return 0x1b, true
	case KeyBackspace:
		return '\b', true
	}
	switch ev.Rune {
	case '\r':
		return '\r', true
	case '\n':
		return '\n', true
	case '\t':
		return '\t', true
	case '\b', 0x7f:
		return '\b', true
	case 0x1b:
		return 0x1b, true
	}
	if ev.Rune != 0 {
		return ev.Rune, true
	}
	if ev.Rune == 0 && ev.Key == KeyNone && ev.Mods&ModCtrl != 0 {
		return 0, true
	}
	return 0, false
}
