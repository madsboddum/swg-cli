// Command swg is a command line tool for working with Star Wars Galaxies file formats.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	name := args[0]
	cmd, ok := lookup(name)
	if !ok {
		fmt.Fprintf(stderr, "swg: unknown command %q\n\n", name)
		printUsage(stderr)
		return 2
	}

	return cmd.run(args[1:], stdout, stderr)
}
