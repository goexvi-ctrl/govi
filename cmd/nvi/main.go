// Command nvi is the terminal entry point for the govi editor: it wires the
// embeddable engine to the tcell terminal frontend. The engine carries no
// terminal dependency; this command is just one host for it.
package main

import (
	"flag"
	"fmt"
	"os"

	"govi/engine"
	tcellfe "govi/frontend/tcell"
)

func main() {
	recover := flag.Bool("r", false, "recover the named file from a recovery file")
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

	args := flag.Args()
	if *recover {
		if len(args) == 0 {
			fe.Close()
			fmt.Fprintln(os.Stderr, "usage: nvi -r file")
			os.Exit(1)
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
