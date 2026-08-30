package stf_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/madsboddum/swg-cli/stf"
)

// wantEntries mirrors the fixtures built by testdata/gen, in the order Entries
// reports them.
var wantEntries = []stf.Entry{
	{Key: "credits", Value: "Crédits — 500 cr"},
	{Key: "disturbance", Value: "You feel a disturbance in the Force."},
	{Key: "empty", Value: ""},
	{Key: "greeting", Value: "Utinni!"},
}

func TestDecodePairsKeysWithValues(t *testing.T) {
	for _, name := range []string{"basic.stf", "version0.stf"} {
		t.Run(name, func(t *testing.T) {
			table, err := stf.Decode(readFixture(t, name))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if got := table.Entries(); !slices.Equal(got, wantEntries) {
				t.Errorf("Entries() = %#v, want %#v", got, wantEntries)
			}
		})
	}
}

func TestLookup(t *testing.T) {
	table, err := stf.Decode(readFixture(t, "basic.stf"))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	got, ok := table.Lookup("disturbance")
	if !ok {
		t.Fatal(`Lookup("disturbance"): not found`)
	}
	if want := "You feel a disturbance in the Force."; got != want {
		t.Errorf("Lookup = %q, want %q", got, want)
	}
	if _, ok := table.Lookup("absent"); ok {
		t.Error("Lookup of a missing key succeeded, want not found")
	}
}

func TestDecodeRejectsMalformedTables(t *testing.T) {
	good := readFixture(t, "basic.stf")

	tests := []struct {
		name  string
		bytes []byte
	}{
		{"empty", nil},
		{"short header", good[:12]},
		{"bad magic", patch(good, 0, []byte{0, 0, 0, 0})},
		{"unsupported version", patch(good, 4, []byte{2})},
		{"entry count runs past the end", patch(good, 9, []byte{0xFF, 0xFF, 0xFF, 0x7F})},
		{"value length runs past the end", patch(good, 21, []byte{0xFF, 0xFF, 0xFF, 0x7F})},
		{"truncated", good[:len(good)/2]},
		{"trailing bytes", append(slices.Clone(good), 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := stf.Decode(tt.bytes); err == nil {
				t.Error("Decode succeeded, want an error")
			}
		})
	}
}

func TestErrFormatIsReported(t *testing.T) {
	if _, err := stf.Decode(patch(readFixture(t, "basic.stf"), 0, []byte("XXXX"))); !errors.Is(err, stf.ErrFormat) {
		t.Errorf("Decode of a non-table = %v, want stf.ErrFormat", err)
	}
}

func TestDecodeRejectsKeyWithoutValue(t *testing.T) {
	// The key block ends with "empty", entry 4; renaming it to an id no value
	// carries leaves the key unpaired.
	good := readFixture(t, "basic.stf")
	off := len(good) - len("empty") - 8
	if _, err := stf.Decode(patch(good, off, []byte{99, 0, 0, 0})); !errors.Is(err, stf.ErrFormat) {
		t.Errorf("Decode with an unpaired key = %v, want stf.ErrFormat", err)
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// patch copies b with replacement written over it at off.
func patch(b []byte, off int, replacement []byte) []byte {
	out := slices.Clone(b)
	copy(out[off:], replacement)
	return out
}
