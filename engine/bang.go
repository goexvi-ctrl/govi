package engine

import (
	"errors"
	"fmt"
	"strings"
)

func (e *Engine) bangCols() int {
	if e.scr.cols > 0 {
		return e.scr.cols
	}
	if c := e.scr.opts.Int("columns"); c > 0 {
		return c
	}
	return 80
}

func (e *Engine) bangRows() int {
	if e.scr.rows > 0 {
		return e.scr.rows
	}
	if r := e.scr.opts.Int("lines"); r > 0 {
		return r
	}
	return 24
}

// presentBangOutput shows utility output from a :! command with no line range
// in the paged overlay (same mechanism as :viusage).
func (e *Engine) presentBangOutput(out string) {
	out = strings.ReplaceAll(out, "\r\n", "\n")
	out = strings.ReplaceAll(out, "\r", "\n")
	out = strings.TrimRight(out, "\n")
	if out == "" {
		// Ex mode signals completion with its closing "!" instead (nvi).
		if e.scr.mode != ModeExText {
			e.scr.msg, e.scr.msgKind = "(command completed)", MsgInfo
		}
		return
	}
	e.showOutput(strings.Split(out, "\n"))
	if e.scr.mode == ModeExText {
		// Ex line mode: showOutput queued the lines for the line host to print;
		// the full-screen display is suspended (or never started) and must not
		// be painted.
		return
	}
	e.fe.Render(e.curView(), ChangeSet{Full: true})
}

// runBangNoRange executes :!cmd without a line range. Output is captured in a
// pty and shown in the editor overlay; nvi runs on the terminal, but govi
// keeps the transcript visible like :viusage. The command must already be
// bang-expanded (prepBang, done by exBang).
func (e *Engine) runBangNoRange(cmd string) error {
	if e.scr.opts.Bool("secure") {
		return fmt.Errorf("The ! command is not supported when the secure edit option is set")
	}
	// nvi ex_bang: a modified file is written back first when autowrite is
	// set; otherwise the warn option announces it before the command runs.
	if e.scr.dirty() {
		if e.scr.opts.Bool("autowrite") {
			if err := e.Save(""); err != nil {
				return err
			}
		} else if e.scr.opts.Bool("warn") {
			e.bangEcho("File modified since last write.")
		}
	}
	cols, rows := e.bangCols(), e.bangRows()
	e.ensureCwd()
	var out string
	var err error
	if runner, ok := e.fe.(BangRunner); ok {
		out, err = runner.RunBang(e.shellProg(), cmd, e.cwd, cols, rows)
	} else {
		out, err = e.runBangPTY(e.shellProg(), cmd, e.cwd, cols, rows)
	}
	if errors.Is(err, errInterrupted) {
		return err // ^C: no partial transcript, report "Interrupted"
	}
	e.presentBangOutput(out)
	if err != nil {
		e.bangError(cmd, err) // a message, not a command error (nvi ex_bang)
	}
	return nil
}
