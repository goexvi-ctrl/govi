package engine

import (
	"fmt"
	"strings"
)

// Ex (line-editor) mode, entered from vi with Q and left with :vi / :visual.
// Unlike the vi colon line, ex mode is a scrolling transcript: each command is
// echoed, its output appended, and a new ":" prompt shown. This corresponds to
// nvi's ex.c top-level loop.

// enterExMode switches from vi command mode into ex mode. Unlike Vim, the 4.4BSD
// ex prints no banner -- it simply drops to a ":" prompt.
func (e *Engine) enterExMode() {
	e.scr.mode = ModeExText
	e.scr.colon = nil
	e.scr.cmdPrefix = ':'
}

// ExActive reports whether the editor is in line-oriented ex mode (entered with
// Q). A terminal host renders this as a scrolling line transcript -- leaving the
// full-screen display -- rather than a cursor-addressed screen.
func (e *Engine) ExActive() bool { return e.scr.mode == ModeExText }

// ExPrompt returns the prompt a line host prints before reading the next line:
// ":" normally, or "" while an a/i/c command is collecting input text.
func (e *Engine) ExPrompt() string {
	if e.scr.exInput != nil {
		return ""
	}
	return ":"
}

// ExFeedLine processes one line entered at the ex prompt by a line-oriented host
// (which has already echoed it) and returns the output lines to print. Entering
// "visual"/"vi" leaves ex mode (ExActive then reports false).
func (e *Engine) ExFeedLine(line string) []string {
	e.exOut = nil
	e.exLineMode = true
	defer func() { e.exLineMode = false }()

	if e.scr.exInput != nil {
		if line == "." {
			e.exInputFinish()
		} else {
			e.scr.exInput.lines = append(e.scr.exInput.lines, []rune(line))
		}
		return e.exOut
	}

	if IsBackslashLine(line) {
		e.quitFromBackslash()
		return []string{QuitCommandDisplay}
	}

	trimmed := strings.TrimSpace(line)
	switch trimmed {
	case "vi", "visual", "vis":
		e.exitExMode()
		return nil
	}
	if trimmed == "" {
		text, _ := e.ExStep()
		e.exOut = append(e.exOut, text)
		return e.exOut
	}
	if err := e.exExecute(line); err != nil {
		e.exOut = append(e.exOut, err.Error())
	} else if e.scr.msg != "" {
		e.exOut = append(e.exOut, e.scr.msg)
		e.scr.msg = ""
	}
	return e.exOut
}

// ExStep advances to the next line for a bare <enter> at the ex prompt, the way
// ex steps through a file. It returns the next line's text and ok=true, or the
// end-of-file message with ok=false. On a successful step the line replaces the
// ":" prompt the host already drew (the host should overwrite the prompt line);
// on failure the prompt stays and the message is shown below it.
func (e *Engine) ExStep() (text string, ok bool) {
	next := e.scr.cursor.Line + 1
	if next > e.scr.store.Lines() {
		return "at end-of-file", false
	}
	e.scr.cursor = Pos{Line: next, Col: 0}
	return string(e.scr.lineRunes(next)), true
}

// exPrintLines prints lines [l1, l2] to the ex output and sets the current line
// to the last one printed -- the ex behavior for a bare line address.
func (e *Engine) exPrintLines(l1, l2 int64) error {
	for ln := l1; ln <= l2; ln++ {
		e.printLine(string(e.scr.lineRunes(ln)))
	}
	e.scr.cursor = Pos{Line: clampLine(e.scr, l2), Col: 0}
	return nil
}

// exitExMode returns to vi command mode.
func (e *Engine) exitExMode() {
	e.scr.mode = ModeCommand
	e.scr.colon = nil
	e.scr.exTranscript = nil
}

// exVisual implements :visual / :vi -- return to vi mode from ex mode.
func (e *Engine) exVisual(c *exCmd) error {
	if e.scr.mode == ModeExText {
		e.exitExMode()
	}
	return nil
}

// exEcho emits a line of ex output: to the line-host buffer when one is driving
// (Q line mode), otherwise appended to the in-screen transcript.
func (e *Engine) exEcho(s string) {
	if e.startup {
		return
	}
	if e.exLineMode {
		e.exOut = append(e.exOut, s)
		return
	}
	e.scr.exTranscript = append(e.scr.exTranscript, s)
}

// showOutput presents multi-line command output: appended to the transcript in
// ex mode, or shown as an overlay (dismissed by the next key) in vi mode.
func (e *Engine) showOutput(lines []string) {
	if e.startup {
		return
	}
	if e.scr.mode == ModeExText {
		for _, l := range lines {
			e.exEcho(l)
		}
		return
	}
	e.scr.pendingOutput = lines
	e.scr.pendingPage = 0
}

// printLine emits a line of command output: to the transcript in ex mode, or to
// the message line in vi mode.
func (e *Engine) printLine(s string) {
	if e.startup {
		return
	}
	if e.scr.mode == ModeExText {
		e.exEcho(s)
	} else {
		e.scr.msg, e.scr.msgKind = s, MsgInfo
	}
}

// exInputState collects the text typed after an ex a/i/c command until a line
// containing only "." terminates it; the collected lines are then inserted.
type exInputState struct {
	kind         rune  // 'a' append, 'i' insert, 'c' change
	at           int64 // insert collected lines after this line (a/i)
	delL1, delL2 int64 // lines to delete first (c)
	lines        [][]rune
}

