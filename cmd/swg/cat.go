package main

import (
	"flag"
	"fmt"
	"io"
	gopath "path"
	"strings"

	"github.com/madsboddum/swg-cli/archive"
	"github.com/madsboddum/swg-cli/stf"
)

const catUsage = `usage: swg cat [-dir directory] path...

Write the contents of paths from the archives to standard output. A path
present in several archives is read from the one that wins, loose files
first and then the highest numbered patch.

Patterns may use * and ? within a path segment and ** across segments, so
one invocation can concatenate a whole tree of files. Quote them, or the
shell will try to expand them first.

String tables are decoded, one entry per line as @file:key|value, the string
id the game writes, which makes a reverse lookup a matter of piping cat into
grep:

    swg cat 'string/en/**.stf' | grep -i "you feel a disturbance"

Every other file is written out as the bytes it holds.

  -dir directory
        directory holding the .tre archives; defaults to $SWG_DIR
`

func runCat(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cat", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, catUsage) }

	dir := fs.String("dir", "", "directory holding the .tre archives")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	operands := fs.Args()
	if len(operands) == 0 {
		fmt.Fprint(stderr, catUsage)
		return 2
	}

	root, err := archiveDir(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "swg cat: %v\n", err)
		return 2
	}

	stack, err := archive.Open(root)
	if err != nil {
		fmt.Fprintf(stderr, "swg cat: %v\n", err)
		return 1
	}
	defer func() { _ = stack.Close() }()

	code := 0
	for _, operand := range operands {
		found, err := match(stack.Paths(), operand)
		if err != nil {
			fmt.Fprintf(stderr, "swg cat: %v\n", err)
			code = 2
			continue
		}
		if len(found) == 0 {
			fmt.Fprintf(stderr, "swg cat: %s: no such path\n", operand)
			code = 1
			continue
		}
		for _, p := range found {
			if err := emit(stack, p, stdout); err != nil {
				fmt.Fprintf(stderr, "swg cat: %v\n", err)
				code = 1
			}
		}
	}
	return code
}

// match resolves one cat argument against the sorted path index. Unlike ls,
// a path naming a directory is not expanded to its entries; use a pattern.
func match(paths []string, operand string) ([]string, error) {
	if !archive.HasMeta(operand) {
		for _, p := range paths {
			if p == operand {
				return []string{p}, nil
			}
		}
		return nil, nil
	}

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

// emit writes one file, decoding it first if it is a string table.
func emit(stack *archive.Stack, path string, stdout io.Writer) error {
	b, err := stack.ReadFile(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(gopath.Ext(path), ".stf") {
		_, err = stdout.Write(b)
		return err
	}

	table, err := stf.Decode(b)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	id := stringID(path)
	for _, e := range table.Entries() {
		if _, err := fmt.Fprintf(stdout, "%s:%s|%s\n", id, e.Key, e.Value); err != nil {
			return err
		}
	}
	return nil
}

// stringID is the @file part of the prefix on a string table's lines. Tables
// live under string/<locale>/ and the client names them by what follows with
// the extension dropped, so @zone_n:yavin4 is a string id as the game's own
// data writes it.
func stringID(path string) string {
	rest := strings.TrimSuffix(path, gopath.Ext(path))
	if after, ok := strings.CutPrefix(rest, "string/"); ok {
		if i := strings.Index(after, "/"); i >= 0 {
			rest = after[i+1:]
		}
	}
	return "@" + rest
}
