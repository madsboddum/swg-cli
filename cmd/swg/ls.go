package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/madsboddum/swg-cli/archive"
)

const lsUsage = `usage: swg ls [-dir directory] [-archive name] [path...]

List paths across every .tre archive in the directory and the loose files
beside them. A path present in several archives is listed once.

A path argument naming a directory lists its immediate entries, with
directories marked by a trailing slash. Patterns may use * and ? within a
path segment and ** across segments. With no arguments, ls lists the top
level.

  -dir directory
        directory holding the .tre archives; defaults to $SWG_DIR
  -archive name
        list only the paths held by this archive, ignoring precedence
`

func runLs(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, lsUsage) }

	dir := fs.String("dir", "", "directory holding the .tre archives")
	archiveName := fs.String("archive", "", "list only this archive's paths")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	root, err := archiveDir(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "swg ls: %v\n", err)
		return 2
	}

	stack, err := archive.Open(root)
	if err != nil {
		fmt.Fprintf(stderr, "swg ls: %v\n", err)
		return 1
	}
	defer func() { _ = stack.Close() }()

	paths := stack.Paths()
	if *archiveName != "" {
		if paths, err = stack.ArchivePaths(*archiveName); err != nil {
			fmt.Fprintf(stderr, "swg ls: %v\n", err)
			return 1
		}
	}

	operands := fs.Args()
	if len(operands) == 0 {
		operands = []string{""}
	}

	code := 0
	for _, operand := range operands {
		found, err := list(paths, operand)
		if err != nil {
			fmt.Fprintf(stderr, "swg ls: %v\n", err)
			code = 2
			continue
		}
		if len(found) == 0 {
			fmt.Fprintf(stderr, "swg ls: %s: no such path\n", operand)
			code = 1
			continue
		}
		for _, p := range found {
			fmt.Fprintln(stdout, p)
		}
	}
	return code
}

// list resolves one ls argument against the sorted path index: a glob pattern
// matches paths, an exact path lists itself, and anything else is read as a
// directory to list the entries of.
func list(paths []string, operand string) ([]string, error) {
	if archive.HasMeta(operand) {
		var out []string
		for _, p := range paths {
			ok, err := archive.MatchPath(operand, p)
			if err != nil {
				return nil, err
			}
			if ok {
				out = append(out, p)
			}
		}
		return out, nil
	}

	name := strings.TrimSuffix(operand, "/")
	if !strings.HasSuffix(operand, "/") {
		for _, p := range paths {
			if p == name {
				return []string{p}, nil
			}
		}
	}
	return children(paths, name), nil
}

// children lists the entries directly under dir, which is "" for the top
// level. Subdirectories are collapsed to a single entry ending in a slash.
func children(paths []string, dir string) []string {
	prefix := ""
	if dir != "" {
		prefix = dir + "/"
	}

	var out []string
	var last string
	for _, p := range paths {
		rest, ok := strings.CutPrefix(p, prefix)
		if !ok || rest == "" {
			continue
		}
		entry := p
		if i := strings.Index(rest, "/"); i >= 0 {
			entry = prefix + rest[:i+1]
		}
		if entry != last {
			out = append(out, entry)
			last = entry
		}
	}
	return out
}
