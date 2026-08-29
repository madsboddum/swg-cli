package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{name: "no args", args: nil, wantCode: 2, wantErr: "Usage:"},
		{name: "unknown command", args: []string{"nope"}, wantCode: 2, wantErr: `unknown command "nope"`},
		{name: "version", args: []string{"version"}, wantCode: 0, wantOut: "swg "},
		{name: "version with extra arg", args: []string{"version", "x"}, wantCode: 2, wantErr: "usage: swg version"},
		{name: "help", args: []string{"help"}, wantCode: 0, wantOut: "Usage:"},
		{name: "help version", args: []string{"help", "version"}, wantCode: 0, wantOut: "usage: swg version"},
		{name: "help unknown", args: []string{"help", "nope"}, wantCode: 2, wantErr: `unknown command "nope"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if tt.wantOut != "" && !strings.Contains(stdout.String(), tt.wantOut) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantOut)
			}
			if tt.wantErr != "" && !strings.Contains(stderr.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantErr)
			}
		})
	}
}

func TestVersionStringFallsBack(t *testing.T) {
	if got := versionString(); got == "" {
		t.Error("versionString() is empty")
	}
}

func TestVersionStringUsesLdflagsValue(t *testing.T) {
	t.Cleanup(func() { version = "" })
	version = "1.2.3"

	if got := versionString(); got != "1.2.3" {
		t.Errorf("versionString() = %q, want %q", got, "1.2.3")
	}
}
