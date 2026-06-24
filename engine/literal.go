package engine

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
