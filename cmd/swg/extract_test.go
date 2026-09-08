package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtract(t *testing.T) {
	dir := extractFixture(t)

	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
		// wantFiles maps a path below the destination to its contents.
		wantFiles map[string]string
	}{
		{
			name:      "a literal path lands under the destination",
			args:      []string{"string/en/ui.stf"},
			wantOut:   "string/en/ui.stf\n",
			wantFiles: map[string]string{"string/en/ui.stf": "base ui"},
		},
		{
			name: "a pattern extracts the whole tree it matches",
			args: []string{"string/en/**.stf"},
			wantOut: "string/en/cmd.stf\n" +
				"string/en/loose.stf\n" +
				"string/en/sub/deep.stf\n" +
				"string/en/ui.stf\n",
			wantFiles: map[string]string{
				"string/en/cmd.stf":      "base cmd",
				"string/en/loose.stf":    "loose",
				"string/en/sub/deep.stf": "base deep",
				"string/en/ui.stf":       "base ui",
			},
		},
		{
			name:      "the winning source is the one written",
			args:      []string{"texture/crate.dds"},
			wantOut:   "texture/crate.dds\n",
			wantFiles: map[string]string{"texture/crate.dds": "patched crate"},
		},
		{
			name:      "-archive ignores precedence",
			args:      []string{"-archive", "base.tre", "texture/crate.dds"},
			wantOut:   "texture/crate.dds\n",
			wantFiles: map[string]string{"texture/crate.dds": "base crate"},
		},
		{
			name: "-archive with no paths extracts everything it holds",
			args: []string{"-archive", "base.tre"},
			wantOut: "string/en/cmd.stf\n" +
				"string/en/sub/deep.stf\n" +
				"string/en/ui.stf\n" +
				"texture/crate.dds\n",
			wantFiles: map[string]string{
				"string/en/cmd.stf": "base cmd",
				"texture/crate.dds": "base crate",
			},
		},
		{
			name:      "-q writes the files without the manifest",
			args:      []string{"-q", "string/en/ui.stf"},
			wantFiles: map[string]string{"string/en/ui.stf": "base ui"},
		},
		{
			name:      "several operands extract in order",
			args:      []string{"texture/crate.dds", "string/en/ui.stf"},
			wantOut:   "texture/crate.dds\nstring/en/ui.stf\n",
			wantFiles: map[string]string{"string/en/ui.stf": "base ui"},
		},
		{
			name:     "a pattern matching nothing",
			args:     []string{"string/de/*.stf"},
			wantCode: 1,
			wantErr:  "no such path",
		},
		{
			name:     "a directory is not expanded",
			args:     []string{"string/en"},
			wantCode: 1,
			wantErr:  "no such path",
		},
		{
			name:     "malformed pattern",
			args:     []string{"string/[en"},
			wantCode: 2,
			wantErr:  "syntax error in pattern",
		},
		{
			name:     "an unknown archive",
			args:     []string{"-archive", "nope.tre"},
			wantCode: 1,
			wantErr:  "nope.tre",
		},
		{
			name:      "one operand of several fails",
			args:      []string{"string/en/ui.stf", "gone.txt"},
			wantOut:   "string/en/ui.stf\n",
			wantCode:  1,
			wantErr:   "gone.txt",
			wantFiles: map[string]string{"string/en/ui.stf": "base ui"},
		},
		{
			name:     "no paths and no archive",
			args:     nil,
			wantCode: 2,
			wantErr:  "usage: swg extract",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SWG_DIR", dir)
			dest := t.TempDir()

			var stdout, stderr bytes.Buffer
			args := append([]string{"extract", "-C", dest}, tt.args...)
			code := run(args, &stdout, &stderr)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, tt.wantCode, stderr.String())
			}
			if got := stdout.String(); got != tt.wantOut {
				t.Errorf("stdout = %q, want %q", got, tt.wantOut)
			}
			if tt.wantErr != "" && !strings.Contains(stderr.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantErr)
			}
			for p, want := range tt.wantFiles {
				b, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(p)))
				if err != nil {
					t.Fatal(err)
				}
				if string(b) != want {
					t.Errorf("%s = %q, want %q", p, b, want)
				}
			}
		})
	}
}

func TestExtractDefaultsToTheWorkingDirectory(t *testing.T) {
	t.Setenv("SWG_DIR", extractFixture(t))
	dest := t.TempDir()
	t.Chdir(dest)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"extract", "string/en/ui.stf"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr.String())
	}
	b, err := os.ReadFile(filepath.Join(dest, "string", "en", "ui.stf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "base ui" {
		t.Errorf("extracted %q, want %q", b, "base ui")
	}
}

func TestExtractOverwritesAnExistingFile(t *testing.T) {
	t.Setenv("SWG_DIR", extractFixture(t))
	dest := t.TempDir()

	target := filepath.Join(dest, "string", "en", "ui.stf")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("stale and much longer than what replaces it"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"extract", "-C", dest, "string/en/ui.stf"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr.String())
	}
	b, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "base ui" {
		t.Errorf("extracted %q, want %q", b, "base ui")
	}
}

// TestExtractRefusesToEscapeTheDestination covers member names crafted to
// write outside the destination. The archive index is untrusted input, so the
// name has to be rejected rather than resolved.
func TestExtractRefusesToEscapeTheDestination(t *testing.T) {
	dir := t.TempDir()
	writeTestArchive(t, filepath.Join(dir, "hostile.tre"), map[string]string{
		"../escape.txt":       "climbed out",
		"a/../../escape2.txt": "climbed out",
		"/etc/passwd":         "root:x:0:0",
		"safe.txt":            "safe",
	})
	t.Setenv("SWG_DIR", dir)

	dest := filepath.Join(t.TempDir(), "out")
	var stdout, stderr bytes.Buffer
	code := run([]string{"extract", "-C", dest, "-archive", "hostile.tre"}, &stdout, &stderr)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if got := stdout.String(); got != "safe.txt\n" {
		t.Errorf("stdout = %q, want only the safe path", got)
	}
	if n := strings.Count(stderr.String(), "refusing"); n != 3 {
		t.Errorf("stderr = %q, want three refusals", stderr.String())
	}

	for _, p := range []string{"../escape.txt", "../../escape2.txt", "escape.txt", "escape2.txt"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(p))); err == nil {
			t.Errorf("%s was written", p)
		}
	}
}

func TestExtractDestPath(t *testing.T) {
	tests := []struct {
		path    string
		wantErr string
	}{
		{path: "string/en/ui.stf"},
		{path: "a..b/c"},
		{path: "..", wantErr: "leaving the destination"},
		{path: "../escape", wantErr: "leaving the destination"},
		{path: "a/../../escape", wantErr: "leaving the destination"},
		{path: `a\..\..\escape`, wantErr: "leaving the destination"},
		{path: "/etc/passwd", wantErr: "absolute path"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := destPath("/dest", tt.path)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("destPath(%q) = %q, want an error", tt.path, got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("destPath(%q) error = %v, want it to mention %q", tt.path, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("destPath(%q): %v", tt.path, err)
			}
			want := filepath.Join("/dest", filepath.FromSlash(tt.path))
			if got != want {
				t.Errorf("destPath(%q) = %q, want %q", tt.path, got, want)
			}
		})
	}
}

func TestHelpExtractShowsTheLongUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"extract", "--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "-C directory") {
		t.Errorf("stdout = %q, want the destination flag documented", stdout.String())
	}
}

// extractFixture builds an archive directory: a base archive, a patch
// shadowing one of its paths, and a loose file beside them.
func extractFixture(t *testing.T) string {
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
