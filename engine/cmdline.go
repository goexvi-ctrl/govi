package engine

import "golang.org/x/text/unicode/norm"

// colonEditOpts configures shared colon/ex command-line editing (nvi v_tcmd).
type colonEditOpts struct {
	leaveOnEmptyBackspace bool // vi colon: backspace past start cancels the line
	onEnter               func(line string)
	onEscape              func()

	// onCedit, when set, enables the cedit trigger (nvi TXT_CEDIT): typing
	// the cedit character ends the input and opens the colon history window.
	// Only the vi ':' prompt sets it. line is any text already typed.
	onCedit func(line string)
}

// colonEditKey handles one key on a colon-style input line: control characters
// (^V quote, ^U kill, ^W word erase, ^H erase, ^X hex) and plain text.
func (e *Engine) colonEditKey(ev KeyEvent, opts colonEditOpts) {
	s := e.scr

	if s.cmdLiteralNext {
		s.cmdLiteralNext = false
		if r, ok := literalRune(ev); ok {
			s.colon = appendColonRune(s.colon, r)
		}
		return
	}

	if s.cmdHexMode {
		if isHexDigit(ev.Rune) {
			s.cmdHexBuf = append(s.cmdHexBuf, ev.Rune)
			return
		}
		if len(s.cmdHexBuf) > 0 {
			s.colon = append(s.colon, s.cmdlineFinishHex())
		}
	}

	if colonInterrupt(ev) {
		s.resetColonEdit()
		if opts.onEscape != nil {
			opts.onEscape()
		}
		return
	}

	// cedit trigger (nvi v_txt.c: checked when not quoted, before the special
	// character handling, so a cedit of <escape> or a control character wins
	// over the cancel/edit-key meanings). When cedit and filec share the same
	// character, cedit wins only at the first input column (govi's colon
	// editor is append-only, so that degenerates to "the line is empty");
	// otherwise fall through to file completion.
	if opts.onCedit != nil && e.ceditTriggerKey(ev) {
		fc := s.opts.Str("filec")
		ce := s.opts.Str("cedit")
		if fc == "" || fc[0] != ce[0] || len(s.colon) == 0 {
			line := string(s.colon)
			s.resetColonEdit()
			opts.onCedit(line)
			return
		}
	}

	if colonControlKey(ev, s) {
		return
	}

	if e.colonFilecKey(ev) && colonExpectsPathArg(s.colon) {
		e.colonDoFileComplete()
		return
	}

	switch {
	case ev.Key == KeyEnter || ev.Rune == '\r' || ev.Rune == '\n':
		line := string(s.colon)
		s.resetColonEdit()
		opts.onEnter(line)
	case ev.Key == KeyEscape:
		s.resetColonEdit()
		if opts.onEscape != nil {
			opts.onEscape()
		}
	case ev.Key == KeyBackspace || ev.Rune == '\b' || ev.Rune == 0x7f:
		if len(s.colon) == 0 && opts.leaveOnEmptyBackspace {
			s.mode = ModeCommand
			s.filterL1, s.filterL2 = 0, 0
		} else {
			s.colonBackspace()
		}
	default:
		if ev.Rune != 0 && ev.Rune != 0x1b {
			s.colon = appendColonRune(s.colon, ev.Rune)
		}
	}
}

func appendColonRune(colon []rune, r rune) []rune {
	return []rune(norm.NFC.String(string(append(colon, r))))
}

func colonInterrupt(ev KeyEvent) bool {
	if ev.Rune == 0x03 {
		return true
	}
	return ev.Mods&ModCtrl != 0 && ev.Key == KeyNone && (ev.Rune == 'c' || ev.Rune == 'C')
}

func colonControlKey(ev KeyEvent, s *screen) bool {
	if ev.Mods&ModCtrl != 0 && ev.Key == KeyNone {
		switch ev.Rune {
		case 'v':
			s.cmdLiteralNext = true
			return true
		case 'u':
			s.colonKill()
			return true
		case 'w':
			s.colonWordErase()
			return true
		case 'h':
			s.colonBackspace()
			return true
		case 'x':
			s.cmdHexMode = true
			s.cmdHexBuf = s.cmdHexBuf[:0]
			return true
		default:
			if r, ok := ctrlRune(ev); ok {
				s.colon = appendColonRune(s.colon, r)
				return true
			}
		}
	}
	switch ev.Rune {
	case 0x15: // ^U VKILL
		s.colonKill()
		return true
	case 0x16: // ^V VLNEXT
		s.cmdLiteralNext = true
		return true
	case 0x17: // ^W VWERASE
		s.colonWordErase()
		return true
	case 0x18: // ^X hex entry
		s.cmdHexMode = true
		s.cmdHexBuf = s.cmdHexBuf[:0]
		return true
	}
	return false
}

func (s *screen) colonBackspace() {
	if len(s.colon) > 0 {
		s.colon = s.colon[:len(s.colon)-1]
	}
}

func (s *screen) colonKill() {
	s.colon = nil
}

func (s *screen) colonWordErase() {
	col := len(s.colon)
	i := col
	for i > 0 && (s.colon[i-1] == ' ' || s.colon[i-1] == '\t') {
		i--
	}
	if i > 0 {
		if isWordRune(s.colon[i-1]) {
			for i > 0 && isWordRune(s.colon[i-1]) {
				i--
			}
		} else {
			for i > 0 && !isWordRune(s.colon[i-1]) && s.colon[i-1] != ' ' && s.colon[i-1] != '\t' {
				i--
			}
		}
	}
	s.colon = s.colon[:i]
}

func (s *screen) cmdlineFinishHex() rune {
	s.cmdHexMode = false
	if len(s.cmdHexBuf) == 0 {
		return 0
	}
	var v int64
	for _, r := range s.cmdHexBuf {
		v = v*16 + int64(hexVal(r))
	}
	s.cmdHexBuf = s.cmdHexBuf[:0]
	return rune(v)
}

func (s *screen) resetColonEdit() {
	s.cmdLiteralNext = false
	s.cmdHexMode = false
	s.cmdHexBuf = s.cmdHexBuf[:0]
}

// colonDisplayMessage formats the in-progress colon/ex command line for the
// status line.
func (v view) colonDisplayMessage() string {
	line := v.s.colon
	if v.s.exInput == nil {
		prefix := v.s.cmdPrefix
		if prefix == 0 {
			prefix = ':'
		}
		line = append([]rune{prefix}, line...)
	}
	text := FormatColonLine(line, v.s.opts.Int("tabstop"), v.s.opts.Bool("list"))
	if v.s.cmdLiteralNext {
		text += "^"
	}
	return text
}
