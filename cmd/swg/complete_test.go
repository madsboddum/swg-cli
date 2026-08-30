package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestComplete(t *testing.T) {
	dir := lsFixture(t)
	t.Setenv("SWG_DIR", dir)

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "top level command", args: []string{"c"}, want: []string{"cat", "completion"}},
		{name: "top level directory", args: []string{"cat", "str"}, want: []string{"string/"}},
		{name: "descend a directory", args: []string{"cat", "string/"}, want: []string{"string/en/"}},
		{
			name: "prefix within a directory",
			args: []string{"cat", "string/en/c"},
			want: []string{"string/en/cmd.stf"},
		},
		{name: "exact file", args: []string{"cat", "string/en/ui.stf"}, want: []string{"string/en/ui.stf"}},
		{name: "flag name", args: []string{"ls", "-a"}, want: []string{"-archive"}},
		{
			name: "flag names for cat",
			args: []string{"cat", "-"},
			want: []string{"-dir"},
		},
		{name: "archive name", args: []string{"ls", "-archive", "ba"}, want: []string{"base.tre"}},
		{name: "dir flag falls back to the shell", args: []string{"ls", "-dir", "/tm"}, want: nil},
		{name: "no words yet", args: nil, want: nil},
		{name: "unknown command completes nothing", args: []string{"nope", ""}, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(append([]string{"__complete"}, tt.args...), &stdout, &stderr)

			if code != 0 {
				t.Fatalf("exit code = %d, stderr %q", code, stderr.String())
			}
			got := stdout.String()
			var lines []string
			if got != "" {
				lines = strings.Split(strings.TrimSuffix(got, "\n"), "\n")
			}
			if !equalSlices(lines, tt.want) {
				t.Errorf("candidates = %v, want %v", lines, tt.want)
			}
		})
	}
}

func TestCompleteWithoutArchiveDirectory(t *testing.T) {
	t.Setenv("SWG_DIR", "")

	var stdout, stderr bytes.Buffer
	code := run([]string{"__complete", "cat", ""}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr.String())
	}
	if stdout.String() != "" {
		t.Errorf("stdout = %q, want no candidates", stdout.String())
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
