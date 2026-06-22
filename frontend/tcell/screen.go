// Package tcell is a terminal Frontend for the editor engine, built on
// github.com/gdamore/tcell/v2. It renders the engine's semantic View onto a
// character grid and translates terminal key events into engine events. It is
// one consumer of the engine boundary; the engine itself has no terminal
// dependency.
package tcell

import (
	"strconv"
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

// Run feeds terminal events to the engine until a quit command is issued. When
// an ambiguous map prefix is pending it arms a timer so the prefix resolves
// instead of hanging.
func (f *Frontend) Run() {
	w, h := f.scr.Size()
	f.eng.Resize(textRows(h), w)

	events := make(chan tc.Event)
	quit := make(chan struct{})
	go f.scr.ChannelEvents(events, quit)
	defer close(quit)

	for !f.eng.ShouldQuit() {
		var timer <-chan time.Time
		if f.eng.MatchPending() {
			// showmatch flash: matchtime is in tenths of a second.
			timer = time.After(time.Duration(f.eng.MatchTime()) * 100 * time.Millisecond)
		} else if f.eng.MapPending() {
			timer = time.After(mapTimeout)
		}
		select {
		case ev := <-events:
			if ev == nil {
				return
			}
			f.handleEvent(ev)
		case <-timer:
			f.eng.Input(engine.TimeoutEvent{})
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
		f.renderOutput(out, w, h)
		f.scr.Show()
		return
	}

	if v.Mode() == engine.ModeExText {
		f.renderExMode(v, w, h)
		f.scr.Show()
		return
	}

	rows := textRows(h)
	top := v.Viewport().Top
	gutter := gutterWidth(v)
	textW := w - gutter
	if textW < 1 {
		textW = 1
	}

	// Draw logical lines from `top`, wrapping each onto continuation rows until
	// the screen is full.
	row := 0
	lno := top
	for row < rows && lno <= v.LineCount() {
		cells := engine.DisplayCells(v.Line(lno))
		first := true
		// Emit at least one row even for an empty line.
		for i := 0; (i < len(cells) || first) && row < rows; i += textW {
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
	// Tildes for rows past the end of the buffer.
	for ; row < rows; row++ {
		f.scr.SetContent(0, row, '~', nil, tc.StyleDefault)
	}

	f.drawStatus(v, w, rows)
	f.placeCursor(v, rows, gutter, textW)
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

// renderOutput shows multi-line command output (e.g. :set all), with a continue
// prompt on the bottom row. If the output exceeds the screen it shows the tail.
func (f *Frontend) renderOutput(lines []string, w, h int) {
	avail := h - 1
	start := 0
	if len(lines) > avail {
		start = len(lines) - avail
	}
	row := 0
	for _, line := range lines[start:] {
		f.drawText(line, row, w)
		row++
	}
	f.drawText("[Press any key to continue]", h-1, w)
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

func (f *Frontend) placeCursor(v engine.View, rows, gutter, textW int) {
	if v.Mode() == engine.ModeExColon {
		msg, _ := v.Message()
		f.scr.ShowCursor(len([]rune(msg)), rows) // end of the colon line
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
	dx := engine.DisplayColumn(v.Line(cur.Line), cur.Col)
	y += dx / textW
	x := gutter + dx%textW

	if y < 0 || y >= rows {
		f.scr.HideCursor()
		return
	}
	f.scr.ShowCursor(x, y)
}

// wrapRowsOf returns how many screen rows a display line occupies at the given
// text width.
func wrapRowsOf(dl engine.DisplayLine, textW int) int {
	w := 0
	for _, n := range dl.Widths {
		w += int(n)
	}
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
