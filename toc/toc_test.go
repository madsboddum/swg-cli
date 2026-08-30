package toc_test

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/madsboddum/swg-cli/toc"
)

// wantMembers mirrors the fixtures built by testdata/gen.
var wantMembers = []struct {
	name    string
	archive string
	data    []byte
}{
	{"string/en/ui.stf", "patch_00.tre", []byte("ui_zoom\x00Zoom\x00")},
	{"appearance/mesh/crate.msh", "patch_00.tre", bytes.Repeat([]byte("FORM"), 64)},
	{"texture/crate.dds", "patch_01.tre", []byte{0xDD, 0x53, 0x00, 0xFF, 0xFE, 0x01}},
	{"string/en/skl_n.stf", "patch_01.tre", bytes.Repeat([]byte("brawler\x00"), 32)},
}

func TestOpenListsArchivesAndMembers(t *testing.T) {
	r := open(t)

	if got, want := r.Archives(), []string{"patch_00.tre", "patch_01.tre"}; !slices.Equal(got, want) {
		t.Errorf("Archives() = %q, want %q", got, want)
	}

	var got, want []string
	for _, f := range r.Files() {
		got = append(got, f.Name)
	}
	for _, m := range wantMembers {
		want = append(want, m.name)
	}
	if !slices.Equal(got, want) {
		t.Errorf("Files() = %q, want %q", got, want)
	}
}

func TestReadFileSpansArchives(t *testing.T) {
	r := open(t)

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
}

func TestFileReportsArchiveAndSize(t *testing.T) {
	r := open(t)

	for _, m := range wantMembers {
		f, ok := r.Lookup(m.name)
		if !ok {
			t.Errorf("Lookup(%q): not found", m.name)
			continue
		}
		if f.Archive != m.archive {
			t.Errorf("%q Archive = %q, want %q", m.name, f.Archive, m.archive)
		}
		if f.Size != int64(len(m.data)) {
			t.Errorf("%q Size = %d, want %d", m.name, f.Size, len(m.data))
		}
	}
}

func TestFileOpenStreamsContents(t *testing.T) {
	r := open(t)

	f, ok := r.Lookup("string/en/skl_n.stf")
	if !ok {
		t.Fatal("Lookup: member not found")
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
	if want := wantMembers[3].data; !bytes.Equal(got, want) {
		t.Errorf("contents = %q, want %q", got, want)
	}
}

func TestReadFileMissingMember(t *testing.T) {
	r := open(t)

	_, err := r.ReadFile("string/en/absent.stf")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFile of a missing member = %v, want fs.ErrNotExist", err)
	}
}

func TestOpenRejectsMalformedTables(t *testing.T) {
	good := readFixture(t)

	tests := []struct {
		name  string
		bytes []byte
	}{
		{"empty", nil},
		{"short header", good[:20]},
		{"bad magic", patch(good, 0, []byte("TOC "))},
		{"unsupported version", patch(good, 4, []byte("2000"))},
		{"index size not a whole number of records", patch(good, 16, []byte{23, 0, 0, 0})},
		{"archive names run past the end", patch(good, 32, []byte{0xFF, 0xFF, 0xFF, 0x7F})},
		{"truncated", good[:len(good)/2]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "client.toc")
			if err := os.WriteFile(path, tt.bytes, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := toc.Open(path); err == nil {
				t.Error("Open succeeded, want an error")
			}
		})
	}
}

func TestErrFormatIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.toc")
	if err := os.WriteFile(path, patch(readFixture(t), 0, []byte("XXXX")), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := toc.Open(path); !errors.Is(err, toc.ErrFormat) {
		t.Errorf("Open on a non-table = %v, want toc.ErrFormat", err)
	}
}

func TestReadFileMissingArchive(t *testing.T) {
	// A table copied away from its archives parses, but reads fail.
	dir := t.TempDir()
	path := filepath.Join(dir, "client.toc")
	if err := os.WriteFile(path, readFixture(t), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := toc.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = r.Close() }()

	if _, err := r.ReadFile("string/en/ui.stf"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFile with the archive absent = %v, want fs.ErrNotExist", err)
	}
}

func open(t *testing.T) *toc.Reader {
	t.Helper()
	r, err := toc.Open(filepath.Join("testdata", "client.toc"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func readFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "client.toc"))
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
