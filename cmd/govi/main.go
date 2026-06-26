// Command govi is the terminal entry point for the govi editor: it wires the
// embeddable engine to the tcell terminal frontend. The engine carries no
// terminal dependency; this command is just one host for it.
//
// With -g, govi instead opens the named files in the GoVi.app macOS GUI (see
// launch_darwin.go) and does not start the terminal editor.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
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
	os.Exit(run(os.Args[1:]))
}

// run is the real entry point; it returns an exit code for main (and tests).
func run(args []string) int {
	return runIO(args, os.Stdout, os.Stderr)
}

func runIO(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("govi", flag.ContinueOnError)
	fs.SetOutput(stderr)

	recover := fs.Bool("r", false, "recover the named file from a recovery file")
	silent := fs.Bool("s", false, "do not read startup files or EXINIT/NEXINIT")
	gui := fs.Bool("g", false, "open the files in the GoVi.app GUI instead of the terminal")
	wait := fs.Bool("w", false, "with -g, block until the tabs/windows for these files are closed")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *gui {
		return launchGUI(*silent, *wait, fs.Args())
	}
	if *wait {
		fmt.Fprintln(stderr, "govi: -w is only valid with -g")
		return 2
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
	} else if err := eng.OpenArgs(openArgs); err != nil {
		fe.Close()
		fmt.Fprintf(stderr, "govi: %v\n", err)
		return 1
	}

	runEditor(fe)
	return 0
}