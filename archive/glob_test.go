package archive

import (
	"errors"
	"testing"
)

func TestMatchPath(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"string/en/ui.stf", "string/en/ui.stf", true},
		{"string/en/ui.stf", "string/fr/ui.stf", false},
		{"string/en/*.stf", "string/en/ui.stf", true},
		{"string/en/*.stf", "string/en/sub/ui.stf", false},
		{"string/**", "string/en/ui.stf", true},
		{"string/en/**.stf", "string/en/sub/ui.stf", true},
		{"string/en/**.stf", "string/en/sub/ui.iff", false},
		{"**.stf", "string/en/ui.stf", true},
		{"ui.?tf", "ui.stf", true},
		{"ui.?tf", "ui.sstf", false},
		{"ui.[sx]tf", "ui.xtf", true},
		{"ui.[!sx]tf", "ui.xtf", false},
		{"ui.[!sx]tf", "ui.ytf", true},
		{`ui\*.stf`, "ui*.stf", true},
		{`ui\*.stf`, "uix.stf", false},
		{"*", "ui.stf", true},
		{"*", "string/ui.stf", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+" vs "+tt.path, func(t *testing.T) {
			got, err := MatchPath(tt.pattern, tt.path)
			if err != nil {
				t.Fatalf("MatchPath(%q, %q) error: %v", tt.pattern, tt.path, err)
			}
			if got != tt.want {
				t.Errorf("MatchPath(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchPathRejectsBadPatterns(t *testing.T) {
	for _, pattern := range []string{"ui.[stf", `ui.stf\`, `ui.[a\`} {
		t.Run(pattern, func(t *testing.T) {
			if _, err := MatchPath(pattern, "ui.stf"); !errors.Is(err, ErrBadPattern) {
				t.Errorf("MatchPath(%q, _) error = %v, want ErrBadPattern", pattern, err)
			}
		})
	}
}

func TestHasMeta(t *testing.T) {
	tests := []struct {
		pattern string
		want    bool
	}{
		{"string/en/ui.stf", false},
		{"string/en/", false},
		{"string/*", true},
		{"ui.?tf", true},
		{"ui.[s]tf", true},
		{`ui\*.stf`, true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			if got := HasMeta(tt.pattern); got != tt.want {
				t.Errorf("HasMeta(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}
