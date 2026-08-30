package tre_test

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/madsboddum/swg-cli/tre"
)

// wantMembers mirrors the fixtures built by testdata/gen.
var wantMembers = []struct {
	name string
	data []byte
}{
	{"string/en/ui.stf", []byte("ui_zoom\x00Zoom\x00")},
	{"appearance/mesh/crate.msh", bytes.Repeat([]byte("FORM"), 64)},
	{"texture/crate.dds", []byte{0xDD, 0x53, 0x00, 0xFF, 0xFE, 0x01}},
}

// fixtures are the same three members stored uncompressed and zlib compressed.
var fixtures = []string{"plain.tre", "zlib.tre"}

func TestOpenListsMembersInIndexOrder(t *testing.T) {
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			r := open(t, name)

			var got []string
			for _, f := range r.Files() {
				got = append(got, f.Name)
			}
			var want []string
			for _, m := range wantMembers {
				want = append(want, m.name)
			}
			if !slices.Equal(got, want) {
				t.Errorf("Files() = %q, want %q", got, want)
			}
		})
	}
}

func TestReadFileReturnsMemberBytes(t *testing.T) {
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			r := open(t, name)

			for _, m := range wantMembers {
				got, err := r.ReadFile(m.name)
				if err != nil {
					t.Errorf("ReadFile(%q): %v", m.name, err)
					continue
				}
				if !bytes.Equal(got, m.data) {
					t.Errorf("ReadFile(%q) = %q, want %q", m.name, got, m.data)
				}
			}
		})
	}
}

func TestFileSizeAndOpen(t *testing.T) {
	r := open(t, "zlib.tre")

	f, ok := r.Lookup("appearance/mesh/crate.msh")
	if !ok {
		t.Fatal("Lookup: member not found")
	}
	want := wantMembers[1].data
	if f.Size != int64(len(want)) {
		t.Errorf("Size = %d, want %d", f.Size, len(want))
	}

	rc, err := f.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = rc.Close() }()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("contents = %q, want %q", got, want)
	}
}

func TestReadFileMissingMember(t *testing.T) {
	r := open(t, "plain.tre")

	_, err := r.ReadFile("string/en/absent.stf")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFile of a missing member = %v, want fs.ErrNotExist", err)
	}
}

func TestNewReaderRejectsMalformedArchives(t *testing.T) {
	good := readFixture(t, "plain.tre")

	tests := []struct {
		name  string
		bytes []byte
	}{
		{"empty", nil},
		{"short header", good[:20]},
		{"bad magic", patch(good, 0, []byte("TREE"))},
		{"unsupported version", patch(good, 4, []byte("6000"))},
		{"index offset past end", patch(good, 12, []byte{0xFF, 0xFF, 0xFF, 0x7F})},
		{"unknown index compression", patch(good, 16, []byte{9, 0, 0, 0})},
		{"truncated", good[:len(good)/2]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tre.NewReader(bytes.NewReader(tt.bytes), int64(len(tt.bytes))); err == nil {
				t.Error("NewReader succeeded, want an error")
			}
		})
	}
}

func TestVersion6000ReportsErrNoIndex(t *testing.T) {
	b := patch(readFixture(t, "plain.tre"), 4, []byte("6000"))

	_, err := tre.NewReader(bytes.NewReader(b), int64(len(b)))
	if !errors.Is(err, tre.ErrNoIndex) {
		t.Errorf("NewReader on a v6000 archive = %v, want tre.ErrNoIndex", err)
	}
	if !errors.Is(err, tre.ErrFormat) {
		t.Errorf("NewReader on a v6000 archive = %v, want it to also match tre.ErrFormat", err)
	}
}

func TestErrFormatIsReported(t *testing.T) {
	b := patch(readFixture(t, "plain.tre"), 0, []byte("XXXX"))

	_, err := tre.NewReader(bytes.NewReader(b), int64(len(b)))
	if !errors.Is(err, tre.ErrFormat) {
		t.Errorf("NewReader on a non-archive = %v, want tre.ErrFormat", err)
	}
}

func open(t *testing.T, name string) *tre.ReadCloser {
	t.Helper()
	r, err := tre.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("Open(%q): %v", name, err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
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
