package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/madsboddum/swg-cli/archive"
)

// completionFlags lists each command's flags, in the order swg help shows
// them, for offering as completion candidates.
var completionFlags = map[string][]string{
	"cat":   {"-dir"},
	"ls":    {"-dir", "-archive"},
	"which": {"-dir", "-all"},
}

// runComplete implements the hidden __complete command: args is the command
// line following "swg", one word per argument, with the last one being the
// word under the cursor (empty if the cursor sits after a space). It writes
// one candidate per line and never fails outright, since a shell completion
// with no matches should just print nothing.
func runComplete(args []string, stdout, _ io.Writer) int {
	if len(args) == 0 {
		return 0
	}
	cur := args[len(args)-1]
	words := args[:len(args)-1]

	if len(words) == 0 {
		printMatches(stdout, candidateCommands(cur))
		return 0
	}

	switch words[0] {
	case "cat", "ls", "which":
		completeArchiveCommand(words[0], words[1:], cur, stdout)
	case "help", "completion":
		printMatches(stdout, candidateCommands(cur))
	}
	return 0
}

func candidateCommands(cur string) []string {
	var out []string
	for _, c := range commands {
		if strings.HasPrefix(c.name, cur) {
			out = append(out, c.name)
		}
	}
	return out
}

// completeArchiveCommand completes the arguments of cat, ls, and which: their
// flags, -archive's values, and paths inside the archives. -dir is left to
// the shell's own directory completion.
func completeArchiveCommand(sub string, prior []string, cur string, stdout io.Writer) {
	if len(prior) > 0 && prior[len(prior)-1] == "-archive" {
		stack, err := archive.Open(completionDir(prior))
		if err != nil {
			return
		}
		defer func() { _ = stack.Close() }()

		var names []string
		for _, name := range stack.Archives() {
			if strings.HasPrefix(name, cur) {
				names = append(names, name)
			}
		}
		printMatches(stdout, names)
		return
	}
	if len(prior) > 0 && prior[len(prior)-1] == "-dir" {
		return
	}

	if strings.HasPrefix(cur, "-") {
		var flags []string
		for _, f := range completionFlags[sub] {
			if strings.HasPrefix(f, cur) {
				flags = append(flags, f)
			}
		}
		printMatches(stdout, flags)
		return
	}

	stack, err := archive.Open(completionDir(prior))
	if err != nil {
		return
	}
	defer func() { _ = stack.Close() }()

	paths := stack.Paths()
	if name := flagValue(prior, "-archive"); name != "" {
		if p, err := stack.ArchivePaths(name); err == nil {
			paths = p
		}
	}
	printMatches(stdout, completePaths(paths, cur))
}

// completePaths lists the entries one directory level below cur, the same
// grouping ls shows, filtered to those starting with cur.
func completePaths(paths []string, cur string) []string {
	dir := ""
	if i := strings.LastIndex(cur, "/"); i >= 0 {
		dir = cur[:i]
	}

	var out []string
	for _, entry := range children(paths, dir) {
		if strings.HasPrefix(entry, cur) {
			out = append(out, entry)
		}
	}
	return out
}

// completionDir resolves the archive directory for a completion: the -dir
// value typed so far, or the environment as archiveDir already does.
func completionDir(words []string) string {
	if d := flagValue(words, "-dir"); d != "" {
		return d
	}
	dir, _ := archiveDir("")
	return dir
}

// flagValue returns the value following the last occurrence of name in
// words, or "" if it is not there or has nothing after it yet.
func flagValue(words []string, name string) string {
	for i, w := range words {
		if w == name && i+1 < len(words) {
			return words[i+1]
		}
	}
	return ""
}

func printMatches(w io.Writer, matches []string) {
	for _, m := range matches {
		fmt.Fprintln(w, m)
	}
}
