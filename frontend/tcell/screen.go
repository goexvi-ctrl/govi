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
	"golang.org/x/term"

	"govi/engine"
)

// Frontend renders the engine to a terminal via tcell.
type Frontend struct {
	scr   tc.Screen
	eng   *engine.Engine
	title string

	// inited is set once the tcell screen has been initialized -- raw mode and
	// the alternate screen buffer. It stays false for a session started in ex
	// mode (goex/-e) until the user enters vi mode: ex is a scrolling line
	// interface on the normal screen, and must not clear the terminal. closed
	// is set once the screen has been finalized; nothing may Resume or paint
	// after that (tcell's Fini is once-only, so a stray Resume would leave the
	// tty raw in the alternate screen with no way back).
	inited bool
	closed bool

	inEventBurst bool
	paintPending bool
	paintUrgent  bool // mode/message/overlay: paint immediately mid-burst
	forceSync    bool // ^L/^R: next paint calls Sync() to recover a corrupted tty
	lastPaintAt  time.Time
}

// New returns a Frontend over the terminal. Call Attach with the engine, then
// Run. The screen is initialized lazily, on first entry to vi mode: a session
// that starts and ends in ex mode never touches raw mode or the alternate
// screen buffer.
func New() (*Frontend, error) {
	scr, err := tc.NewScreen()
	if err != nil {
		return nil, err
	}
	return &Frontend{scr: scr}, nil
}

// NewWithScreen builds a Frontend over a caller-provided tcell Screen. It is
// used by tests (with a SimulationScreen) and by hosts that manage their own
// screen. The screen is initialized here, eagerly.
func NewWithScreen(scr tc.Screen) (*Frontend, error) {
	f := &Frontend{scr: scr}
	if err := f.ensureScreen(); err != nil {
		return nil, err
	}
	return f, nil
}

// ensureScreen initializes the tcell screen (raw mode + alternate screen
// buffer) if it is not already running.
func (f *Frontend) ensureScreen() error {
	if f.inited {
		return nil
	}
	// Save cooked termios and install fatal-signal handlers before tcell
	// switches the tty to raw mode. Go's signal.Notify cannot catch synchronous
	// signals (SIGSEGV, SIGBUS, ...); without this, kill(1) leaves the shell raw.
	saveEmergencyTermios()
	installEmergencyHandlers()
	if err := f.scr.Init(); err != nil {
		return err
	}
	f.scr.SetStyle(tc.StyleDefault)
	f.scr.EnablePaste()
	f.inited = true
	return nil
}

// Attach binds the engine the frontend drives.
func (f *Frontend) Attach(e *engine.Engine) { f.eng = e }

