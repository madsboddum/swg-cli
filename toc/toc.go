// Package toc reads the client table of contents, the sku*_client.toc files
// that sit alongside a Star Wars Galaxies install's .tre archives.
//
// A table of contents is a single index over many archives: each entry names a
// member, the archive holding its bytes, and where in that archive to find
// them. Version 6000 .tre archives carry no index of their own — they are bare
// runs of member data — so the table of contents is the only way to address
// their contents by path. It indexes version 5000 archives just the same.
package toc

//go:generate go run ./testdata/gen

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

const (
	magic   = " COT"
	version = "1000"

	headerSize = 36
	recordSize = 24
)

// Compression identifiers, shared with the .tre format.
const (
	compNone = 0
	compZlib = 2
)

// ErrFormat reports that the file is not a table of contents this package can
// read.
var ErrFormat = errors.New("toc: not a supported .toc file")

// header is the fixed 36-byte prefix of every table of contents.
type header struct {
	Magic   [4]byte
	Version [4]byte
	_       uint32 // zero in every table observed
	// RecordCount is the number of indexed members.
	RecordCount uint32
	IndexSize   uint32 // on-disk size of the record index
	NamesSize   uint32 // on-disk size of the name block
	NamesLength uint32 // size of the name block once decompressed
	// ArchiveCount is the number of .tre files the table spans.
	ArchiveCount     uint32
	ArchiveNamesSize uint32
}

// record is one 24-byte entry of the index. Unlike .tre records these carry no
// name offset: names sit in the name block in record order.
type record struct {
	Comp       uint16
	Archive    uint16 // index into the table's archive list
	Checksum   uint32 // CRC of the member name; unused for lookup
	NameLength uint32
	Offset     uint32
	Size       uint32
	StoredSize uint32
}

// storedLen is how many bytes the member occupies in its archive.
func (r record) storedLen() int64 {
	if r.Comp == compNone {
		return int64(r.Size)
	}
	return int64(r.StoredSize)
}

// File is a single member indexed by a table of contents.
type File struct {
	// Name is the member's path, slash separated.
	Name string
	// Size is the member's length once decompressed.
	Size int64
	// Archive is the base name of the .tre file holding the member's bytes.
	Archive string

	r   *Reader
	rec record
}

// Reader gives access to the members a table of contents indexes.
type Reader struct {
	dir      string
	archives []string
	files    []*File
	byName   map[string]*File

	mu   sync.Mutex
	open map[string]*os.File // archives opened so far, keyed by base name
}

// Open reads the table of contents at path. Member data is read from the .tre
// archives in the same directory, opened lazily; call Close to release them.
func Open(path string) (*Reader, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r := &Reader{dir: filepath.Dir(path), open: map[string]*os.File{}}
	if err := r.init(b); err != nil {
		return nil, fmt.Errorf("toc: %s: %w", path, err)
	}
	return r, nil
}

// Close releases every archive the Reader opened.
func (r *Reader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var err error
	for _, f := range r.open {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}
	clear(r.open)
	return err
}

func (r *Reader) init(b []byte) error {
	var h header
	if len(b) < headerSize {
		return fmt.Errorf("%w: file is %d bytes, too short for a header", ErrFormat, len(b))
	}
	if err := binary.Read(bytes.NewReader(b[:headerSize]), binary.LittleEndian, &h); err != nil {
		return fmt.Errorf("toc: reading header: %w", err)
	}
	if string(h.Magic[:]) != magic {
		return fmt.Errorf("%w: magic is %q, want %q", ErrFormat, h.Magic[:], magic)
	}
	if string(h.Version[:]) != version {
		return fmt.Errorf("%w: version %q is not supported", ErrFormat, h.Version[:])
	}

	// The three blocks follow the header back to back, so each one's start is
	// the previous one's end.
	archiveNames, rest, err := split(b[headerSize:], int64(h.ArchiveNamesSize), "archive name block")
	if err != nil {
		return err
	}
	index, rest, err := split(rest, int64(h.IndexSize), "record index")
	if err != nil {
		return err
	}
	storedNames, _, err := split(rest, int64(h.NamesSize), "name block")
	if err != nil {
		return err
	}
	names, err := inflate(storedNames, int64(h.NamesLength))
	if err != nil {
		return fmt.Errorf("toc: reading name block: %w", err)
	}

	r.archives = make([]string, 0, h.ArchiveCount)
	for name := range bytes.SplitSeq(archiveNames, []byte{0}) {
		if len(name) > 0 {
			r.archives = append(r.archives, string(name))
		}
	}
	if len(r.archives) != int(h.ArchiveCount) {
		return fmt.Errorf("%w: header claims %d archives, name block holds %d", ErrFormat, h.ArchiveCount, len(r.archives))
	}

	if int64(h.IndexSize) != int64(h.RecordCount)*recordSize {
		return fmt.Errorf("%w: index of %d bytes does not hold %d records", ErrFormat, h.IndexSize, h.RecordCount)
	}

	r.files = make([]*File, 0, h.RecordCount)
	r.byName = make(map[string]*File, h.RecordCount)
	rr := bytes.NewReader(index)
	var nameStart int64
	for i := range h.RecordCount {
		var rec record
		if err := binary.Read(rr, binary.LittleEndian, &rec); err != nil {
			return fmt.Errorf("toc: reading record %d: %w", i, err)
		}
		name, next, err := nameAt(names, nameStart, int64(rec.NameLength))
		if err != nil {
			return fmt.Errorf("toc: record %d: %w", i, err)
		}
		nameStart = next
		if int(rec.Archive) >= len(r.archives) {
			return fmt.Errorf("%w: %s names archive %d of %d", ErrFormat, name, rec.Archive, len(r.archives))
		}
		f := &File{Name: name, Size: int64(rec.Size), Archive: r.archives[rec.Archive], r: r, rec: rec}
		r.files = append(r.files, f)
		// Later duplicates would shadow earlier ones; keep the first, matching
		// the order the index lists them in.
		if _, dup := r.byName[name]; !dup {
			r.byName[name] = f
		}
	}
	return nil
}

