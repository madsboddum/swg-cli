package archive

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/madsboddum/swg-cli/tre"
)

func TestOpenIndexesEveryArchive(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "base.tre"), map[string]string{
		"string/en/ui.stf":  "base ui",
		"texture/crate.dds": "base crate",
	})
	writeArchive(t, filepath.Join(dir, "patch_01.tre"), map[string]string{
		"string/en/cmd.stf": "patch cmd",
	})

	s := open(t, dir)

	want := []string{"string/en/cmd.stf", "string/en/ui.stf", "texture/crate.dds"}
	if got := s.Paths(); !slices.Equal(got, want) {
		t.Errorf("Paths() = %v, want %v", got, want)
	}
	if got := s.Archives(); !slices.Equal(got, []string{"base.tre", "patch_01.tre"}) {
		t.Errorf("Archives() = %v", got)
	}
	if got := s.Dir(); got != dir {
		t.Errorf("Dir() = %q, want %q", got, dir)
	}
}

func TestSourcesRankLooseFilesOverArchives(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "base.tre"), map[string]string{
		"string/en/ui.stf": "base",
	})
	writeArchive(t, filepath.Join(dir, "patch_01.tre"), map[string]string{
		"string/en/ui.stf": "patch one",
	})
	writeArchive(t, filepath.Join(dir, "patch_02.tre"), map[string]string{
		"string/en/ui.stf": "patch two",
	})
	writeLoose(t, dir, "string/en/ui.stf", "loose")

	s := open(t, dir)

	got := s.Sources("string/en/ui.stf")
	want := []Source{{}, {Archive: "patch_02.tre"}, {Archive: "patch_01.tre"}, {Archive: "base.tre"}}
	if !slices.Equal(got, want) {
		t.Fatalf("Sources() = %v, want %v", got, want)
	}
	if !got[0].Loose() {
		t.Error("the winning source is not loose")
	}
	if got[0].String() != "loose" {
		t.Errorf("loose Source.String() = %q", got[0].String())
	}
	if got[1].String() != "patch_02.tre" {
		t.Errorf("archive Source.String() = %q", got[1].String())
	}

	// A shadowed path is still listed only once.
	if p := s.Paths(); !slices.Equal(p, []string{"string/en/ui.stf"}) {
		t.Errorf("Paths() = %v, want one entry", p)
	}
}

func TestSourcesReportsUnknownPath(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "base.tre"), map[string]string{"a.stf": "a"})

	if got := open(t, dir).Sources("nope.stf"); got != nil {
		t.Errorf("Sources() = %v, want nil", got)
	}
}

func TestLooseFilesAddPathsOfTheirOwn(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "base.tre"), map[string]string{"a.stf": "a"})
	writeLoose(t, dir, "string/en/loose.stf", "loose")

	s := open(t, dir)

	want := []string{"a.stf", "string/en/loose.stf"}
	if got := s.Paths(); !slices.Equal(got, want) {
		t.Errorf("Paths() = %v, want %v", got, want)
	}
}

// Files sitting directly beside the archives are the client's own, not data.
func TestFilesBesideTheArchivesAreNotPaths(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "base.tre"), map[string]string{"a.stf": "a"})
	writeLoose(t, dir, "client.cfg", "config")

	if got := open(t, dir).Paths(); !slices.Equal(got, []string{"a.stf"}) {
		t.Errorf("Paths() = %v, want just the archive path", got)
	}
}

func TestReadFileTakesTheWinningSource(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "base.tre"), map[string]string{
		"string/en/ui.stf":  "base ui",
		"texture/crate.dds": "base crate",
	})
	writeArchive(t, filepath.Join(dir, "patch_01.tre"), map[string]string{
		"string/en/ui.stf": "patched ui",
	})
	writeLoose(t, dir, "texture/crate.dds", "loose crate")

	s := open(t, dir)

	for path, want := range map[string]string{
		"string/en/ui.stf":  "patched ui",
		"texture/crate.dds": "loose crate",
	} {
		b, err := s.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", path, err)
		}
		if string(b) != want {
			t.Errorf("ReadFile(%q) = %q, want %q", path, b, want)
		}
	}
}

func TestReadFileFromNamesTheArchive(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "base.tre"), map[string]string{"a.stf": "base a"})
	writeArchive(t, filepath.Join(dir, "patch_01.tre"), map[string]string{"a.stf": "patched a"})

	s := open(t, dir)

	b, err := s.ReadFileFrom("a.stf", Source{Archive: "base.tre"})
	if err != nil {
		t.Fatalf("ReadFileFrom(): %v", err)
	}
	if string(b) != "base a" {
		t.Errorf("ReadFileFrom() = %q, want the shadowed copy", b)
	}
	if _, err := s.ReadFileFrom("a.stf", Source{Archive: "absent.tre"}); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFileFrom(absent) error = %v, want fs.ErrNotExist", err)
	}
}

