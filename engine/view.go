package engine

import (
	"strconv"
	"strings"
)

// view adapts one screen to the read-only View interface handed to
// Frontend.Render. It holds no state of its own; every method reads live screen
// fields, which are quiescent for the duration of a Render call. A lone screen
// renders through `view` directly; a split renders through renderView, which
// embeds the active screen's view and adds the full screen list.
type view struct{ s *screen }

func (v view) LineCount() int64 { return v.s.lineCount() }

func (v view) Line(lno int64) DisplayLine { return v.s.displayLine(lno) }

func (v view) Cursor() Pos { return v.s.cursor }

func (v view) Mode() Mode { return v.s.mode }

func (v view) Viewport() Viewport {
	return Viewport{Top: v.s.top, Rows: v.s.rows, MapRows: v.s.mapRows, Cols: v.s.cols}
}

func (v view) Message() (string, MessageKind) {
	// While entering a command line, the message line shows the prompt prefix
	// (':', '/', or '?') followed by what has been typed.
	if v.s.mode == ModeExColon || v.s.mode == ModeExText {
		return v.colonDisplayMessage(), MsgNone
	}
	if v.s.msg != "" {
		return fitStatus(v.s.msg, v.s.statusCols()), v.s.msgKind
	}
	return v.statusLine(), MsgNone
}

func (v view) Name() string { return v.s.name }

func (v view) Modified() bool { return v.s.dirty() }

func (v view) Number() bool { return v.s.opts.Bool("number") }

func (v view) List() bool { return v.s.opts.Bool("list") }

func (v view) ExTranscript() []string { return v.s.exTranscript }

func (v view) PendingOutput() []string { return v.s.pendingPageLines() }

func (v view) PendingOutputPrompt() string { return v.s.pendingOutputPrompt() }

func (v view) PendingOutputFirst() bool { return v.s.pendingOutputFirst() }

func (v view) MatchHighlight() (Pos, bool) { return v.s.matchPos, v.s.matchActive }

// Split reports whether more than one screen is displayed. A lone `view` is
// never split; renderView overrides this.
func (v view) Split() bool { return false }

// Screens returns the single screen as one active pane. renderView overrides
// this to enumerate every split screen.
func (v view) Screens() []ScreenView {
	return []ScreenView{screenView{view: v, active: true, split: false}}
}

// renderView is the View over a full set of displayed (split) screens, with the
// active screen highlighted. It embeds the active screen's view so all the
// single-screen accessors report the active screen, and overrides Split/Screens
// to expose the whole layout.
type renderView struct {
	view
	all []*screen
	cur int
}

func (rv renderView) Split() bool { return len(rv.all) > 1 }

func (rv renderView) Screens() []ScreenView {
	if len(rv.all) <= 1 {
		return rv.view.Screens()
	}
	out := make([]ScreenView, len(rv.all))
	for i, s := range rv.all {
		out[i] = screenView{view: view{s: s}, active: i == rv.cur, split: true}
	}
	return out
}

// screenView is one pane within a split: a per-screen view plus its geometry,
// active flag, and whether the display is split (so its status line shows the
// reverse-video modeline divider rather than the single-screen status line).
type screenView struct {
	view
	active bool
	split  bool
}

func (sv screenView) Roff() int    { return sv.s.roff }
func (sv screenView) Coff() int    { return sv.s.coff }
func (sv screenView) Rows() int    { return sv.s.rows }
func (sv screenView) Cols() int    { return sv.s.cols }
func (sv screenView) Active() bool { return sv.active }

// Message for a split pane shows the screen's transient message or colon input,
// otherwise the modeline ("name: unmodified: line N", nvi msgq_status without
// MSTAT_SHOWLAST) that serves as the inter-screen divider.
func (sv screenView) Message() (string, MessageKind) {
	if !sv.split {
		return sv.view.Message()
	}
	if sv.s.mode == ModeExColon || sv.s.mode == ModeExText {
		return sv.colonDisplayMessage(), MsgNone
	}
	if sv.s.msg != "" {
		return fitStatus(sv.s.msg, sv.s.statusCols()), sv.s.msgKind
	}
	return fitStatus(sv.modeline(), sv.s.statusCols()), MsgNone
}

// modeline builds nvi's persistent per-screen status divider (vi/vs_refresh.c
// vs_modeline): the file's base name (capped at cols/2 with a "... " ellipsis),
// plus the ruler (centered) and showmode/dirty flag (right) when those options
// are set. The transient "name: ...: line N" form (msgq_status) is shown only
// right after a split or screen switch -- see Engine.screenStatusMsg.
func (sv screenView) modeline() string {
	s := sv.s
	cols := s.cols
	if cols < 1 {
		cols = 80
	}
	left := splitModelineName(s.name, cols/2)
	o := &s.opts
	if !o.Bool("ruler") && !o.Bool("showmode") {
		return left
	}
	ruler := ""
	if o.Bool("ruler") {
		dl := sv.Line(s.cursor.Line)
		col := DisplayColumn(dl, s.cursor.Col) + 1
		ruler = strconv.FormatInt(s.cursor.Line, 10) + "," + strconv.Itoa(col)
	}
	suffix := ""
	if o.Bool("showmode") {
		if s.dirty() {
			suffix = "*"
		}
		suffix += s.showModeLabel
	}
	return layoutModeline(cols-1, left, ruler, suffix)
}

// splitModelineName returns the file's base name (the part after the last '/')
// capped to maxCols display columns, prefixed with "... " when it had to be
// shortened, mirroring vs_modeline's backward scan.
func splitModelineName(name string, maxCols int) string {
	if maxCols < 1 {
		maxCols = 1
	}
	r := []rune(name)
	start := 0
	for i := len(r) - 1; i >= 0; i-- {
		if r[i] == '/' {
			start = i + 1
			break
		}
	}
	base := r[start:]
	if len(base) <= maxCols {
		return string(base)
	}
	keep := maxCols - 4 // room for "... "
	if keep < 0 {
		keep = 0
	}
	return "... " + string(base[len(base)-keep:])
}

// layoutModeline places left (file name) at the start, ruler near the middle,
// and suffix (showmode) at the end of a max-wide modeline (vs_modeline layout).
func layoutModeline(max int, left, ruler, suffix string) string {
	if max < 1 {
		return ""
	}
	buf := make([]rune, max)
	for i := range buf {
		buf[i] = ' '
	}
	leftLen := copy(buf, []rune(left))
	if ruler != "" {
		rr := []rune(ruler)
		midpoint := (max - (len(rr)+1)/2) / 2
		pos := -1
		if leftLen < midpoint && midpoint+len(rr) <= max {
			pos = midpoint
		} else if leftLen+2+len(rr) <= max {
			pos = leftLen + 2
		}
		if pos >= 0 {
			copy(buf[pos:], rr)
			leftLen = pos + len(rr)
		}
	}
	if suffix != "" {
		sr := []rune(suffix)
		endpoint := max - len(sr)
		if endpoint >= leftLen+2 {
			copy(buf[endpoint:], sr)
		}
	}
	return strings.TrimRight(string(buf), " ")
}
