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

	if args := flag.Args(); len(args) > 0 {
		if err := eng.Open(args[0]); err != nil {
			fe.Close()
			fmt.Fprintf(os.Stderr, "nvi: %v\n", err)
			os.Exit(1)
		}
	}

	fe.Run()
}
