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

	inEventBurst bool
	paintPending bool
	paintUrgent  bool // mode/message/overlay: paint immediately mid-burst
	lastPaintAt  time.Time
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
		// processEvents drains input bursts without selecting on sigCh; signals
		// wait until the burst ends (bursts are short in practice).
		select {
		case ev := <-events:
			if f.processEvents(events, ev) {
				return false // events channel closed
			}
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
		f.inputKey(translateKey(ev))
	case *tc.EventInterrupt:
		f.eng.Input(engine.InterruptEvent{})
	}
}

// inputKey feeds a translated key event to the engine, splitting the Alt-merged
// event tcell synthesizes for a lone ESC that is immediately followed by another
// byte in the same read. tcell's input parser turns "ESC x" into Alt+x (and
// "ESC ^W" into Ctrl-Alt+w); see input.go inpStateEsc. vi has no Meta bindings,
// so nvi resolves such a buffered ESC -- one that does not begin a recognized
// key sequence -- to a plain Escape followed by the next key. Without this, an
// <Esc> arriving in the same read as trailing bytes (scripted or pasted input)
// would never exit insert mode and the trailing byte would be inserted as text.
func (f *Frontend) inputKey(ev engine.Event) {
	if k, ok := ev.(engine.KeyEvent); ok && k.Key == engine.KeyNone && k.Mods&engine.ModAlt != 0 {
		f.eng.Input(engine.KeyEvent{Key: engine.KeyEscape})
		f.eng.Input(engine.KeyEvent{Rune: k.Rune, Mods: k.Mods &^ engine.ModAlt})
		return
	}
	f.eng.Input(ev)
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

// Render paints the current View. During a burst, processEvents schedules
// repaints; urgent changes (mode, message, overlay) paint on the next loop pass.
func (f *Frontend) Render(v engine.View, cs engine.ChangeSet) {
	if f.inEventBurst {
		f.paintPending = true
		if renderUrgent(v, cs) {
			f.paintUrgent = true
		}
		return
	}
	f.ensurePainted()
}

// paintNow performs a full-screen repaint. Phase 2 always clears and redraws;
// ChangeSet incremental drawing is a later phase. With split screens it draws
// each pane in its own row/column band, every pane carrying its own status line
// (reverse video as a divider when split); the cursor goes in the active pane.
func (f *Frontend) paintNow(v engine.View) {
	f.scr.Clear()
	w, h := f.scr.Size()

	if v.Mode() == engine.ModeExText {
		f.renderExMode(v, w, h)
		f.scr.Show()
		return
	}

	split := v.Split()
	for _, sv := range v.Screens() {
		f.paintScreen(sv, split, w)
	}

	// An ex-output overlay (e.g. :p, :set all) is drawn over the bottom of the
	// buffer: a "+=+=" divider, the output lines, and a continue prompt on the
	// last row -- the buffer stays visible above (nvi vs_msg).
	if out := v.PendingOutput(); out != nil {
		f.renderOutput(out, v.PendingOutputPrompt(), v.PendingOutputFirst(), w, h)
	}
	f.scr.Show()
}

// paintScreen draws one screen (split pane): its text band [roff, roff+rows),
// its status/colon/message line at roff+rows, and the cursor when it is the
// active pane. split selects the reverse-video status divider.
func (f *Frontend) paintScreen(sv engine.ScreenView, split bool, termW int) {
	roff := sv.Roff()
	coff := sv.Coff()
	rows := sv.Rows()
	cols := sv.Cols()
	statusRow := roff + rows

	gutter := engine.GutterWidth(sv.LineCount(), sv.Number())
	textW := cols - gutter
	if textW < 1 {
		textW = 1
	}
	vp := sv.Viewport()
	top := vp.Top
	mapRows := vp.MapRows
	if mapRows <= 0 || mapRows > rows {
		mapRows = rows
	}

	// Draw logical lines from `top` into the active map (nvi t_rows); rows below
	// the map stay blank until j expands the map.
	row := 0
	lno := top
	for row < mapRows && lno <= sv.LineCount() {
		cells := engine.DisplayCells(sv.Line(lno))
		first := true
		// Emit at least one row even for an empty line.
		for i := 0; (i < len(cells) || first) && row < mapRows; i += textW {
			if gutter > 0 && first {
				f.drawGutter(lno, coff, roff+row, gutter)
			}
			x := coff + gutter
			for j := i; j < i+textW && j < len(cells); j++ {
				// Continuation cells (Rune == 0) belong to a preceding wide
				// glyph; tcell draws the wide rune spanning both columns, so skip
				// them but still advance the column.
				if cells[j].Rune != 0 {
					f.scr.SetContent(x, roff+row, cells[j].Rune, nil, styleFor(cells[j].Style))
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
		f.scr.SetContent(coff, roff+row, '~', nil, tc.StyleDefault)
	}
	// Blank filler below a reduced z[count] map.
	for ; row < rows; row++ {
		for x := 0; x < cols; x++ {
			f.scr.SetContent(coff+x, roff+row, ' ', nil, tc.StyleDefault)
		}
	}

	f.drawStatus(sv, coff, statusRow, cols, split)

	// Vertical-split divider: a '|' in the sacrificed column to the right of this
	// pane, on the text rows only (nvi vs_vsplit). A full-width screen ends at the
	// terminal edge and gets none.
	if coff+cols < termW {
		for r := roff; r < statusRow; r++ {
			f.scr.SetContent(coff+cols, r, '|', nil, tc.StyleDefault)
		}
	}

	if sv.Active() {
		f.placeCursor(sv, coff, roff, statusRow, gutter, textW, mapRows)
	}
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

// divideStr is nvi's ex-output divider (vs_divider DIVIDESTR), truncated to the
// screen width.
const divideStr = "+=+=+=+=+=+=+=+"

// renderOutput overlays one page of command output at the bottom of the screen:
// a divider line (only where the output begins), the lines, and a continue
// prompt on the last row (nvi vs_msg / vs_divider). The buffer drawn above stays
// visible; the block is anchored to the bottom.
func (f *Frontend) renderOutput(lines []string, prompt string, first bool, w, h int) {
	n := len(lines)
	sep := 0
	if first {
		sep = 1
	}
	top := h - (n + sep + 1)
	if top < 0 {
		top = 0
	}
	// Clear the block rows, then draw divider / lines / prompt.
	for r := top; r < h; r++ {
		for x := 0; x < w; x++ {
			f.scr.SetContent(x, r, ' ', nil, tc.StyleDefault)
		}
	}
	row := top
	if first {
		d := divideStr
		if len(d) > w {
			d = d[:w]
		}
		f.drawText(d, row, w)
		row++
	}
	for i := 0; i < n && row < h-1; i++ {
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

func (f *Frontend) drawGutter(lno int64, coff, row, gutter int) {
	label := strconv.FormatInt(lno, 10)
	pad := gutter - 1 - len(label)
	x := coff
	for i := 0; i < pad; i++ {
		f.scr.SetContent(x, row, ' ', nil, tc.StyleDefault)
		x++
	}
	for _, r := range label {
		f.scr.SetContent(x, row, r, nil, tc.StyleDefault)
		x++
	}
	f.scr.SetContent(x, row, ' ', nil, tc.StyleDefault)
}

// drawStatus draws a screen's status/colon/message line at (coff, row), cols
// wide. In a split the line is the inter-screen divider: its text is drawn in
// reverse video (standout) while the trailing pad stays normal, matching nvi's
// vs_modeline (standout text + clrtoeol). The blank divider column between
// vertically split screens is left untouched.
func (f *Frontend) drawStatus(sv engine.ScreenView, coff, row, cols int, split bool) {
	msg, _ := sv.Message()
	rs := []rune(msg)
	for x := 0; x < cols; x++ {
		st := tc.StyleDefault
		r := ' '
		if x < len(rs) {
			r = rs[x]
			if split {
				st = st.Reverse(true)
			}
		}
		f.scr.SetContent(coff+x, row, r, nil, st)
	}
}

func (f *Frontend) placeCursor(sv engine.ScreenView, coff, roff, statusRow, gutter, textW, mapRows int) {
	if sv.Mode() == engine.ModeExColon {
		msg, _ := sv.Message()
		f.scr.ShowCursor(coff+engine.DisplayStringColumns(msg, 8), statusRow) // end of the colon line
		return
	}
	cur := sv.Cursor()
	if mp, ok := sv.MatchHighlight(); ok {
		cur = mp // showmatch: flash the cursor at the matching bracket
	}
	top := sv.Viewport().Top

	// Screen row within the pane: sum the wrapped row counts of lines
	// [top, cur.Line) then add the cursor's wrapped row within its own line.
	y := 0
	for ln := top; ln < cur.Line; ln++ {
		y += wrapRowsOf(sv.Line(ln), textW)
	}
	dx := engine.CursorDisplayColumn(sv.Line(cur.Line), cur.Col, sv.Mode())
	y += dx / textW
	x := coff + gutter + dx%textW

	if y < 0 || y >= mapRows {
		f.scr.HideCursor()
		return
	}
	f.scr.ShowCursor(x, roff+y)
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
