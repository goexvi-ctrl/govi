package engine

import (
	"fmt"
	"strings"
)

// Ex (line-editor) mode, entered from vi with Q and left with :vi / :visual.
// Unlike the vi colon line, ex mode is a scrolling transcript: each command is
// echoed, its output appended, and a new ":" prompt shown. This corresponds to
// nvi's ex.c top-level loop.

// enterExMode switches from vi command mode into ex mode.
func (e *Engine) enterExMode() {
	e.scr.mode = ModeExText
	e.scr.colon = nil
	e.scr.cmdPrefix = ':'
	e.exEcho("Entering Ex mode.  Type \"visual\" to go to Normal mode.")
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

// exEcho appends a line to the ex transcript.
func (e *Engine) exEcho(s string) {
	e.scr.exTranscript = append(e.scr.exTranscript, s)
}

// showOutput presents multi-line command output: appended to the transcript in
// ex mode, or shown as an overlay (dismissed by the next key) in vi mode.
func (e *Engine) showOutput(lines []string) {
	if e.scr.mode == ModeExText {
		for _, l := range lines {
			e.exEcho(l)
		}
		return
	}
	e.scr.pendingOutput = lines
}

// printLine emits a line of command output: to the transcript in ex mode, or to
// the message line in vi mode.
func (e *Engine) printLine(s string) {
	if e.scr.mode == ModeExText {
		e.exEcho(s)
	} else {
		e.scr.msg, e.scr.msgKind = s, MsgInfo
	}
}

// exModeKey handles a keypress while in ex mode.
func (e *Engine) exModeKey(ev KeyEvent) {
	s := e.scr
	switch {
	case ev.Key == KeyEnter || ev.Rune == '\r' || ev.Rune == '\n':
		cmd := string(s.colon)
		s.colon = nil
		e.exEcho(":" + cmd)
		switch strings.TrimSpace(cmd) {
		case "vi", "visual", "vis":
			e.exitExMode()
			return
		}
		if err := e.exExecute(cmd); err != nil {
			e.exEcho(err.Error())
		} else if s.msg != "" {
			e.exEcho(s.msg)
			s.msg = ""
		}
	case ev.Key == KeyBackspace || ev.Rune == 0x7f || ev.Rune == '\b':
		if len(s.colon) > 0 {
			s.colon = s.colon[:len(s.colon)-1]
		}
	default:
		if ev.Rune != 0 {
			s.colon = append(s.colon, ev.Rune)
		}
	}
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
	for ln := l1; ln <= l2; ln++ {
		text := string(e.scr.lineRunes(ln))
		if list {
			text = strings.ReplaceAll(text, "\t", "^I") + "$"
		}
		if number {
			text = fmt.Sprintf("%6d  %s", ln, text)
		}
		e.printLine(text)
	}
	e.scr.cursor = Pos{Line: clampLine(e.scr, l2), Col: 0}
	return nil
}
