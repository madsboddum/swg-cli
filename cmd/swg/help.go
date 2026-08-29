package main

import (
	"fmt"
	"io"
)

func runHelp(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}

	if len(args) > 1 {
		fmt.Fprint(stderr, "usage: swg help [command]\n")
		return 2
	}

	cmd, ok := lookup(args[0])
	if !ok {
		fmt.Fprintf(stderr, "swg help: unknown command %q\n", args[0])
		return 2
	}

	fmt.Fprintf(stdout, "usage: swg %s\n\n%s\n", cmd.name, cmd.summary)
	return 0
}
