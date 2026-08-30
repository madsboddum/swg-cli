// Package stf reads .stf string tables, the files Star Wars Galaxies keeps its
// display text in. A table maps a key such as "disturbance" to the wide string
// the client shows, and a string id names one of them as file plus key.
package stf

//go:generate go run ./testdata/gen

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"unicode/utf16"
)

const (
	magic = 0x0000abcd

	headerSize = 13
	// Fixed part of an entry: id, checksum and length for a value, id and length
	// for a key.
	valueHeaderSize = 12
	keyHeaderSize   = 8
)

// ErrFormat reports that the file is not a string table this package can read.
var ErrFormat = errors.New("stf: not a supported .stf file")

// Entry is one key and the string it names.
type Entry struct {
	Key   string
	Value string
}

// Table is a decoded string table.
type Table struct {
	entries []Entry
	byKey   map[string]string
}

// Decode reads a string table from b.
//
// The values come first, each tagged with an id, then the keys tagged with the
// same ids; Decode pairs them up.
func Decode(b []byte) (*Table, error) {
	if len(b) < headerSize {
		return nil, fmt.Errorf("%w: file is %d bytes, too short for a header", ErrFormat, len(b))
	}
	if got := binary.LittleEndian.Uint32(b); got != magic {
		return nil, fmt.Errorf("%w: magic is %#08x, want %#08x", ErrFormat, got, magic)
	}
	// Byte 4 is the format version, 0 or 1. Both lay their entries out the same
	// way; only the per-entry checksum differs, and that goes unused here.
	if v := b[4]; v > 1 {
		return nil, fmt.Errorf("%w: version %d is not supported", ErrFormat, v)
	}
	// The uint32 that follows the version is the next id the client would hand
	// out. It runs ahead of the count once entries have been deleted.
	count := int64(binary.LittleEndian.Uint32(b[9:]))
	// An entry costs at least a value and a key header, so a count past that
	// bound cannot be honest. Checking it up front keeps a corrupt file from
	// sizing the allocations below.
	if max := int64(len(b)-headerSize) / (valueHeaderSize + keyHeaderSize); count > max {
		return nil, fmt.Errorf("%w: header claims %d entries, the file has room for %d", ErrFormat, count, max)
	}

	values, rest, err := readValues(b[headerSize:], count)
	if err != nil {
		return nil, err
	}
	keys, rest, err := readKeys(rest, count)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("%w: %d bytes past the last key", ErrFormat, len(rest))
	}

	t := &Table{entries: make([]Entry, 0, count), byKey: make(map[string]string, count)}
	for _, k := range keys {
		v, ok := values[k.id]
		if !ok {
			return nil, fmt.Errorf("%w: key %q names entry %d, which has no value", ErrFormat, k.key, k.id)
		}
		t.entries = append(t.entries, Entry{Key: k.key, Value: v})
		// A repeated key would shadow the earlier one; keep the first.
		if _, dup := t.byKey[k.key]; !dup {
			t.byKey[k.key] = v
		}
	}
	sort.Slice(t.entries, func(i, j int) bool { return t.entries[i].Key < t.entries[j].Key })
	return t, nil
}

// Entries returns the table's pairs, sorted by key.
func (t *Table) Entries() []Entry { return t.entries }

// Lookup returns the string the key names.
func (t *Table) Lookup(key string) (string, bool) {
	v, ok := t.byKey[key]
	return v, ok
}

// keyEntry is one entry of the key block, tying a key to the value carrying the
// same id.
type keyEntry struct {
	id  uint32
	key string
}

// readValues reads count entries of id, checksum and a UTF-16 string, returning
// the strings by id along with the bytes that follow the block.
func readValues(b []byte, count int64) (map[uint32]string, []byte, error) {
	values := make(map[uint32]string, count)
	for i := range count {
		if len(b) < valueHeaderSize {
			return nil, nil, fmt.Errorf("%w: value %d runs past the end of the file", ErrFormat, i)
		}
		id := binary.LittleEndian.Uint32(b)
		// b[4:8] is a checksum of the value, which swg has no use for.
		n := int64(binary.LittleEndian.Uint32(b[8:])) * 2
		b = b[valueHeaderSize:]
		if n < 0 || n > int64(len(b)) {
			return nil, nil, fmt.Errorf("%w: value %d runs past the end of the file", ErrFormat, i)
		}
		if _, dup := values[id]; dup {
			return nil, nil, fmt.Errorf("%w: entry %d has two values", ErrFormat, id)
		}
		values[id] = utf16le(b[:n])
		b = b[n:]
	}
	return values, b, nil
}

// readKeys reads count entries of id and an ASCII key, in the order the block
// lists them, along with the bytes that follow.
func readKeys(b []byte, count int64) ([]keyEntry, []byte, error) {
	keys := make([]keyEntry, 0, count)
	for i := range count {
		if len(b) < keyHeaderSize {
			return nil, nil, fmt.Errorf("%w: key %d runs past the end of the file", ErrFormat, i)
		}
		id := binary.LittleEndian.Uint32(b)
		n := int64(binary.LittleEndian.Uint32(b[4:]))
		b = b[keyHeaderSize:]
		if n < 0 || n > int64(len(b)) {
			return nil, nil, fmt.Errorf("%w: key %d runs past the end of the file", ErrFormat, i)
		}
		keys = append(keys, keyEntry{id: id, key: string(b[:n])})
		b = b[n:]
	}
	return keys, b, nil
}

// utf16le decodes b as little endian UTF-16.
func utf16le(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}
