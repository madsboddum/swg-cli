package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCompletion(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{name: "bash", args: []string{"bash"}, wantOut: "complete -o nospace -F _swg_complete swg"},
		{name: "zsh", args: []string{"zsh"}, wantOut: "#compdef swg"},
		{name: "fish", args: []string{"fish"}, wantOut: "complete -c swg -f"},
		{name: "unknown shell", args: []string{"nope"}, wantCode: 2, wantErr: `unknown shell "nope"`},
		{name: "no shell", args: nil, wantCode: 2, wantErr: "usage: swg completion"},
		{name: "extra operand", args: []string{"bash", "zsh"}, wantCode: 2, wantErr: "usage: swg completion"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(append([]string{"completion"}, tt.args...), &stdout, &stderr)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, tt.wantCode, stderr.String())
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

func TestHelpCompletionShowsTheLongUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"help", "completion"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "bash|zsh|fish") {
		t.Errorf("stdout = %q, want the usage documented", stdout.String())
	}
}
