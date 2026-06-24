// Package tcell is a terminal Frontend for the editor engine, built on
// github.com/gdamore/tcell/v2. It renders the engine's semantic View onto a
// character grid and translates terminal key events into engine events. It is
// one consumer of the engine boundary; the engine itself has no terminal
// dependency.
package tcell

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tc "github.com/gdamore/tcell/v2"

	"govi/engine"
)

// Frontend renders the engine to a terminal via tcell.
type Frontend struct {
	scr   tc.Screen
	eng   *engine.Engine
	title string
}

// New initializes a terminal screen and returns a Frontend. Call Attach with
// the engine, then Run.
func New() (*Frontend, error) {
	scr, err := tc.NewScreen()
	if err != nil {
		return nil, err
	}
	return NewWithScreen(scr)
}

// NewWithScreen builds a Frontend over a caller-provided tcell Screen. It is
// used by tests (with a SimulationScreen) and by hosts that manage their own
// screen. The screen is initialized here.
func NewWithScreen(scr tc.Screen) (*Frontend, error) {
	// Save cooked termios and install fatal-signal handlers before tcell
	// switches the tty to raw mode. Go's signal.Notify cannot catch synchronous
	// signals (SIGSEGV, SIGBUS, ...); without this, kill(1) leaves the shell raw.
	saveEmergencyTermios()
	installEmergencyHandlers()
	if err := scr.Init(); err != nil {
		return nil, err
	}
	scr.SetStyle(tc.StyleDefault)
	scr.EnablePaste()
	return &Frontend{scr: scr}, nil
}

// Attach binds the engine the frontend drives.
func (f *Frontend) Attach(e *engine.Engine) { f.eng = e }

// Close restores the terminal.
func (f *Frontend) Close() {
	if f.scr != nil {
		f.scr.Fini()
	}
}

// mapTimeout is how long to wait for the next key before resolving an ambiguous
// map prefix (the 'timeout' behavior).
const mapTimeout = 500 * time.Millisecond

// recoverFlushDelay is how long after the user pauses to flush the recovery
// file with any changes the throttled sync has not yet captured.
const recoverFlushDelay = 2 * time.Second

// Run drives the editor until a quit command is issued. It alternates between
// the full-screen visual loop and, when the user enters ex mode with Q, a
// line-oriented ex REPL that leaves the alternate screen (matching nvi: ex is a
// scrolling line interface usable on a paper terminal).
func (f *Frontend) Run() {
	w, h := f.scr.Size()
	f.eng.Resize(textRows(h), w)

	sigCh := f.installSignals()
	defer signalStop(sigCh)

	for !f.eng.ShouldQuit() {
		if f.eng.ExActive() {
			f.runExMode(sigCh)
			if !f.eng.ShouldQuit() {
				// Returning to vi: repaint the full-screen display.
				f.scr.Sync()
				w, h := f.scr.Size()
				f.eng.Resize(textRows(h), w)
			}
			continue
		}
		if f.runVisual(sigCh) {
			return
		}
	}
}

// runVisual is the full-screen event loop. It returns true when the host should
// exit immediately (fatal signal).
func (f *Frontend) runVisual(sigCh <-chan os.Signal) bool {
	events := make(chan tc.Event)
	quit := make(chan struct{})
	go f.scr.ChannelEvents(events, quit)
	defer func() {
		close(quit)
		// Unblock a PollEvent that may be waiting for a key so the goroutine sees
		// the closed quit channel and exits before we suspend the screen.
		f.scr.PostEvent(tc.NewEventInterrupt(nil))
	}()

	for !f.eng.ShouldQuit() && !f.eng.ExActive() {
		var timer <-chan time.Time
		recoverFlush := false
		switch {
		case f.eng.MatchPending():
			// showmatch flash: matchtime is in tenths of a second.
			timer = time.After(time.Duration(f.eng.MatchTime()) * 100 * time.Millisecond)
		case f.eng.MapPending():
			timer = time.After(mapTimeout)
		case f.eng.NeedsRecoverySync():
			// Flush the recovery file shortly after the user pauses.
			timer = time.After(recoverFlushDelay)
			recoverFlush = true
		}
		select {
		case ev := <-events:
			if ev == nil {
				return false
			}
			f.handleEvent(ev)
		case sig := <-sigCh:
			if f.handleSignal(sig) {
				return true
			}
		case <-timer:
			if recoverFlush {
				f.eng.SyncRecovery()
			} else {
				f.eng.Input(engine.TimeoutEvent{})
			}
		}
	}
	return false
}

