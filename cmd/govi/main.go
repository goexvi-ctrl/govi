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
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"govi/engine"
	tcellfe "govi/frontend/tcell"
)

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
	fs := flag.NewFlagSet("govi", flag.ContinueOnError)
	fs.SetOutput(stderr)

	recover := fs.Bool("r", false, "recover the named file from a recovery file")
	silent := fs.Bool("s", false, "do not read startup files or EXINIT/NEXINIT")
	exMode := fs.Bool("e", false, "start in ex mode (as if invoked as ex)")
	viMode := fs.Bool("v", false, "start in vi mode (overrides an ex program name; wins over -e)")
	gui := fs.Bool("g", false, "open the files in the GoVi.app GUI instead of the terminal")
	guiWait := fs.Bool("G", false, "like -g, and block until the tabs/windows for these files are closed")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *gui || *guiWait {
		return launchGUI(*silent, *guiWait, fs.Args())
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

	if !*silent {
		if err := eng.LoadStartup(); err != nil {
			fe.Close()
			fmt.Fprintf(stderr, "govi: %v\n", err)
			return 1
		}
		if eng.ShouldQuit() {
			return 0
		}
	}

	openArgs := fs.Args()
	if *recover {
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

	if (*exMode || exProgname(progname)) && !*viMode {
		eng.EnterEx()
	}

	runEditor(fe)
	return 0
}
