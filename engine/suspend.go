package engine

import "fmt"

// doSuspend job-control suspends the editor session (^Z, :suspend, :stop).
// When force is false and autowrite is set, a modified buffer is written first.
func (e *Engine) doSuspend(force bool) error {
	if e.scr.opts.Bool("secure") {
		return fmt.Errorf("Suspend is not permitted in secure mode")
	}
	if !force && e.scr.opts.Bool("autowrite") && e.scr.dirty() {
		if err := e.Save(""); err != nil {
			return err
		}
	}
	sp, ok := e.fe.(Suspender)
	if !ok {
		return fmt.Errorf("Suspend not supported")
	}
	return sp.Suspend()
}

// exStop implements :stop and :suspend (ex/ex_stop.c).
func (e *Engine) exStop(c *exCmd) error {
	return e.doSuspend(c.force)
}