// Close restores the terminal. It is a no-op if the screen was never
// initialized (a pure ex-mode session) or is already closed.
func (f *Frontend) Close() {
	if f.scr != nil && f.inited && !f.closed {
		f.closed = true
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
// the full-screen visual loop and, when in ex mode (started as goex/-e, or
// entered with Q), a line-oriented ex REPL on the normal screen buffer
// (matching nvi: ex is a scrolling line interface usable on a paper terminal).
// The tcell screen -- raw mode, alternate buffer -- starts on first entry to
// vi mode, so a pure ex session never disturbs the terminal.
func (f *Frontend) Run() {
	if f.eng.ExActive() {
		// Starting in ex mode: no screen yet, so size the engine (ex output
		// formatting, the window option) from the tty directly.
		w, h := termSize()
		f.eng.Resize(textRows(h), w)
	}

	sigCh := f.installSignals()
	defer signalStop(sigCh)

	for !f.eng.ShouldQuit() {
		if f.eng.ExActive() {
			f.runExMode(sigCh)
			continue
		}
		if err := f.ensureScreen(); err != nil {
			fmt.Fprintf(os.Stderr, "govi: cannot initialize terminal: %v\n", err)
			return
		}
		// Entering (or returning to) vi: repaint the full-screen display.
		f.scr.Sync()
		w, h := f.scr.Size()
		f.eng.Resize(textRows(h), w)
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
	raw := make(chan tc.Event)
	go f.scr.ChannelEvents(raw, quit)
	go f.forwardInterrupts(raw, events)
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

// runExMode runs ex as a cooked-mode line REPL: print the prompt, read a line
// (echoed and line-edited by the terminal), feed it to the engine, and print
// the output, scrolling like a terminal. It returns when the user types
// visual/vi or quits. When entered from vi (Q), the full-screen display is
// suspended around it; when the session started in ex mode there is no screen
// to leave.
func (f *Frontend) runExMode(sigCh <-chan os.Signal) {
	if f.inited {
		if err := f.scr.Suspend(); err != nil {
			return
		}
		fmt.Fprintln(os.Stdout) // separate from the full-screen display we just left
	}
	// Resume the full-screen display only when going back to vi mode. On quit
	// (or a fatal signal) the screen has been finalized -- Fini is once-only,
	// so a Resume here would put the tty back into raw mode and the alternate
	// screen with nothing left to restore it.
	defer func() {
		if f.inited && !f.closed {
			f.scr.Resume()
		}
	}()

	in := bufio.NewReader(os.Stdin)
	out := os.Stdout

	// A message queued before the REPL starts (the file-load line at an
	// ex-mode startup) prints ahead of the first prompt, as nvi's ex does.
	if m, _ := f.eng.TakeMessage(); m != "" {
		fmt.Fprintln(out, m)
	}

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

// forwardInterrupts relays polled tcell events from raw to the main loop's
// channel, but first records the user's ^C out of band via eng.Interrupt(). It
// runs on its own goroutine and, once anything is queued, never blocks solely on
// the outgoing channel: it keeps draining raw via select even while the main
// goroutine is stuck inside a long Engine.Input. That way a ^C is seen -- and
// the interrupt flag set -- immediately, even if typed-ahead events sit in front
// of it. It closes out when raw closes so the main loop still detects shutdown.
func (f *Frontend) forwardInterrupts(raw <-chan tc.Event, out chan<- tc.Event) {
	var queue []tc.Event
	note := func(ev tc.Event) {
		if isInterruptEvent(ev) {
			f.eng.Interrupt()
		}
		queue = append(queue, ev)
	}
	for {
		if len(queue) == 0 {
			ev, ok := <-raw
			if !ok {
				close(out)
				return
			}
			note(ev)
			continue
		}
		select {
		case out <- queue[0]:
			queue = queue[1:]
		case ev, ok := <-raw:
			if !ok {
				close(out) // shutdown: drop any tail, the main loop is exiting
				return
			}
			note(ev)
		}
	}
}

// isInterruptEvent reports whether ev is the user's interrupt: a typed ^C (raw
// mode delivers it as KeyCtrlC, not a signal) or tcell's own interrupt event.
func isInterruptEvent(ev tc.Event) bool {
	switch ev := ev.(type) {
	case *tc.EventKey:
		return ev.Key() == tc.KeyCtrlC
	case *tc.EventInterrupt:
		return true
	}
	return false
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

// termSize returns the tty's size for a session running in ex line mode,
// where no tcell screen exists to ask; falls back to 80x24.
func termSize() (w, h int) {
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 && h > 0 {
		return w, h
	}
	return 80, 24
}

// Bell rings the terminal bell. In ex line mode (no screen) it writes BEL to
// the terminal directly.
func (f *Frontend) Bell() {
	if !f.inited || f.closed {
		os.Stdout.WriteString("\a")
		return
	}
	f.scr.Beep()
}

// SetTitle records the desired window title. (Terminal title-setting is applied
// in a later phase; stored here so the behavior is observable.)
func (f *Frontend) SetTitle(title string) { f.title = title }

// Render paints the current View. During a burst, processEvents schedules
// repaints; urgent changes (mode, message, overlay) paint on the next loop pass.
func (f *Frontend) Render(v engine.View, cs engine.ChangeSet) {
	if cs.Sync {
		// Sticky until the next paint actually happens, so a ^L coalesced into a
		// burst still forces the Sync() when the burst's paint fires.
		f.forceSync = true
	}
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
	if !f.inited || f.closed {
		return // ex line mode (screen never started) or already shut down
	}
	f.scr.Clear()
	w, h := f.scr.Size()

	if v.Mode() == engine.ModeExText {
		f.renderExMode(v, w, h)
		f.present()
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
	f.present()
}

// present flushes the back buffer to the terminal. Normally Show() emits only
// the cells that changed; after ^L/^R (forceSync) Sync() redraws every cell from
// scratch, discarding tcell's model of the on-screen contents so output another
// program wrote to the tty is overwritten.
func (f *Frontend) present() {
	if f.forceSync {
		f.scr.Sync()
	} else {
		f.scr.Show()
	}
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
		// Emit at least one row even for an empty line. The gutter occupies
		// the first row only; continuation rows start at column 0 and span
		// the full width (nvi vs_line: the number is drawn once per line).
		i, first := 0, true
		for (i < len(cells) || first) && row < mapRows {
			w, x := textW, coff+gutter
			if !first {
				w, x = cols, coff
			}
			if gutter > 0 && first {
				f.drawGutter(lno, coff, roff+row, gutter)
			}
			for j := i; j < i+w && j < len(cells); j++ {
				// Continuation cells (Rune == 0) belong to a preceding wide
				// glyph; tcell draws the wide rune spanning both columns, so skip
				// them but still advance the column.
				if cells[j].Rune != 0 {
					f.scr.SetContent(x, roff+row, cells[j].Rune, nil, styleFor(cells[j].Style))
				}
				x++
			}
			i += w
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
		f.placeCursor(sv, coff, roff, statusRow, gutter, cols, mapRows)
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

func (f *Frontend) placeCursor(sv engine.ScreenView, coff, roff, statusRow, gutter, cols, mapRows int) {
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
		y += wrapRowsOf(sv.Line(ln), cols, gutter)
	}
	dx := engine.CursorDisplayColumn(sv.Line(cur.Line), cur.Col, sv.Mode())
	sub, sx := engine.WrapCellPos(dx, cols, gutter)
	y += sub
	x := coff + sx

	if y < 0 || y >= mapRows {
		f.scr.HideCursor()
		return
	}
	f.scr.ShowCursor(x, roff+y)
}

// wrapRowsOf returns how many screen rows a display line occupies at screen
// width cols with a number gutter of g columns (drawn on the first row only).
func wrapRowsOf(dl engine.DisplayLine, cols, g int) int {
	return engine.WrapRowCount(engine.DisplayLineWidth(dl), cols, g)
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
