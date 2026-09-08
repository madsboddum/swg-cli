package pal_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/madsboddum/swg-cli/pal"
)

// fixture reads one of the palettes built by testdata/gen.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDecodeReadsEntriesInOrder(t *testing.T) {
	p, err := pal.Decode(fixture(t, "basic.pal"))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	want := []pal.Color{
		{R: 0x5d, G: 0x35, B: 0x34},
		{R: 0x6a, G: 0x3e, B: 0x3d},
		{R: 0xff, G: 0xff, B: 0xff},
	}
	if len(p.Colors) != len(want) {
		t.Fatalf("Colors = %+v, want %+v", p.Colors, want)
	}
	for i, c := range want {
		if p.Colors[i] != c {
			t.Errorf("Colors[%d] = %+v, want %+v", i, p.Colors[i], c)
		}
	}
}

// The flags byte of the last basic.pal entry is 0xff. Reading it as alpha
// would be wrong, and reading it as colour data would shift every channel.
func TestDecodeIgnoresTheFlagsByte(t *testing.T) {
	p, err := pal.Decode(fixture(t, "basic.pal"))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if _, _, _, a := p.Colors[2].RGBA(); a != 0xffff {
		t.Errorf("alpha = %#04x, want 0xffff", a)
	}
}

// Three of the game's palettes store more entries than they declare and
// understate the RIFF size to match; the count is what the client honours.
func TestDecodeHonoursTheCountOverTrailingData(t *testing.T) {
	p, err := pal.Decode(fixture(t, "padded.pal"))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(p.Colors) != 1 {
		t.Fatalf("got %d colours, want 1", len(p.Colors))
	}
	if want := (pal.Color{R: 0x11, G: 0x22, B: 0x33}); p.Colors[0] != want {
		t.Errorf("Colors[0] = %+v, want %+v", p.Colors[0], want)
	}
}

func TestColorHex(t *testing.T) {
	if got := (pal.Color{R: 0x5d, G: 0x35, B: 0x34}).Hex(); got != "#5d3534" {
		t.Errorf("Hex = %q, want %q", got, "#5d3534")
	}
}

func TestHasMagic(t *testing.T) {
	if !pal.HasMagic(fixture(t, "basic.pal")) {
		t.Error("HasMagic = false for a palette")
	}
	if pal.HasMagic([]byte("RIFF\x00\x00\x00\x00WAVEfmt ")) {
		t.Error("HasMagic = true for a RIFF file that is not a palette")
	}
	if pal.HasMagic([]byte("RIFF")) {
		t.Error("HasMagic = true for a truncated header")
	}
}

func TestDecodeRejects(t *testing.T) {
	good := fixture(t, "basic.pal")

	corrupt := func(edit func(b []byte) []byte) []byte {
		b := append([]byte(nil), good...)
		return edit(b)
	}

	tests := []struct {
		name string
		in   []byte
	}{
		{"too short for a header", good[:16]},
		{"not RIFF", corrupt(func(b []byte) []byte { copy(b, "RIFX"); return b })},
		{"not a palette form", corrupt(func(b []byte) []byte { copy(b[8:], "WAVE"); return b })},
		{"missing the data chunk", corrupt(func(b []byte) []byte { copy(b[12:], "fmt "); return b })},
		{"data chunk runs past the end", corrupt(func(b []byte) []byte { b[16] = 0xff; return b })},
		{"unsupported version", corrupt(func(b []byte) []byte { b[21] = 0x04; return b })},
		{"count runs past the end", corrupt(func(b []byte) []byte { b[22] = 0xff; return b })},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := pal.Decode(tt.in); !errors.Is(err, pal.ErrFormat) {
				t.Errorf("Decode error = %v, want ErrFormat", err)
			}
		})
	}
}
