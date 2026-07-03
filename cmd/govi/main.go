// Command govi is the terminal entry point for the govi editor: it wires the
// embeddable engine to the tcell terminal frontend. The engine carries no
// terminal dependency; this command is just one host for it.
//
// Invoked as ex, nex, or goex (or with -e), the session starts in ex mode
// instead of vi mode, following nvi's program-name convention; -v forces vi
// mode back on, and view/nview/goview start with the readonly option set. In
// ex mode, -s (or a redirected stdin) selects nvi's batch mode: the ex script
// on stdin runs headlessly with no terminal.
//
// With -g (or -G, which also waits for the files to be closed), govi instead
// opens the named files in the GoVi.app macOS GUI (see launch_darwin.go) and
// does not start the terminal editor.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/pborman/options"
	"golang.org/x/term"

	"govi/engine"
	tcellfe "govi/frontend/tcell"
)

// cliOptions declares govi's flags for pborman/options, giving traditional
// getopt(3) behavior: short flags combine (-Gs), -- ends option parsing, and
// parsing stops at the first non-option argument. The set matches nvi's
// (common/main.c) except -s, which nvi spells as ex batch mode and govi as
// skip-startup, plus the -g/-G GUI extensions.
type cliOptions struct {
	Help      bool     `getopt:"--help -h display this help and exit"`
	Commands  []string `getopt:"-c=command run the ex command once the file is loaded (+command also works)"`
	Ex        bool     `getopt:"-e start in ex mode (as if invoked as ex)"`
	NoSnap    bool     `getopt:"-F historic no-snapshot option; not supported"`
	GUI       bool     `getopt:"-g open the files in the GoVi.app GUI instead of the terminal"`
	GUIWait   bool     `getopt:"-G like -g, and block until the tabs/windows for these files are closed"`
	Lisp      bool     `getopt:"-l set the showmatch option (and lisp, which is inert here as in nvi)"`
	Readonly  bool     `getopt:"-R set the readonly option (also: invoked as view/nview/goview)"`
	Recover   bool     `getopt:"-r recover the named file from a recovery file"`
	NoStartup bool     `getopt:"-n do not read startup files or EXINIT/NEXINIT"`
	Secure    bool     `getopt:"-S set the secure option: shell access disabled"`
	Silent    bool     `getopt:"-s ex batch mode: run the ex commands on standard input (ex only; implied when stdin is not a terminal)"`
	Tag       string   `getopt:"-t=tag start editing at the tag"`
	Vi        bool     `getopt:"-v start in vi mode (overrides an ex program name; wins over -e)"`
	Window    int      `getopt:"-w=size set the window option to size lines"`
}

// editorHost is the terminal frontend run() drives. Tests substitute a
// simulation screen via newEditorFrontend.
type editorHost interface {
	engine.Frontend
	Close()
	Attach(*engine.Engine)
	Run()
}

var (
	newEditorFrontend = func() (editorHost, error) {
		return tcellfe.New()
	}
	runEditor = func(fe editorHost) {
		fe.Run()
	}
	launchGUI = runGUI
)

func main() {
	// Editing is bursty: idle most of the time, then a flurry of allocation
	// during a big command (e.g. :%s over a large file). Raising the GC target
	// trades a little peak memory for noticeably less GC work during those
	// bursts. Respect an explicit GOGC (a user editing huge files may want to
	// cap memory by lowering it).
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(200)
	}
	os.Exit(run(os.Args[1:]))
}

// run is the real entry point; it returns an exit code for main (and tests).
func run(args []string) int {
	scripted := !term.IsTerminal(int(os.Stdin.Fd()))
	return runIO(filepath.Base(os.Args[0]), args, scripted, os.Stdin, os.Stdout, os.Stderr)
}

// exProgname reports whether the program name selects ex mode, the way nvi
// keys SC_EX off argv[0] being ex or nex; goex is govi's spelling.
func exProgname(progname string) bool {
	return progname == "ex" || progname == "nex" || progname == "goex"
}

// viewProgname reports whether the program name selects the readonly option,
// as nvi does for view and nview; goview is govi's spelling.
func viewProgname(progname string) bool {
	return progname == "view" || progname == "nview" || progname == "goview"
}

// obsArgs rewrites historic vi arguments getopt cannot parse (nvi
// v_obsolete): "+" becomes -c$ (start at the last line) and "+cmd" becomes
// -ccmd, and a bare "-" becomes -s (historic ex batch mode). Rewriting stops
// at "--" so a file named "+foo" stays reachable, and skips the argument of
// -c/-t/-w so it is not mistaken for a +command.
func obsArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return append(out, args[i:]...)
		case a == "-":
			out = append(out, "-s")
		case a == "+":
			out = append(out, "-c$")
		case strings.HasPrefix(a, "+"):
			out = append(out, "-c"+a[1:])
		default:
			out = append(out, a)
			if (a == "-c" || a == "-t" || a == "-w") && i+1 < len(args) {
				i++
				out = append(out, args[i])
			}
		}
	}
	return out
}

