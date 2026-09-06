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
	if name == "__complete" {
		return runComplete(args[1:], stdout, stderr)
	}
	if isHelpFlag(name) {
		printUsage(stdout)
		return 0
	}

	cmd, ok := lookup(name)
	if !ok {
		fmt.Fprintf(stderr, "swg: unknown command %q\n\n", name)
		printUsage(stderr)
		return 2
	}

	// Handled here rather than by each command's flag set, so that --help wins
	// over any other argument and prints to stdout with a zero exit code.
	for _, a := range args[1:] {
		if isHelpFlag(a) {
			printCommandUsage(stdout, cmd)
			return 0
		}
	}

	return cmd.run(args[1:], stdout, stderr)
}

func isHelpFlag(arg string) bool {
	return arg == "-h" || arg == "-help" || arg == "--help"
}
