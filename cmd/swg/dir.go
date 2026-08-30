package main

import (
	"errors"
	"os"
)

// dirEnv is the environment variable naming the directory the .tre archives
// live in.
const dirEnv = "SWG_DIR"

// errNoDir reports that the archive directory was not configured anywhere.
var errNoDir = errors.New("no archive directory set; pass --dir or set " + dirEnv)

// archiveDir resolves the directory holding the .tre archives. The --dir flag
// wins over the environment.
func archiveDir(flagDir string) (string, error) {
	if flagDir != "" {
		return flagDir, nil
	}
	if d := os.Getenv(dirEnv); d != "" {
		return d, nil
	}
	return "", errNoDir
}
