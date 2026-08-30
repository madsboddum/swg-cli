package main

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// command is a single swg subcommand. run receives the arguments following the
// subcommand name and returns the process exit code.
type command struct {
	name    string
	summary string
	// usage is the long help text. Commands without one fall back to summary.
	usage string
	run   func(args []string, stdout, stderr io.Writer) int
}

// commands is the dispatch table, in the order help lists them. Every entry is
// a verb and none is a format name: docs/decisions/0002-commands-are-verbs.md
var commands = []command{
	{name: "cat", summary: "Write paths from the archives to standard output", run: runCat, usage: catUsage},
	{name: "help", summary: "Show usage for swg or a subcommand"},
	{name: "ls", summary: "List paths across the archives", run: runLs, usage: lsUsage},
	{name: "version", summary: "Print the swg version", run: runVersion},
	{name: "which", summary: "Show which archive a path is read from", run: runWhich, usage: whichUsage},
}

// runHelp reads commands, so wiring it in the literal above would be an
// initialization cycle.
func init() {
	for i := range commands {
		if commands[i].name == "help" {
			commands[i].run = runHelp
		}
	}
}

func lookup(name string) (command, bool) {
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, "swg is a command line tool for Star Wars Galaxies file formats.\n\n")
	fmt.Fprint(w, "Usage:\n\n\tswg <command> [arguments]\n\nCommands:\n\n")

	tw := tabwriter.NewWriter(w, 0, 0, 4, ' ', 0)
	for _, c := range commands {
		fmt.Fprintf(tw, "\t%s\t%s\n", c.name, c.summary)
	}
	tw.Flush()

	fmt.Fprint(w, "\nRun \"swg help <command>\" for details on a command.\n")
}
