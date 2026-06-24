package engine

// view adapts the engine's active screen to the read-only View interface
// handed to Frontend.Render. It holds no state of its own; every method reads
// live screen fields, which are quiescent for the duration of a Render call.
type view struct{ s *screen }

func (v view) LineCount() int64 { return v.s.lineCount() }

func (v view) Line(lno int64) DisplayLine {
	return makeDisplayLine(v.s.lineRunes(lno), v.s.opts.Int("tabstop"), v.s.opts.Bool("list"))
}

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
		return v.s.msg, v.s.msgKind
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

func (v view) MatchHighlight() (Pos, bool) { return v.s.matchPos, v.s.matchActive }