func runIO(progname string, args []string, scripted bool, stdin io.Reader, stdout, stderr io.Writer) int {
	vopts, set := options.RegisterNew(progname, &cliOptions{})
	opts := vopts.(*cliOptions)
	set.SetParameters("[file ...]")
	if err := set.Getopt(append([]string{progname}, obsArgs(args)...), nil); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", progname, err)
		set.PrintUsage(stderr)
		return 2
	}
	if opts.Help {
		set.PrintUsage(stdout)
		return 0
	}
	if opts.NoSnap {
		fmt.Fprintf(stderr, "%s: -F option no longer supported.\n", progname)
	}
	if len(opts.Commands) > 1 {
		fmt.Fprintf(stderr, "%s: only one -c command may be specified.\n", progname)
		return 2
	}
	if opts.Recover && opts.Tag != "" {
		fmt.Fprintf(stderr, "%s: only one of -r and -t may be specified.\n", progname)
		return 2
	}

	if opts.GUI || opts.GUIWait {
		return launchGUI(opts.NoStartup, opts.GUIWait, set.Args())
	}

	// Ex batch mode (nvi -s): the session is an ex script on stdin, run with
	// no terminal at all. As in nvi, redirected stdin implies it in ex mode,
	// and asking for it in vi mode is an error.
	exMode := (opts.Ex || exProgname(progname)) && !opts.Vi
	if opts.Silent && !exMode {
		fmt.Fprintf(stderr, "%s: -s option is only applicable to ex.\n", progname)
		return 1
	}
	batch := exMode && (opts.Silent || scripted)

	var fe editorHost
	if batch {
		fe = nullHost{}
	} else {
		var err error
		fe, err = newEditorFrontend()
		if err != nil {
			fmt.Fprintf(stderr, "govi: cannot initialize terminal: %v\n", err)
			return 1
		}
	}
	defer fe.Close()

	eng := engine.New(fe, engine.Options{})
	fe.Attach(eng)
	defer eng.Close()

	// Option-setting flags apply before the startup files are read, matching
	// nvi's opts_init-then-exrc order. The option names are fixed, so these
	// cannot fail.
	if opts.Lisp {
		_ = eng.RunEx("set lisp showmatch")
	}
	if opts.Readonly || viewProgname(progname) {
		_ = eng.RunEx("set readonly")
	}
	if opts.Secure {
		_ = eng.RunEx("set secure")
	}
	if opts.Window > 0 {
		eng.SetStartupWindow(opts.Window)
	}

	if !opts.NoStartup && !batch {
		if err := eng.LoadStartup(); err != nil {
			fe.Close()
			fmt.Fprintf(stderr, "govi: %v\n", err)
			return 1
		}
		if eng.ShouldQuit() {
			return 0
		}
	}

	openArgs := set.Args()
	if opts.Recover {
		if len(openArgs) == 0 {
			fe.Close()
			entries, err := eng.ListRecoverable()
			if err != nil {
				fmt.Fprintf(stderr, "govi: %v\n", err)
				return 1
			}
			if len(entries) == 0 {
				fmt.Fprintln(stdout, "govi: No files to recover")
				return 0
			}
			for _, ent := range entries {
				fmt.Fprintf(stdout, "%s: %s\n", ent.Mtime.Format(time.ANSIC), ent.Orig)
			}
			return 0
		}
		if err := eng.Recover(openArgs[0]); err != nil {
			fe.Close()
			fmt.Fprintf(stderr, "govi: %v\n", err)
			return 1
		}
	} else if len(openArgs) == 0 {
		// No file: edit a throwaway temp file like nvi -- $TMPDIR/vi.XXXXXX,
		// removed on exit. :w with no name writes it; quitting discards it.
		tmp, err := os.CreateTemp("", "vi.")
		if err != nil {
			fe.Close()
			fmt.Fprintf(stderr, "govi: %v\n", err)
			return 1
		}
		tmpPath := tmp.Name()
		tmp.Close()
		defer os.Remove(tmpPath)
		if err := eng.Open(tmpPath); err != nil {
			fe.Close()
			fmt.Fprintf(stderr, "govi: %v\n", err)
			return 1
		}
		eng.SetTemporary()
	} else if err := eng.OpenArgs(openArgs); err != nil {
		fe.Close()
		fmt.Fprintf(stderr, "govi: %v\n", err)
		return 1
	}

	// nvi order: the -t tag is looked up first, then the -c/+cmd command runs
	// (c_option). Either may quit the editor (e.g. -c wq). Their errors show
	// on the status line, not stderr, since the screen is about to start.
	if opts.Tag != "" {
		eng.RunStartupEx("tag " + opts.Tag)
	}
	for _, cmd := range opts.Commands {
		eng.RunStartupEx(cmd)
	}
	if eng.ShouldQuit() {
		return 0
	}

	if exMode {
		eng.EnterEx()
	}
	if batch {
		return runBatch(eng, stdin, stdout)
	}

	runEditor(fe)
	return 0
}

// nullHost is the editor host for ex batch mode: there is no terminal, and
// all output flows through ExFeedLine's return values.
type nullHost struct{}

func (nullHost) Render(engine.View, engine.ChangeSet) {}
func (nullHost) Bell()                                {}
func (nullHost) SetTitle(string)                      {}
func (nullHost) Close()                               {}
func (nullHost) Attach(*engine.Engine)                {}
func (nullHost) Run()                                 {}

// runBatch drives ex mode from a script: each stdin line is an ex command (or
// a/i/c input text), explicit output goes to stdout with no prompts and no
// informative messages, and the first failing command aborts the script with
// exit 1 -- nvi's -s batch mode. EOF without :q simply ends the session.
func runBatch(eng *engine.Engine, stdin io.Reader, stdout io.Writer) int {
	sc := bufio.NewScanner(stdin)
	for lineno := 1; !eng.ShouldQuit() && sc.Scan(); lineno++ {
		out, err := eng.ExBatchLine(sc.Text())
		for _, o := range out {
			fmt.Fprintln(stdout, o)
		}
		if err != nil {
			fmt.Fprintf(stdout, "script, %d: %v\n", lineno, err)
			return 1
		}
	}
	return 0
}
