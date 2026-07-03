// Command govi is the terminal entry point for the govi editor: it wires the
// embeddable engine to the tcell terminal frontend. The engine carries no
// terminal dependency; this command is just one host for it.
//
// Invoked as ex, nex, or goex (or with -e), the session starts in ex mode
// instead of vi mode, following nvi's program-name convention; -v forces vi
// mode back on.
//
// With -g (or -G, which also waits for the files to be closed), govi instead
// opens the named files in the GoVi.app macOS GUI (see launch_darwin.go) and
// does not start the terminal editor.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/pborman/options"

	"govi/engine"
	tcellfe "govi/frontend/tcell"
)

// cliOptions declares govi's flags for pborman/options, giving traditional
// getopt(3) behavior: short flags combine (-Gs), -- ends option parsing, and
// parsing stops at the first non-option argument.
type cliOptions struct {
	Help    bool `getopt:"--help -h display this help and exit"`
	Ex      bool `getopt:"-e start in ex mode (as if invoked as ex)"`
	Vi      bool `getopt:"-v start in vi mode (overrides an ex program name; wins over -e)"`
	GUI     bool `getopt:"-g open the files in the GoVi.app GUI instead of the terminal"`
	GUIWait bool `getopt:"-G like -g, and block until the tabs/windows for these files are closed"`
	Recover bool `getopt:"-r recover the named file from a recovery file"`
	Silent  bool `getopt:"-s do not read startup files or EXINIT/NEXINIT"`
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
	return runIO(filepath.Base(os.Args[0]), args, os.Stdout, os.Stderr)
}

// exProgname reports whether the program name selects ex mode, the way nvi
// keys SC_EX off argv[0] being ex or nex; goex is govi's spelling.
func exProgname(progname string) bool {
	return progname == "ex" || progname == "nex" || progname == "goex"
}

func runIO(progname string, args []string, stdout, stderr io.Writer) int {
	vopts, set := options.RegisterNew(progname, &cliOptions{})
	opts := vopts.(*cliOptions)
	set.SetParameters("[file ...]")
	if err := set.Getopt(append([]string{progname}, args...), nil); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", progname, err)
		set.PrintUsage(stderr)
		return 2
	}
	if opts.Help {
		set.PrintUsage(stdout)
		return 0
	}

	if opts.GUI || opts.GUIWait {
		return launchGUI(opts.Silent, opts.GUIWait, set.Args())
	}

	fe, err := newEditorFrontend()
	if err != nil {
		fmt.Fprintf(stderr, "govi: cannot initialize terminal: %v\n", err)
		return 1
	}
	defer fe.Close()

	eng := engine.New(fe, engine.Options{})
	fe.Attach(eng)
	defer eng.Close()

	if !opts.Silent {
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

	if (opts.Ex || exProgname(progname)) && !opts.Vi {
		eng.EnterEx()
	}

	runEditor(fe)
	return 0
}
