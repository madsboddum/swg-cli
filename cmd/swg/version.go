package main

import (
	"flag"
	"fmt"
	"io"
	"runtime/debug"
)

// version is stamped at build time via -ldflags "-X main.version=...".
// Empty means the binary was not built by the release pipeline, so fall back
// to the version the Go toolchain recorded.
var version string

func runVersion(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, "usage: swg version\n")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fs.Usage()
		return 2
	}

	fmt.Fprintf(stdout, "swg %s\n", versionString())
	return 0
}

func versionString() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "unknown"
}