// runExMode leaves the full-screen display and runs ex as a cooked-mode line
// REPL: print the prompt, read a line (echoed and line-edited by the terminal),
// feed it to the engine, and print the output, scrolling like a terminal. It
// returns when the user types visual/vi or quits.
func (f *Frontend) runExMode(sigCh <-chan os.Signal) {
	if err := f.scr.Suspend(); err != nil {
		return
	}
	defer f.scr.Resume()

	in := bufio.NewReader(os.Stdin)
	out := os.Stdout
	fmt.Fprintln(out) // separate from the full-screen display we just left

	for f.eng.ExActive() && !f.eng.ShouldQuit() {
		prompt := f.eng.ExPrompt()
		fmt.Fprint(out, prompt)
		type readResult struct {
			line string
			err  error
		}
		readCh := make(chan readResult, 1)
		go func() {
			line, err := in.ReadString('\n')
			readCh <- readResult{line, err}
		}()
		var line string
		var err error
		select {
		case res := <-readCh:
			line, err = res.line, res.err
		case sig := <-sigCh:
			if f.handleSignal(sig) {
				return
			}
			// Non-fatal signal handled; wait for the read to finish or retry.
			res := <-readCh
			line, err = res.line, res.err
		}
		if err != nil { // EOF (^D) or read error: return to visual mode
			fmt.Fprintln(out)
			f.eng.RunEx("visual")
			return
		}
		line = strings.TrimRight(line, "\r\n")

		// A bare <enter> at the ":" prompt steps to the next line. nvi replaces
		// the prompt with that line, so the terminal (which already echoed the
		// newline after ":") is moved back up to overwrite the prompt line; a
		// failed step leaves the ":" and prints the message below it.
		if prompt == ":" && strings.TrimSpace(line) == "" {
			if text, ok := f.eng.ExStep(); ok {
				fmt.Fprintf(out, "\x1b[A\r\x1b[K%s\r\n", text)
			} else {
				fmt.Fprintln(out, text)
			}
			continue
		}

		for _, o := range f.eng.ExFeedLine(line) {
			fmt.Fprintln(out, o)
		}
		if f.eng.ShouldQuit() {
			f.shutdown(f.eng.ExitMessage())
			return
		}
	}
}

func (f *Frontend) handleEvent(ev tc.Event) {
	switch ev := ev.(type) {
	case *tc.EventResize:
		f.scr.Sync()
		w, h := ev.Size()
		f.eng.Resize(textRows(h), w)
	case *tc.EventKey:
		f.eng.Input(translateKey(ev))
	case *tc.EventInterrupt:
		f.eng.Input(engine.InterruptEvent{})
	}
}

// textRows is the number of buffer rows, reserving the bottom row for the
// status/message line.
func textRows(h int) int {
	if h <= 1 {
		return 1
	}
	return h - 1
}

// Bell rings the terminal bell.
func (f *Frontend) Bell() { f.scr.Beep() }

// SetTitle records the desired window title. (Terminal title-setting is applied
// in a later phase; stored here so the behavior is observable.)
func (f *Frontend) SetTitle(title string) { f.title = title }

// Render paints the current View. Phase 2 does a straightforward full repaint;
// the ChangeSet hints are honored for incremental drawing in a later phase.
func (f *Frontend) Render(v engine.View, _ engine.ChangeSet) {
	f.scr.Clear()
	w, h := f.scr.Size()

	if out := v.PendingOutput(); out != nil {
		f.renderOutput(out, v.PendingOutputPrompt(), w, h)
		f.scr.Show()
		return
	}

	if v.Mode() == engine.ModeExText {
		f.renderExMode(v, w, h)
		f.scr.Show()
		return
	}

	rows := textRows(h)
	vp := v.Viewport()
	top := vp.Top
	mapRows := vp.MapRows
	if mapRows <= 0 || mapRows > rows {
		mapRows = rows
	}
	gutter := gutterWidth(v)
	textW := w - gutter
	if textW < 1 {
		textW = 1
	}

	// Draw logical lines from `top` into the active map (nvi t_rows); rows below
	// the map stay blank until j expands the map.
	row := 0
	lno := top
	for row < mapRows && lno <= v.LineCount() {
		cells := engine.DisplayCells(v.Line(lno))
		first := true
		// Emit at least one row even for an empty line.
		for i := 0; (i < len(cells) || first) && row < mapRows; i += textW {
			if gutter > 0 && first {
				f.drawGutter(lno, row, gutter)
			}
			x := gutter
			for j := i; j < i+textW && j < len(cells); j++ {
				// Continuation cells (Rune == 0) belong to a preceding wide
				// glyph; tcell draws the wide rune spanning both columns, so skip
				// them but still advance the column.
				if cells[j].Rune != 0 {
					f.scr.SetContent(x, row, cells[j].Rune, nil, styleFor(cells[j].Style))
				}
				x++
			}
			row++
			first = false
		}
		lno++
	}
	// Tildes for map rows past the end of the buffer.
	for ; row < mapRows; row++ {
		f.scr.SetContent(0, row, '~', nil, tc.StyleDefault)
	}
	// Blank filler below a reduced z[count] map.
	for ; row < rows; row++ {
		for x := 0; x < w; x++ {
			f.scr.SetContent(x, row, ' ', nil, tc.StyleDefault)
		}
	}

	f.drawStatus(v, w, rows)
	f.placeCursor(v, mapRows, rows, gutter, textW)
	f.scr.Show()
}