func TestReadFileThroughTheTableOfContents(t *testing.T) {
	dir := t.TempDir()
	writeTable(t, dir, "sku0_client.toc", []tocMember{
		{archive: "patch_00.tre", name: "string/en/ui.stf", data: "base ui"},
		{archive: "patch_01.tre", name: "string/en/ui.stf", data: "patched ui"},
	})

	s := open(t, dir)

	b, err := s.ReadFile("string/en/ui.stf")
	if err != nil {
		t.Fatalf("ReadFile(): %v", err)
	}
	if string(b) != "patched ui" {
		t.Errorf("ReadFile() = %q, want %q", b, "patched ui")
	}

	b, err = s.ReadFileFrom("string/en/ui.stf", Source{Archive: "patch_00.tre"})
	if err != nil {
		t.Fatalf("ReadFileFrom(): %v", err)
	}
	if string(b) != "base ui" {
		t.Errorf("ReadFileFrom() = %q, want %q", b, "base ui")
	}
}

func TestReadFileReportsUnknownPath(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "base.tre"), map[string]string{"a.stf": "a"})

	if _, err := open(t, dir).ReadFile("nope.stf"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFile() error = %v, want fs.ErrNotExist", err)
	}
}

func TestArchivePathsIgnorePrecedence(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "base.tre"), map[string]string{
		"b.stf": "b",
		"a.stf": "a",
	})
	writeArchive(t, filepath.Join(dir, "patch_01.tre"), map[string]string{"a.stf": "shadowing"})

	s := open(t, dir)

	got, err := s.ArchivePaths("base.tre")
	if err != nil {
		t.Fatalf("ArchivePaths() error: %v", err)
	}
	if !slices.Equal(got, []string{"a.stf", "b.stf"}) {
		t.Errorf("ArchivePaths() = %v", got)
	}

	if _, err := s.ArchivePaths("missing.tre"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ArchivePaths(missing) error = %v, want fs.ErrNotExist", err)
	}
}

func TestIndexlessArchivesAreReadThroughTheTable(t *testing.T) {
	dir := t.TempDir()
	writeTable(t, dir, "sku0_client.toc", []tocMember{
		{archive: "patch_00.tre", name: "string/en/ui.stf", data: "base ui"},
		{archive: "patch_00.tre", name: "texture/crate.dds", data: "base crate"},
		{archive: "patch_01.tre", name: "string/en/ui.stf", data: "patched ui"},
	})

	s := open(t, dir)

	want := []string{"string/en/ui.stf", "texture/crate.dds"}
	if got := s.Paths(); !slices.Equal(got, want) {
		t.Errorf("Paths() = %v, want %v", got, want)
	}
	got := s.Sources("string/en/ui.stf")
	if !slices.Equal(got, []Source{{Archive: "patch_01.tre"}, {Archive: "patch_00.tre"}}) {
		t.Errorf("Sources() = %v", got)
	}
}

func TestTableAndSelfIndexedArchivesMix(t *testing.T) {
	dir := t.TempDir()
	writeTable(t, dir, "sku0_client.toc", []tocMember{
		{archive: "patch_00.tre", name: "string/en/ui.stf", data: "table ui"},
	})
	writeArchive(t, filepath.Join(dir, "bottom.tre"), map[string]string{"a.stf": "a"})

	s := open(t, dir)

	if got := s.Paths(); !slices.Equal(got, []string{"a.stf", "string/en/ui.stf"}) {
		t.Errorf("Paths() = %v", got)
	}
	if got := s.Archives(); !slices.Equal(got, []string{"bottom.tre", "patch_00.tre"}) {
		t.Errorf("Archives() = %v", got)
	}
}

// A table naming archives that are not installed is normal: the client ships
// one table per SKU.
func TestTableEntriesForMissingArchivesAreSkipped(t *testing.T) {
	dir := t.TempDir()
	writeTable(t, dir, "sku0_client.toc", []tocMember{
		{archive: "patch_00.tre", name: "string/en/ui.stf", data: "ui"},
		{archive: "absent.tre", name: "string/en/gone.stf", data: "gone"},
	})
	if err := os.Remove(filepath.Join(dir, "absent.tre")); err != nil {
		t.Fatal(err)
	}

	if got := open(t, dir).Paths(); !slices.Equal(got, []string{"string/en/ui.stf"}) {
		t.Errorf("Paths() = %v", got)
	}
}

