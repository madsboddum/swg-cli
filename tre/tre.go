// Package tre reads .tre archives, the container Star Wars Galaxies keeps its
// data files in. An archive holds a flat list of members addressed by a
// slash-separated path such as "string/en/ui.stf".
package tre

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
)

const (
	magic   = "EERT"
	version = "5000"

	// versionIndexless archives are bare runs of member data. Their header is
	// zeroed past the version and they hold no record index or names at all.
	versionIndexless = "6000"

	headerSize = 36
	recordSize = 24
)

// Compression identifiers used by both the index blocks and member data.
const (
	compNone = 0
	compZlib = 2
)

// ErrFormat reports that the file is not a .tre archive this package can read.
var ErrFormat = errors.New("tre: not a supported .tre archive")

// ErrNoIndex reports a version 6000 archive. These carry no index of their own,
// so their members can only be reached through a client .toc file; see the toc
// package.
var ErrNoIndex = errors.New(`version "6000" archives carry no index; read them through the client .toc`)

// header is the fixed 36-byte prefix of every archive.
type header struct {
	Magic       [4]byte
	Version     [4]byte
	RecordCount uint32
	IndexOffset uint32
	IndexComp   uint32
	IndexSize   uint32 // on-disk size of the record index
	NamesComp   uint32
	NamesSize   uint32 // on-disk size of the name block
	NamesLength uint32 // size of the name block once decompressed
}

// record is one 24-byte entry of the archive index.
type record struct {
	Checksum   uint32 // CRC of the member name; unused for lookup
	Size       uint32
	Offset     uint32
	Comp       uint32
	StoredSize uint32
	NameOffset uint32
}

// storedLen is how many bytes the member occupies in the file. Uncompressed
// members leave StoredSize at zero and are stored at their full size.
func (r record) storedLen() int64 {
	if r.StoredSize == 0 {
		return int64(r.Size)
	}
	return int64(r.StoredSize)
}

// File is a single member of an archive.
type File struct {
	// Name is the member's path, slash separated.
	Name string
	// Size is the member's length once decompressed.
	Size int64

	r   io.ReaderAt
	rec record
}

// Reader gives access to the members of a .tre archive.
type Reader struct {
	files  []*File
	byName map[string]*File
}

// ReadCloser is a Reader over an archive file that must be closed.
type ReadCloser struct {
	Reader
	f *os.File
}

// Open opens the named .tre archive for reading.
func Open(name string) (*ReadCloser, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	rc := &ReadCloser{f: f}
	if err := rc.init(f, info.Size()); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("tre: %s: %w", name, err)
	}
	return rc, nil
}

// Close closes the underlying archive file.
func (rc *ReadCloser) Close() error { return rc.f.Close() }

// NewReader reads the archive index from r, which holds size bytes.
func NewReader(r io.ReaderAt, size int64) (*Reader, error) {
	tr := &Reader{}
	if err := tr.init(r, size); err != nil {
		return nil, err
	}
	return tr, nil
}

