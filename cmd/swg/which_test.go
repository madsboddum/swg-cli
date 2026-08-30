package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWhich(t *testing.T) {
	dir := whichFixture(t)

	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{
			name:    "loose file wins over every archive",
			args:    []string{"which", "string/en/ui.stf"},
			wantOut: "string/en/ui.stf  ->  ./string/en/ui.stf\n",
		},
		{
			name:    "highest patch wins",
			args:    []string{"which", "texture/crate.dds"},
			wantOut: "texture/crate.dds  ->  patch_01.tre\n",
		},
		{
			name:    "only one archive holds it",
			args:    []string{"which", "string/en/cmd.stf"},
			wantOut: "string/en/cmd.stf  ->  base.tre\n",
		},
		{
			name: "all lists the precedence order",
			args: []string{"which", "-all", "string/en/ui.stf"},
			wantOut: "string/en/ui.stf\n" +
				"  ./string/en/ui.stf  (winner, loose)\n" +
				"  patch_02.tre\n" +
				"  patch_01.tre\n" +
				"  base.tre\n",
		},
		{
			name: "all marks an archive winner",
			args: []string{"which", "-all", "texture/crate.dds"},
			wantOut: "texture/crate.dds\n" +
				"  patch_01.tre  (winner)\n" +
				"  base.tre\n",
		},
		{
			name:    "pattern resolves every match",
			args:    []string{"which", "string/en/*.stf"},
			wantOut: "string/en/cmd.stf  ->  base.tre\nstring/en/ui.stf   ->  ./string/en/ui.stf\n",
		},
		{
			name:     "path found nowhere",
			args:     []string{"which", "string/en/gone.stf"},
			wantCode: 1,
			wantErr:  "no such path",
		},
		{
			name:     "one operand of several found nowhere",
			args:     []string{"which", "gone.txt", "string/en/cmd.stf"},
			wantOut:  "string/en/cmd.stf  ->  base.tre\n",
			wantCode: 1,
			wantErr:  "gone.txt",
		},
		{
			name:     "malformed pattern",
			args:     []string{"which", "string/[en"},
			wantCode: 2,
			wantErr:  "syntax error in pattern",
		},
		{
			name:     "no operands",
			args:     []string{"which"},
			wantCode: 2,
			wantErr:  "usage: swg which",
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

func TestWhichWithoutDirectoryConfigured(t *testing.T) {
	t.Setenv("SWG_DIR", "")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"which", "string/en/ui.stf"}, &stdout, &stderr); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "--dir") || !strings.Contains(stderr.String(), "SWG_DIR") {
		t.Errorf("stderr = %q, want it to name both ways to set the directory", stderr.String())
	}
}

func TestHelpWhichShowsTheLongUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"help", "which"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "-all") {
		t.Errorf("stdout = %q, want the which flags documented", stdout.String())
	}
}

// whichFixture builds an archive directory where one path sits in three
// archives and in a loose file, and another in two archives only.
func whichFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeTestArchive(t, filepath.Join(dir, "base.tre"), map[string]string{
		"string/en/ui.stf":  "base ui",
		"string/en/cmd.stf": "base cmd",
		"texture/crate.dds": "base crate",
	})
	writeTestArchive(t, filepath.Join(dir, "patch_01.tre"), map[string]string{
		"string/en/ui.stf":  "patch 1 ui",
		"texture/crate.dds": "patch 1 crate",
	})
	writeTestArchive(t, filepath.Join(dir, "patch_02.tre"), map[string]string{
		"string/en/ui.stf": "patch 2 ui",
	})

	loose := filepath.Join(dir, "string", "en", "ui.stf")
	if err := os.MkdirAll(filepath.Dir(loose), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loose, []byte("loose ui"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
