package engine

// LaunchContext is an optional startup override a host may supply before
// LoadStartup: a working directory plus EXINIT/NEXINIT text and exrc paths to
// use instead of reading the process environment. A zero value means "resolve
// startup from the real environment and files," which is what both frontends use
// today (GoVi.app passes the cwd separately via GoviSetCwd and lets LoadStartup
// resolve exrc itself).
type LaunchContext struct {
	Cwd      string // invocation directory for ./.nexrc / ./.exrc
	Silent   bool   // -s: skip all startup
	Nexinit  string // if set, used instead of NEXINIT
	Exinit   string // if set (and Nexinit empty), used instead of EXINIT
	SysExrc  string // explicit /etc/vi.exrc path
	HomeExrc string // explicit $HOME/.nexrc or .exrc path
}

// SetLaunchContext supplies a startup override for the next LoadStartup call.
func (e *Engine) SetLaunchContext(ctx LaunchContext) {
	e.launchCtx = ctx
}
