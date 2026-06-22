package engine

// view adapts the engine's active screen to the read-only View interface
// handed to Frontend.Render. It holds no state of its own; every method reads
// live screen fields, which are quiescent for the duration of a Render call.
type view struct{ s *screen }

func (v view) LineCount() int64 { return v.s.lineCount() }

func (v view) Line(lno int64) DisplayLine {
	return makeDisplayLine(v.s.lineRunes(lno), v.s.opts.tabstop)
}

func (v view) Cursor() Pos { return v.s.cursor }

func (v view) Mode() Mode { return v.s.mode }

func (v view) Viewport() Viewport {
	return Viewport{Top: v.s.top, Rows: v.s.rows, Cols: v.s.cols}
}

func (v view) Message() (string, MessageKind) {
	// While entering a command line, the message line shows the prompt prefix
	// (':', '/', or '?') followed by what has been typed.
	if v.s.mode == ModeExColon {
		prefix := v.s.cmdPrefix
		if prefix == 0 {
			prefix = ':'
		}
		return string(prefix) + string(v.s.colon), MsgNone
	}
	return v.s.msg, v.s.msgKind
}

func (v view) Name() string { return v.s.name }

func (v view) Modified() bool { return v.s.modified }

func (v view) Number() bool { return v.s.opts.number }
