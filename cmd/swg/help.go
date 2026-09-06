package main

import (
	"fmt"
	"io"
)

// printCommandUsage writes a command's long help, falling back to its summary
// for commands that carry no usage text.
func printCommandUsage(w io.Writer, cmd command) {
	if cmd.usage != "" {
		fmt.Fprint(w, cmd.usage)
		return
	}
	fmt.Fprintf(w, "usage: swg %s\n\n%s\n", cmd.name, cmd.summary)
}