// exInputActive reports whether ex input (a/i/c) is collecting lines.
func (e *Engine) exInputActive() bool { return e.scr.exInput != nil }

// exStartInput begins collecting input for an a/i/c command.
func (e *Engine) exStartInput(c *exCmd, kind rune) error {
	s := e.scr
	st := &exInputState{kind: kind}
	switch kind {
	case 'a':
		at := s.cursor.Line
		if c.addrCount > 0 {
			at = c.addr2
		}
		st.at = at
	case 'i':
		at := s.cursor.Line
		if c.addrCount > 0 {
			at = c.addr2
		}
		st.at = at - 1 // insert before the line = append after the previous one
	case 'c':
		l1, l2, err := e.rangeOf(c)
		if err != nil {
			return err
		}
		st.delL1, st.delL2 = l1, l2
	}
	s.exInput = st
	return nil
}

// exInputKey handles a keypress while collecting a/i/c input. Lines accumulate
// until one consisting solely of "." ends the command.
func (e *Engine) exInputKey(ev KeyEvent) {
	s := e.scr
	e.colonEditKey(ev, colonEditOpts{
		onEnter: func(line string) {
			s.colon = nil
			if line == "." {
				if s.mode == ModeExText {
					e.exEcho(".")
				}
				e.exInputFinish()
				return
			}
			if s.mode == ModeExText {
				e.exEcho(line)
			}
			s.exInput.lines = append(s.exInput.lines, []rune(line))
		},
		onEscape: func() {
			e.exInputFinish()
		},
	})
}

// exInputFinish inserts the collected lines into the buffer as one undo unit.
func (e *Engine) exInputFinish() {
	s := e.scr
	st := s.exInput
	s.exInput = nil
	if st == nil {
		return
	}
	e.beginChange()
	if st.kind == 'c' {
		for ln := st.delL2; ln >= st.delL1; ln-- {
			s.deleteLine(ln)
		}
		st.at = st.delL1 - 1
	}
	last := e.insertLinesAfter(st.at, st.lines)
	if s.store.Lines() == 0 {
		s.log.Insert(1, []rune{})
		last = 1
	}
	e.endChange()
	if last < 1 {
		last = 1
	}
	s.cursor = Pos{Line: clampLine(s, last), Col: 0}
	s.clampCursor()
}

// insertLinesAfter inserts lines into the buffer after line `at` (at == 0 means
// before the first line), returning the last inserted line number.
func (e *Engine) insertLinesAfter(at int64, lines [][]rune) int64 {
	s := e.scr
	last := at
	for i, ln := range lines {
		if at == 0 && i == 0 {
			s.insertLine(1, cloneR(ln))
			last = 1
		} else {
			s.appendLine(last, cloneR(ln))
			last++
		}
	}
	return last
}

// ex a/i/c command entry points.
func (e *Engine) exAppend(c *exCmd) error { return e.exStartInput(c, 'a') }
func (e *Engine) exInsert(c *exCmd) error { return e.exStartInput(c, 'i') }
func (e *Engine) exChange(c *exCmd) error { return e.exStartInput(c, 'c') }

// exModeKey handles a keypress while in ex mode.
func (e *Engine) exModeKey(ev KeyEvent) {
	s := e.scr
	if ev.Rune == '\x1c' || (ev.Mods&ModCtrl != 0 && ev.Rune == '\\') {
		e.quitFromBackslash()
		return
	}
	e.colonEditKey(ev, colonEditOpts{
		onEnter: func(cmd string) {
			s.colon = nil
			trimmed := strings.TrimSpace(cmd)
			switch trimmed {
			case "vi", "visual", "vis":
				e.exEcho(":" + cmd)
				e.exitExMode()
				return
			}
			if trimmed == "" {
				if text, ok := e.ExStep(); ok {
					e.exEcho(text)
				} else {
					e.exEcho(":")
					e.exEcho(text)
				}
				return
			}
			e.exEcho(":" + cmd)
			if err := e.exExecute(cmd); err != nil {
				e.exEcho(err.Error())
			} else if s.msg != "" {
				e.exEcho(s.msg)
				s.msg = ""
			}
		},
	})
}

// --- print commands (also usable from the vi colon line) ---

func (e *Engine) exPrint(c *exCmd) error  { return e.printRange(c, false, false) }
func (e *Engine) exNumber(c *exCmd) error { return e.printRange(c, true, false) }
func (e *Engine) exList(c *exCmd) error   { return e.printRange(c, false, true) }

func (e *Engine) printRange(c *exCmd, number, list bool) error {
	l1, l2, err := e.rangeOf(c)
	if err != nil {
		return err
	}
	useList := list || e.scr.opts.Bool("list")
	for ln := l1; ln <= l2; ln++ {
		runes := e.scr.lineRunes(ln)
		var text string
		if useList {
			text = FormatListLine(runes)
		} else {
			text = string(runes)
		}
		if number {
			text = fmt.Sprintf("%6d  %s", ln, text)
		}
		e.printLine(text)
	}
	e.scr.cursor = Pos{Line: clampLine(e.scr, l2), Col: 0}
	return nil
}
