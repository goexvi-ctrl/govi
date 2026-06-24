// Command govi is the terminal entry point for the govi editor: it wires the
// embeddable engine to the tcell terminal frontend. The engine carries no
// terminal dependency; this command is just one host for it.
//
// With -g, govi instead opens the named files in the Govi.app macOS GUI (see
// launch_darwin.go) and does not start the terminal editor.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"govi/engine"
	tcellfe "govi/frontend/tcell"
)

func main() {
	recover := flag.Bool("r", false, "recover the named file from a recovery file")
	silent := flag.Bool("s", false, "do not read startup files or EXINIT/NEXINIT")
	gui := flag.Bool("g", false, "open the files in the Govi.app GUI instead of the terminal")
	wait := flag.Bool("w", false, "with -g, block until the opened tabs/windows close")
	flag.Parse()

	if *gui {
		os.Exit(runGUI(*silent, *wait, flag.Args()))
	}
	if *wait {
		fmt.Fprintln(os.Stderr, "govi: -w is only valid with -g")
		os.Exit(2)
	}

	fe, err := tcellfe.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "govi: cannot initialize terminal: %v\n", err)
		os.Exit(1)
	}
	defer fe.Close()

	eng := engine.New(fe, engine.Options{})
	fe.Attach(eng)
	defer eng.Close()

	if !*silent {
		if err := eng.LoadStartup(); err != nil {
			fe.Close()
			fmt.Fprintf(os.Stderr, "govi: %v\n", err)
			os.Exit(1)
		}
		if eng.ShouldQuit() {
			os.Exit(0)
		}
	}

	args := flag.Args()
	if *recover {
		if len(args) == 0 {
			fe.Close()
			entries, err := eng.ListRecoverable()
			if err != nil {
				fmt.Fprintf(os.Stderr, "govi: %v\n", err)
				os.Exit(1)
			}
			if len(entries) == 0 {
				fmt.Println("govi: No files to recover")
				os.Exit(0)
			}
			for _, ent := range entries {
				fmt.Printf("%s: %s\n", ent.Mtime.Format(time.ANSIC), ent.Orig)
			}
			os.Exit(0)
		}
		if err := eng.Recover(args[0]); err != nil {
			fe.Close()
			fmt.Fprintf(os.Stderr, "govi: %v\n", err)
			os.Exit(1)
		}
	} else if err := eng.OpenArgs(args); err != nil {
		fe.Close()
		fmt.Fprintf(os.Stderr, "govi: %v\n", err)
		os.Exit(1)
	}

	fe.Run()
}
