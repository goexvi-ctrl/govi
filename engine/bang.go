package engine

import (
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

// presentBangOutput shows utility output from a :! command with no line range.
func (e *Engine) presentBangOutput(out string) {
	out = strings.TrimRight(out, "\r\n")
	if out == "" {
		e.scr.msg, e.scr.msgKind = "(command completed)", MsgInfo
		return
	}
	e.showOutput(strings.Split(out, "\n"))
	e.fe.Render(view{e.scr}, ChangeSet{Full: true})
}

// runBangNoRange executes :!cmd without a line range (nvi ex_exec_proc).
func (e *Engine) runBangNoRange(cmd string) error {
	if e.scr.opts.Bool("secure") {
		return fmt.Errorf("The ! command is not supported when the secure edit option is set")
	}
	cols, rows := e.bangCols(), e.bangRows()
	e.ensureCwd()
	if runner, ok := e.fe.(BangRunner); ok {
		out, onTerminal, err := runner.RunBang(e.shellProg(), cmd, e.cwd, cols, rows)
		if err != nil {
			return fmt.Errorf("%s: %v", cmd, err)
		}
		if !onTerminal {
			e.presentBangOutput(out)
		}
		return nil
	}
	out, err := e.runShellCmd(cmd, "", cols, rows)
	if err != nil {
		return fmt.Errorf("%s: %v", cmd, err)
	}
	e.presentBangOutput(out)
	return nil
}