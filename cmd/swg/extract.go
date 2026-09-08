package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	gopath "path"
	"path/filepath"
	"strings"

	"github.com/madsboddum/swg-cli/archive"
)

const extractUsage = `usage: swg extract [-dir directory] [-C directory] [-archive name] [-q] [path...]

Write paths from the archives out as files, recreating each archive path as
directories below the destination. A path present in several archives is read
from the one that wins, loose files first and then the highest numbered patch.

Patterns may use * and ? within a path segment and ** across segments, so one
invocation can pull out a whole tree. Quote them, or the shell will try to
expand them first.

Bytes are written exactly as stored, whatever the format, so an extracted tree
round-trips back into an archive. Rendering a file is cat's job.

An existing file is overwritten. Each path is printed as it is written, so the
output is a manifest of what landed on disk.

  -dir directory
        directory holding the .tre archives; defaults to $SWG_DIR
  -C directory
        directory to write the files under; defaults to the working directory
  -archive name
        extract from this archive alone, ignoring precedence; with no path
        arguments it extracts everything the archive holds
  -q
        do not print the paths as they are written
`

func runExtract(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, extractUsage) }

	dir := fs.String("dir", "", "directory holding the .tre archives")
	dest := fs.String("C", ".", "directory to write the files under")
	archiveName := fs.String("archive", "", "extract from this archive alone")
	quiet := fs.Bool("q", false, "do not print the paths as they are written")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	operands := fs.Args()
	// Only -archive names a set of paths by itself; without it there is nothing
	// to extract.
	if len(operands) == 0 && *archiveName == "" {
		fmt.Fprint(stderr, extractUsage)
		return 2
	}

	root, err := archiveDir(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "swg extract: %v\n", err)
		return 2
	}

	stack, err := archive.Open(root)
	if err != nil {
		fmt.Fprintf(stderr, "swg extract: %v\n", err)
		return 1
	}
	defer func() { _ = stack.Close() }()

	paths := stack.Paths()
	if *archiveName != "" {
		if paths, err = stack.ArchivePaths(*archiveName); err != nil {
			fmt.Fprintf(stderr, "swg extract: %v\n", err)
			return 1
		}
	}

	e := &extractor{
		stack:   stack,
		dest:    *dest,
		archive: *archiveName,
		quiet:   *quiet,
		stdout:  stdout,
		stderr:  stderr,
	}
	if len(operands) == 0 {
		return e.writeAll(paths)
	}
	return e.extractAll(paths, operands)
}

// extractor writes resolved paths out under a destination directory.
type extractor struct {
	stack *archive.Stack
	dest  string
	// archive names the one archive to read from, empty to follow precedence.
	archive string
	quiet   bool
	stdout  io.Writer
	stderr  io.Writer
}

// extractAll writes every path each operand resolves to, reporting failures as
// it goes and returning the exit code they add up to.
func (e *extractor) extractAll(paths, operands []string) int {
	code := 0
	for _, operand := range operands {
		found, err := match(paths, operand)
		if err != nil {
			fmt.Fprintf(e.stderr, "swg extract: %v\n", err)
			code = 2
			continue
		}
		if len(found) == 0 {
			fmt.Fprintf(e.stderr, "swg extract: %s: no such path\n", operand)
			code = 1
			continue
		}
		if c := e.writeAll(found); c != 0 {
			code = c
		}
	}
	return code
}

func (e *extractor) writeAll(paths []string) int {
	code := 0
	for _, p := range paths {
		if err := e.write(p); err != nil {
			fmt.Fprintf(e.stderr, "swg extract: %v\n", err)
			code = 1
		}
	}
	return code
}

// write extracts one path, overwriting whatever sits at the destination.
func (e *extractor) write(path string) error {
	target, err := destPath(e.dest, path)
	if err != nil {
		return err
	}

	b, err := e.read(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(target, b, 0o644); err != nil {
		return err
	}

	if e.quiet {
		return nil
	}
	_, err = fmt.Fprintln(e.stdout, path)
	return err
}

// read returns a path's bytes, from the one archive -archive named or from
// whichever source wins.
func (e *extractor) read(path string) ([]byte, error) {
	if e.archive == "" {
		return e.stack.ReadFile(path)
	}
	return e.stack.ReadFileFrom(path, archive.Source{Archive: e.archive})
}

// destPath resolves an archive path to the file it extracts to. Member names
// are untrusted, so one that would land outside dest is refused rather than
// resolved.
func destPath(dest, path string) (string, error) {
	if gopath.IsAbs(path) || filepath.IsAbs(path) {
		return "", fmt.Errorf("%s: refusing to extract an absolute path", path)
	}
	// Split on the backslash too: it is an ordinary filename character here but
	// a separator on Windows, so rejecting only "/../" would let a crafted name
	// escape there.
	for _, seg := range strings.FieldsFunc(path, isPathSeparator) {
		if seg == ".." {
			return "", fmt.Errorf("%s: refusing to extract a path leaving the destination", path)
		}
	}
	return filepath.Join(dest, filepath.FromSlash(path)), nil
}

func isPathSeparator(r rune) bool { return r == '/' || r == '\\' }