// Archives returns the .tre file names the table spans, in table order.
func (r *Reader) Archives() []string { return r.archives }

// Files returns the indexed members in index order.
func (r *Reader) Files() []*File { return r.files }

// Lookup returns the member with the given path.
func (r *Reader) Lookup(name string) (*File, bool) {
	f, ok := r.byName[name]
	return f, ok
}

// ReadFile returns the decompressed contents of the named member. It reports
// fs.ErrNotExist if the table has no such member.
func (r *Reader) ReadFile(name string) ([]byte, error) {
	f, ok := r.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("toc: %s: %w", name, fs.ErrNotExist)
	}
	return f.Bytes()
}

// archive returns the named .tre file, opening it on first use.
func (r *Reader) archive(name string) (*os.File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if f, ok := r.open[name]; ok {
		return f, nil
	}
	f, err := os.Open(filepath.Join(r.dir, name))
	if err != nil {
		return nil, err
	}
	r.open[name] = f
	return f, nil
}

// Open returns a reader over the member's decompressed contents.
func (f *File) Open() (io.ReadCloser, error) {
	archive, err := f.r.archive(f.Archive)
	if err != nil {
		return nil, err
	}
	section := io.NewSectionReader(archive, int64(f.rec.Offset), f.rec.storedLen())
	switch f.rec.Comp {
	case compNone:
		return io.NopCloser(section), nil
	case compZlib:
		zr, err := zlib.NewReader(section)
		if err != nil {
			return nil, fmt.Errorf("toc: %s: %w", f.Name, err)
		}
		return zr, nil
	default:
		return nil, fmt.Errorf("toc: %s: %w: compression %d", f.Name, ErrFormat, f.rec.Comp)
	}
}

// Bytes returns the member's decompressed contents.
func (f *File) Bytes() ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	buf := make([]byte, f.Size)
	if _, err := io.ReadFull(rc, buf); err != nil {
		return nil, fmt.Errorf("toc: %s: %w", f.Name, err)
	}
	return buf, nil
}

// split takes the leading n bytes of b, returning them and what follows.
func split(b []byte, n int64, what string) (block, rest []byte, err error) {
	if n < 0 || n > int64(len(b)) {
		return nil, nil, fmt.Errorf("%w: %s of %d bytes runs past the end of the file", ErrFormat, what, n)
	}
	return b[:n], b[n:], nil
}

// inflate expands a block to length bytes, passing it through unchanged when it
// is stored uncompressed.
func inflate(b []byte, length int64) ([]byte, error) {
	if int64(len(b)) == length {
		return b, nil
	}
	zr, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()
	buf := make([]byte, length)
	if _, err := io.ReadFull(zr, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// nameAt returns the name of length n starting at off, and the offset of the
// name after it. Names are NUL terminated and stored in record order.
func nameAt(names []byte, off, n int64) (string, int64, error) {
	end := off + n
	if n < 0 || end >= int64(len(names)) {
		return "", 0, fmt.Errorf("%w: name of %d bytes at offset %d runs past the name block", ErrFormat, n, off)
	}
	if names[end] != 0 {
		return "", 0, fmt.Errorf("%w: name at offset %d is not terminated", ErrFormat, off)
	}
	return string(names[off:end]), end + 1, nil
}
