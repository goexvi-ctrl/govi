// Command nvi is the terminal entry point for the govi editor: it wires the
// embeddable engine to the tcell terminal frontend. The engine carries no
// terminal dependency; this command is just one host for it.
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
	flag.Parse()

	fe, err := tcellfe.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "nvi: cannot initialize terminal: %v\n", err)
		os.Exit(1)
	}
	defer fe.Close()

	eng := engine.New(fe, engine.Options{})
	fe.Attach(eng)
	defer eng.Close()

	if !*silent {
		if err := eng.LoadStartup(); err != nil {
			fe.Close()
			fmt.Fprintf(os.Stderr, "nvi: %v\n", err)
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
				fmt.Fprintf(os.Stderr, "nvi: %v\n", err)
				os.Exit(1)
			}
			if len(entries) == 0 {
				fmt.Println("nvi: No files to recover")
				os.Exit(0)
			}
			for _, ent := range entries {
				fmt.Printf("%s: %s\n", ent.Mtime.Format(time.ANSIC), ent.Orig)
			}
			os.Exit(0)
		}
		if err := eng.Recover(args[0]); err != nil {
			fe.Close()
			fmt.Fprintf(os.Stderr, "nvi: %v\n", err)
			os.Exit(1)
		}
	} else if err := eng.OpenArgs(args); err != nil {
		fe.Close()
		fmt.Fprintf(os.Stderr, "nvi: %v\n", err)
		os.Exit(1)
	}

	fe.Run()
}