// renderExMode draws the ex-mode scrolling transcript with the current prompt
// on the bottom line.
func (f *Frontend) renderExMode(v engine.View, w, h int) {
	prompt, _ := v.Message() // ":" + current input
	transcript := v.ExTranscript()

	// Show the tail of the transcript above the prompt line.
	avail := h - 1
	start := 0
	if len(transcript) > avail {
		start = len(transcript) - avail
	}
	row := 0
	for _, line := range transcript[start:] {
		f.drawText(line, row, w)
		row++
	}
	f.drawText(prompt, h-1, w)
	f.scr.ShowCursor(len([]rune(prompt)), h-1)
}

// renderOutput shows one page of command output with a continue prompt on the
// bottom row (nvi msg_cmsg CMSG_CONT_Q / CMSG_CONT_EX).
func (f *Frontend) renderOutput(lines []string, prompt string, w, h int) {
	avail := h - 1
	row := 0
	for i := 0; i < len(lines) && row < avail; i++ {
		f.drawText(lines[i], row, w)
		row++
	}
	if prompt == "" {
		prompt = "Press any key to continue [: to enter more ex commands]: "
	}
	f.drawText(prompt, h-1, w)
	f.scr.HideCursor()
}

func (f *Frontend) drawText(s string, row, w int) {
	x := 0
	for _, r := range s {
		if x >= w {
			break
		}
		f.scr.SetContent(x, row, r, nil, tc.StyleDefault)
		x++
	}
}

// gutterWidth returns the width of the line-number gutter (0 when :set number
// is off), shared with the engine's wrap math.
func gutterWidth(v engine.View) int {
	return engine.GutterWidth(v.LineCount(), v.Number())
}

func (f *Frontend) drawGutter(lno int64, row, gutter int) {
	label := strconv.FormatInt(lno, 10)
	pad := gutter - 1 - len(label)
	x := 0
	for ; x < pad; x++ {
		f.scr.SetContent(x, row, ' ', nil, tc.StyleDefault)
	}
	for _, r := range label {
		f.scr.SetContent(x, row, r, nil, tc.StyleDefault)
		x++
	}
	f.scr.SetContent(x, row, ' ', nil, tc.StyleDefault)
}

func (f *Frontend) drawStatus(v engine.View, w, row int) {
	msg, _ := v.Message()
	st := tc.StyleDefault
	x := 0
	for _, r := range msg {
		if x >= w {
			break
		}
		f.scr.SetContent(x, row, r, nil, st)
		x++
	}
}

func (f *Frontend) placeCursor(v engine.View, mapRows, statusRow, gutter, textW int) {
	if v.Mode() == engine.ModeExColon {
		msg, _ := v.Message()
		f.scr.ShowCursor(len([]rune(msg)), statusRow) // end of the colon line
		return
	}
	cur := v.Cursor()
	if mp, ok := v.MatchHighlight(); ok {
		cur = mp // showmatch: flash the cursor at the matching bracket
	}
	top := v.Viewport().Top

	// Screen row: sum the wrapped row counts of lines [top, cur.Line) then add
	// the cursor's wrapped row within its own line.
	y := 0
	for ln := top; ln < cur.Line; ln++ {
		y += wrapRowsOf(v.Line(ln), textW)
	}
	dx := engine.CursorDisplayColumn(v.Line(cur.Line), cur.Col, v.Mode())
	y += dx / textW
	x := gutter + dx%textW

	if y < 0 || y >= mapRows {
		f.scr.HideCursor()
		return
	}
	f.scr.ShowCursor(x, y)
}

// wrapRowsOf returns how many screen rows a display line occupies at the given
// text width.
func wrapRowsOf(dl engine.DisplayLine, textW int) int {
	w := engine.DisplayLineWidth(dl)
	if w <= 0 {
		return 1
	}
	if textW < 1 {
		textW = 1
	}
	return (w + textW - 1) / textW
}

func styleFor(s engine.Style) tc.Style {
	st := tc.StyleDefault
	if s&engine.StyleReverse != 0 {
		st = st.Reverse(true)
	}
	if s&engine.StyleBold != 0 {
		st = st.Bold(true)
	}
	if s&engine.StyleUnderline != 0 {
		st = st.Underline(true)
	}
	return st
}
