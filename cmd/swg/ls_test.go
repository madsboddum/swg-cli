package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestLs(t *testing.T) {
	dir := lsFixture(t)

	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{
			name:    "top level",
			args:    []string{"ls"},
			wantOut: "string/\ntexture/\n",
		},
		{
			name:    "directory",
			args:    []string{"ls", "string/en/"},
			wantOut: "string/en/cmd.stf\nstring/en/loose.stf\nstring/en/sub/\nstring/en/ui.stf\n",
		},
		{
			name:    "directory without trailing slash",
			args:    []string{"ls", "string"},
			wantOut: "string/en/\n",
		},
		{
			name:    "exact path",
			args:    []string{"ls", "string/en/ui.stf"},
			wantOut: "string/en/ui.stf\n",
		},
		{
			name:    "glob within a segment",
			args:    []string{"ls", "string/en/*.stf"},
			wantOut: "string/en/cmd.stf\nstring/en/loose.stf\nstring/en/ui.stf\n",
		},
		{
			name:    "recursive glob",
			args:    []string{"ls", "string/en/**.stf"},
			wantOut: "string/en/cmd.stf\nstring/en/loose.stf\nstring/en/sub/deep.stf\nstring/en/ui.stf\n",
		},
		{
			name:    "shadowed path listed once",
			args:    []string{"ls", "texture/"},
			wantOut: "texture/crate.dds\n",
		},
		{
			name:    "archive scopes the listing",
			args:    []string{"ls", "-archive", "base.tre"},
			wantOut: "string/\ntexture/\n",
		},
		{
			name:     "archive that is not in the stack",
			args:     []string{"ls", "-archive", "nope.tre"},
			wantCode: 1,
			wantErr:  "nope.tre",
		},
		{
			name:     "pattern matching nothing",
			args:     []string{"ls", "string/de/*"},
			wantCode: 1,
			wantErr:  "no such path",
		},
		{
			name:     "malformed pattern",
			args:     []string{"ls", "string/[en"},
			wantCode: 2,
			wantErr:  "syntax error in pattern",
		},
		{
			name:     "several operands",
			args:     []string{"ls", "string/en/loose.stf", "texture/crate.dds"},
			wantOut:  "string/en/loose.stf\ntexture/crate.dds\n",
			wantCode: 0,
		},
		{
			name:     "one operand of several matches nothing",
			args:     []string{"ls", "string/en/loose.stf", "gone.txt"},
			wantOut:  "string/en/loose.stf\n",
			wantCode: 1,
			wantErr:  "gone.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SWG_DIR", dir)
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, tt.wantCode, stderr.String())
			}
			if got := stdout.String(); got != tt.wantOut {
				t.Errorf("stdout = %q, want %q", got, tt.wantOut)
			}
			if tt.wantErr != "" && !strings.Contains(stderr.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantErr)
			}
		})
	}
}

func TestLsDirFlagBeatsEnvironment(t *testing.T) {
	dir := lsFixture(t)
	t.Setenv("SWG_DIR", t.TempDir())

	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls", "-dir", dir, "string/en/loose.stf"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr.String())
	}
	if got := stdout.String(); got != "string/en/loose.stf\n" {
		t.Errorf("stdout = %q", got)
	}
}

func TestLsWithoutDirectoryConfigured(t *testing.T) {
	t.Setenv("SWG_DIR", "")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls"}, &stdout, &stderr); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "--dir") || !strings.Contains(stderr.String(), "SWG_DIR") {
		t.Errorf("stderr = %q, want it to name both ways to set the directory", stderr.String())
	}
}

func TestLsWithoutArchives(t *testing.T) {
	t.Setenv("SWG_DIR", t.TempDir())

	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls"}, &stdout, &stderr); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no .tre archives") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestHelpLsShowsTheLongUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"help", "ls"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "-archive") {
		t.Errorf("stdout = %q, want the ls flags documented", stdout.String())
	}
}

// lsFixture builds an archive directory: a base archive, a patch shadowing one
// of its paths, and a loose file beside them.
func lsFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeTestArchive(t, filepath.Join(dir, "base.tre"), map[string]string{
		"string/en/ui.stf":       "base ui",
		"string/en/cmd.stf":      "base cmd",
		"string/en/sub/deep.stf": "base deep",
		"texture/crate.dds":      "base crate",
	})
	writeTestArchive(t, filepath.Join(dir, "patch_01.tre"), map[string]string{
		"texture/crate.dds": "patched crate",
	})

	loose := filepath.Join(dir, "string", "en", "loose.stf")
	if err := os.MkdirAll(filepath.Dir(loose), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loose, []byte("loose"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeTestArchive builds a minimal uncompressed .tre archive.
func writeTestArchive(t *testing.T, path string, members map[string]string) {
	t.Helper()

	names := make([]string, 0, len(members))
	for n := range members {
		names = append(names, n)
	}
	sort.Strings(names)

	const headerSize = 36
	var data, nameBlock, index bytes.Buffer
	for _, n := range names {
		rec := [6]uint32{0, uint32(len(members[n])), uint32(headerSize + data.Len()), 0, 0, uint32(nameBlock.Len())}
		if err := binary.Write(&index, binary.LittleEndian, rec); err != nil {
			t.Fatal(err)
		}
		data.WriteString(members[n])
		nameBlock.WriteString(n)
		nameBlock.WriteByte(0)
	}

	var out bytes.Buffer
	out.WriteString("EERT")
	out.WriteString("5000")
	for _, v := range []uint32{
		uint32(len(names)),
		uint32(headerSize + data.Len()),
		0,
		uint32(index.Len()),
		0,
		uint32(nameBlock.Len()),
		uint32(nameBlock.Len()),
	} {
		if err := binary.Write(&out, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	out.Write(data.Bytes())
	out.Write(index.Bytes())
	out.Write(nameBlock.Bytes())

	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