func TestOpenRejectsIndexlessArchiveWithoutTable(t *testing.T) {
	dir := t.TempDir()
	writeTable(t, dir, "sku0_client.toc", []tocMember{
		{archive: "patch_00.tre", name: "string/en/ui.stf", data: "ui"},
	})
	if err := os.Remove(filepath.Join(dir, "sku0_client.toc")); err != nil {
		t.Fatal(err)
	}

	_, err := Open(dir)
	if !errors.Is(err, tre.ErrNoIndex) {
		t.Fatalf("Open() error = %v, want tre.ErrNoIndex", err)
	}
	if !strings.Contains(err.Error(), ".toc") {
		t.Errorf("Open() error = %q, want it to point at the .toc", err)
	}
}

func TestOpenRejectsDirectoryWithoutArchives(t *testing.T) {
	dir := t.TempDir()
	writeLoose(t, dir, "string/en/ui.stf", "loose")

	if _, err := Open(dir); !errors.Is(err, ErrNoArchives) {
		t.Errorf("Open() error = %v, want ErrNoArchives", err)
	}
}

func TestOpenReportsMissingDirectory(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "nope")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open() error = %v, want fs.ErrNotExist", err)
	}
}

func TestOpenReportsUnreadableArchive(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.tre"), []byte("not an archive"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(dir); err == nil {
		t.Error("Open() succeeded on a broken archive")
	}
}

func open(t *testing.T, dir string) *Stack {
	t.Helper()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open(%q): %v", dir, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func writeLoose(t *testing.T, dir, path, data string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// tocMember is one member of a table of contents fixture.
type tocMember struct {
	archive string
	name    string
	data    string
}

// writeTable builds a table of contents named name over the version 6000
// archives holding members, writing the archives out alongside it. Archives
// are laid out in the order the members first name them.
func writeTable(t *testing.T, dir, name string, members []tocMember) {
	t.Helper()

	const headerSize = 36
	var archives []string
	blobs := map[string]*bytes.Buffer{}

	var index, names bytes.Buffer
	for _, m := range members {
		blob, ok := blobs[m.archive]
		if !ok {
			blob = &bytes.Buffer{}
			blob.WriteString("EERT")
			blob.WriteString("6000")
			blob.Write(make([]byte, headerSize-8))
			blobs[m.archive] = blob
			archives = append(archives, m.archive)
		}
		rec := []any{
			uint16(0), // uncompressed
			uint16(slices.Index(archives, m.archive)),
			uint32(0), // name checksum, unused
			uint32(len(m.name)),
			uint32(blob.Len()),
			uint32(len(m.data)),
			uint32(len(m.data)),
		}
		for _, v := range rec {
			if err := binary.Write(&index, binary.LittleEndian, v); err != nil {
				t.Fatal(err)
			}
		}
		blob.WriteString(m.data)
		names.WriteString(m.name)
		names.WriteByte(0)
	}

	var archiveNames bytes.Buffer
	for _, a := range archives {
		archiveNames.WriteString(a)
		archiveNames.WriteByte(0)
	}

	var out bytes.Buffer
	out.WriteString(" COT")
	out.WriteString("1000")
	for _, v := range []uint32{
		0,
		uint32(len(members)),
		uint32(index.Len()),
		uint32(names.Len()),
		uint32(names.Len()),
		uint32(len(archives)),
		uint32(archiveNames.Len()),
	} {
		if err := binary.Write(&out, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	out.Write(archiveNames.Bytes())
	out.Write(index.Bytes())
	out.Write(names.Bytes())

	if err := os.WriteFile(filepath.Join(dir, name), out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	for a, blob := range blobs {
		if err := os.WriteFile(filepath.Join(dir, a), blob.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// writeArchive builds a minimal uncompressed .tre archive holding members.
func writeArchive(t *testing.T, path string, members map[string]string) {
	t.Helper()

	names := make([]string, 0, len(members))
	for n := range members {
		names = append(names, n)
	}
	sort.Strings(names)

	const headerSize = 36
	var data, nameBlock, index bytes.Buffer
	for _, n := range names {
		rec := [6]uint32{
			0,                               // name checksum, unused
			uint32(len(members[n])),         // size
			uint32(headerSize + data.Len()), // offset
			0,                               // uncompressed
			0,                               // stored size, zero when uncompressed
			uint32(nameBlock.Len()),         // name offset
		}
		if err := binary.Write(&index, binary.LittleEndian, rec); err != nil {
			t.Fatal(err)
		}
		data.WriteString(members[n])
		nameBlock.WriteString(n)
		nameBlock.WriteByte(0)
	}

	var out bytes.Buffer
	out.WriteString("EERT")
	out.WriteString("5000")
	for _, v := range []uint32{
		uint32(len(names)),
		uint32(headerSize + data.Len()),
		0,
		uint32(index.Len()),
		0,
		uint32(nameBlock.Len()),
		uint32(nameBlock.Len()),
	} {
		if err := binary.Write(&out, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	out.Write(data.Bytes())
	out.Write(index.Bytes())
	out.Write(nameBlock.Bytes())

	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
