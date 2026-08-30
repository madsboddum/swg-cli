package main

import (
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/madsboddum/swg-cli/archive"
)

const whichUsage = `usage: swg which [-dir directory] [-all] path...

Show which archive a path is read from. Loose files beside the archives win
over anything inside one, and among archives filename sort order decides with
the last one winning, so patch_02.tre shadows patch_01.tre.

Patterns may use * and ? within a path segment and ** across segments. Quote
them, or the shell will try to expand them first.

  -dir directory
        directory holding the .tre archives; defaults to $SWG_DIR
  -all
        list every location holding the path, in precedence order
`

func runWhich(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("which", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, whichUsage) }

	dir := fs.String("dir", "", "directory holding the .tre archives")
	all := fs.Bool("all", false, "list every location holding the path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	operands := fs.Args()
	if len(operands) == 0 {
		fmt.Fprint(stderr, whichUsage)
		return 2
	}

	root, err := archiveDir(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "swg which: %v\n", err)
		return 2
	}

	stack, err := archive.Open(root)
	if err != nil {
		fmt.Fprintf(stderr, "swg which: %v\n", err)
		return 1
	}
	defer func() { _ = stack.Close() }()

	// Buffered so the winner column lines up across every operand at once.
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)

	code := 0
	for _, operand := range operands {
		found, err := match(stack.Paths(), operand)
		if err != nil {
			fmt.Fprintf(stderr, "swg which: %v\n", err)
			code = 2
			continue
		}
		if len(found) == 0 {
			fmt.Fprintf(stderr, "swg which: %s: no such path\n", operand)
			code = 1
			continue
		}
		for _, p := range found {
			if *all {
				printAll(tw, stack, p)
			} else {
				fmt.Fprintf(tw, "%s\t->\t%s\n", p, location(stack.Sources(p)[0], p))
			}
		}
	}
	if err := tw.Flush(); err != nil {
		fmt.Fprintf(stderr, "swg which: %v\n", err)
		return 1
	}
	return code
}

// printAll writes the path and every location holding it, winner first. The
// heading carries no tab, which ends the aligned block so one path's locations
// do not set the indent of the next one's.
func printAll(w io.Writer, stack *archive.Stack, path string) {
	fmt.Fprintln(w, path)
	for i, src := range stack.Sources(path) {
		notes := ""
		switch {
		case i == 0 && src.Loose():
			notes = "(winner, loose)"
		case i == 0:
			notes = "(winner)"
		case src.Loose():
			notes = "(loose)"
		}
		if notes == "" {
			fmt.Fprintf(w, "\t%s\n", location(src, path))
			continue
		}
		fmt.Fprintf(w, "\t%s\t%s\n", location(src, path), notes)
	}
}

// location names where a source holds the path: the archive filename, or the
// loose file's path relative to the archive directory.
func location(src archive.Source, path string) string {
	if src.Loose() {
		return "./" + path
	}
	return src.Archive
}
