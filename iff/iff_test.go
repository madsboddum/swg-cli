package iff_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/madsboddum/swg-cli/iff"
)

// fixture mirrors the tree built by testdata/gen: a root FORM of type "TEST"
// holding a leaf NAME chunk, then a nested FORM of type "CHLD" holding a leaf
// DATA chunk.
func fixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "nested.iff"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseBuildsTheNodeTree(t *testing.T) {
	root, err := iff.Parse(fixture(t))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if !root.IsForm() || root.Type != "TEST" {
		t.Fatalf("root = %+v, want a TEST form", root)
	}
	if len(root.Children) != 2 {
		t.Fatalf("root has %d children, want 2", len(root.Children))
	}

	name := root.Children[0]
	if name.IsForm() || name.Tag != "NAME" || string(name.Data) != "hello" {
		t.Errorf("children[0] = %+v, want leaf NAME %q", name, "hello")
	}

	child := root.Children[1]
	if !child.IsForm() || child.Type != "CHLD" {
		t.Fatalf("children[1] = %+v, want a CHLD form", child)
	}
	if len(child.Children) != 1 {
		t.Fatalf("CHLD has %d children, want 1", len(child.Children))
	}
	data := child.Children[0]
	want := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	if data.IsForm() || data.Tag != "DATA" || string(data.Data) != string(want) {
		t.Errorf("CHLD child = %+v, want leaf DATA %v", data, want)
	}
}

func TestParseRejectsMalformedChunks(t *testing.T) {
	good := fixture(t)

	tests := []struct {
		name  string
		bytes []byte
	}{
		{"empty", nil},
		{"short header", good[:6]},
		{"leaf claims more than remains", patch(good, 0, mustChunkHeader(t, "NAME", 0xFFFFFF))},
		{"FORM too short for a type", []byte("FORM\x00\x00\x00\x02XX")},
		{"nested chunk claims more than remains", patchNestedOverlong(good)},
		{"trailing bytes", append(good, 0)},
		{"truncated mid-chunk", good[:len(good)-1]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := iff.Parse(tt.bytes); err == nil {
				t.Error("Parse succeeded, want an error")
			}
		})
	}
}

func TestErrFormatIsReported(t *testing.T) {
	if _, err := iff.Parse(nil); !errors.Is(err, iff.ErrFormat) {
		t.Errorf("error does not wrap ErrFormat: %v", err)
	}
}

// patch returns a copy of b with the bytes starting at off replaced by with.
func patch(b []byte, off int, with []byte) []byte {
	out := append([]byte(nil), b...)
	copy(out[off:], with)
	return out
}

// mustChunkHeader lays out a chunk header naming a length no fixture body
// could satisfy, so the chunk is rejected as overlong.
func mustChunkHeader(t *testing.T, tag string, size uint32) []byte {
	t.Helper()
	b := []byte(tag)
	b = append(b, byte(size>>24), byte(size>>16), byte(size>>8), byte(size))
	return b
}

// patchNestedOverlong inflates the length of the fixture's inner NAME chunk
// past what the outer FORM actually holds for it.
func patchNestedOverlong(good []byte) []byte {
	// NAME's length field sits right after its 4-byte tag, at offset 12:
	// FORM header (8) + type (4).
	return patch(good, 16, []byte{0x00, 0x00, 0xFF, 0xFF})
}