func (r *Reader) init(ra io.ReaderAt, size int64) error {
	var h header
	if size < headerSize {
		return fmt.Errorf("%w: file is %d bytes, too short for a header", ErrFormat, size)
	}
	if err := binary.Read(io.NewSectionReader(ra, 0, headerSize), binary.LittleEndian, &h); err != nil {
		return fmt.Errorf("tre: reading header: %w", err)
	}
	if string(h.Magic[:]) != magic {
		return fmt.Errorf("%w: magic is %q, want %q", ErrFormat, h.Magic[:], magic)
	}
	if string(h.Version[:]) != version {
		if string(h.Version[:]) == versionIndexless {
			return fmt.Errorf("%w: %w", ErrFormat, ErrNoIndex)
		}
		return fmt.Errorf("%w: version %q is not supported", ErrFormat, h.Version[:])
	}

	indexLength := int64(h.RecordCount) * recordSize
	index, err := readBlock(ra, size, int64(h.IndexOffset), h.IndexComp, int64(h.IndexSize), indexLength)
	if err != nil {
		return fmt.Errorf("tre: reading record index: %w", err)
	}
	namesOffset := int64(h.IndexOffset) + int64(h.IndexSize)
	names, err := readBlock(ra, size, namesOffset, h.NamesComp, int64(h.NamesSize), int64(h.NamesLength))
	if err != nil {
		return fmt.Errorf("tre: reading name block: %w", err)
	}

	r.files = make([]*File, 0, h.RecordCount)
	r.byName = make(map[string]*File, h.RecordCount)
	rr := bytes.NewReader(index)
	for i := range h.RecordCount {
		var rec record
		if err := binary.Read(rr, binary.LittleEndian, &rec); err != nil {
			return fmt.Errorf("tre: reading record %d: %w", i, err)
		}
		name, err := nameAt(names, rec.NameOffset)
		if err != nil {
			return fmt.Errorf("tre: record %d: %w", i, err)
		}
		if int64(rec.Offset)+rec.storedLen() > size {
			return fmt.Errorf("%w: %s extends past the end of the file", ErrFormat, name)
		}
		f := &File{Name: name, Size: int64(rec.Size), r: ra, rec: rec}
		r.files = append(r.files, f)
		// Later duplicates would shadow earlier ones; keep the first, matching
		// the order the index lists them in.
		if _, dup := r.byName[name]; !dup {
			r.byName[name] = f
		}
	}
	return nil
}

// Files returns the archive members in index order.
func (r *Reader) Files() []*File { return r.files }

// Lookup returns the member with the given path.
func (r *Reader) Lookup(name string) (*File, bool) {
	f, ok := r.byName[name]
	return f, ok
}

// ReadFile returns the decompressed contents of the named member. It reports
// fs.ErrNotExist if the archive has no such member.
func (r *Reader) ReadFile(name string) ([]byte, error) {
	f, ok := r.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("tre: %s: %w", name, fs.ErrNotExist)
	}
	return f.Bytes()
}

// Open returns a reader over the member's decompressed contents.
func (f *File) Open() (io.ReadCloser, error) {
	section := io.NewSectionReader(f.r, int64(f.rec.Offset), f.rec.storedLen())
	switch f.rec.Comp {
	case compNone:
		return io.NopCloser(section), nil
	case compZlib:
		zr, err := zlib.NewReader(section)
		if err != nil {
			return nil, fmt.Errorf("tre: %s: %w", f.Name, err)
		}
		return zr, nil
	default:
		return nil, fmt.Errorf("tre: %s: %w: compression %d", f.Name, ErrFormat, f.rec.Comp)
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
		return nil, fmt.Errorf("tre: %s: %w", f.Name, err)
	}
	return buf, nil
}

// readBlock reads one of the archive's index blocks, decompressing it to
// length bytes. stored is the block's size as it sits in the file.
func readBlock(ra io.ReaderAt, size, offset int64, comp uint32, stored, length int64) ([]byte, error) {
	if offset < headerSize || stored < 0 || length < 0 || offset+stored > size {
		return nil, fmt.Errorf("%w: block at %d spanning %d bytes lies outside the file", ErrFormat, offset, stored)
	}
	section := io.NewSectionReader(ra, offset, stored)

	var src io.Reader = section
	if comp == compZlib {
		zr, err := zlib.NewReader(section)
		if err != nil {
			return nil, err
		}
		defer func() { _ = zr.Close() }()
		src = zr
	} else if comp != compNone {
		return nil, fmt.Errorf("%w: compression %d", ErrFormat, comp)
	}

	buf := make([]byte, length)
	if _, err := io.ReadFull(src, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// nameAt returns the NUL-terminated string starting at off in the name block.
func nameAt(names []byte, off uint32) (string, error) {
	if int64(off) >= int64(len(names)) {
		return "", fmt.Errorf("%w: name offset %d is past the name block", ErrFormat, off)
	}
	rest := names[off:]
	end := bytes.IndexByte(rest, 0)
	if end < 0 {
		return "", fmt.Errorf("%w: name at offset %d is not terminated", ErrFormat, off)
	}
	return string(rest[:end]), nil
}
