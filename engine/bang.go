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
		e.scr.msg, e.scr.msgKind = "(command completed)", MsgInfo
		return
	}
	e.showOutput(strings.Split(out, "\n"))
	e.fe.Render(e.curView(), ChangeSet{Full: true})
}

// runBangNoRange executes :!cmd without a line range. Output is captured in a
// pty and shown in the editor overlay; nvi runs on the terminal, but govi
// keeps the transcript visible like :viusage.
func (e *Engine) runBangNoRange(cmd string) error {
	if e.scr.opts.Bool("secure") {
		return fmt.Errorf("The ! command is not supported when the secure edit option is set")
	}
	cmd, err := e.expandShellNames(cmd)
	if err != nil {
		return err
	}
	cols, rows := e.bangCols(), e.bangRows()
	e.ensureCwd()
	var out string
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
		return fmt.Errorf("%s: %v", cmd, err)
	}
	return nil
}
