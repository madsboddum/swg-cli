// Package dtable decodes DTII datatables, the schema most Star Wars Galaxies
// game data ships in: a fixed set of typed columns with rows of packed
// values underneath. It consumes a tree already parsed by iff; it does no
// byte-level container work of its own.
package dtable

//go:generate go run ./testdata/gen

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/madsboddum/swg-cli/iff"
)

// FormType is the 4CC a datatable's root FORM carries.
const FormType = "DTII"

// ErrFormat reports that the node tree is not a DTII datatable this package
// can read.
var ErrFormat = errors.New("dtable: not a supported DTII datatable")

// Type is the storage kind a column's cells are packed as. The client's
// richer column specs (enums, bit vectors, hash strings, packed obj vars)
// all reduce to one of these three on disk.
type Type int

const (
	Int Type = iota
	Float
	String
)

func (t Type) String() string {
	switch t {
	case Int:
		return "int"
	case Float:
		return "float"
	case String:
		return "string"
	default:
		return "unknown"
	}
}

// Column is one field of the table: its name and the storage type its cells
// are packed as.
type Column struct {
	Name string
	Type Type
}

// Row is one record, one cell per column in Table.Columns order. Each cell
// holds an int32, a float32 or a string, matching its column's Type.
type Row []any

// Table is a decoded datatable.
type Table struct {
	Columns []Column
	Rows    []Row
}

// Decode reads a datatable out of root, the node tree of a FORM DTII.
func Decode(root *iff.Node) (*Table, error) {
	if !root.IsForm() || root.Type != FormType {
		return nil, fmt.Errorf("%w: root is not a %s form", ErrFormat, FormType)
	}
	if len(root.Children) != 1 || !root.Children[0].IsForm() {
		return nil, fmt.Errorf("%w: expected a single nested form", ErrFormat)
	}

	version := root.Children[0]
	cols, err := child(version, "COLS")
	if err != nil {
		return nil, err
	}
	typ, err := child(version, "TYPE")
	if err != nil {
		return nil, err
	}
	rows, err := child(version, "ROWS")
	if err != nil {
		return nil, err
	}

	names, err := readColumnNames(cols.Data)
	if err != nil {
		return nil, err
	}

	var types []Type
	switch version.Type {
	case "0000":
		types, err = readColumnTypesV0000(typ.Data, len(names))
	case "0001":
		types, err = readColumnTypesV0001(typ.Data, len(names))
	default:
		return nil, fmt.Errorf("%w: unsupported datatable version %q", ErrFormat, version.Type)
	}
	if err != nil {
		return nil, err
	}

	columns := make([]Column, len(names))
	for i, name := range names {
		columns[i] = Column{Name: name, Type: types[i]}
	}

	table := &Table{Columns: columns}
	table.Rows, err = readRows(rows.Data, types)
	if err != nil {
		return nil, err
	}
	return table, nil
}

// child returns the leaf named tag among n's children.
func child(n *iff.Node, tag string) (*iff.Node, error) {
	for _, c := range n.Children {
		if c.Tag == tag {
			return c, nil
		}
	}
	return nil, fmt.Errorf("%w: %s form has no %s chunk", ErrFormat, n.Type, tag)
}

// readColumnNames reads the column count and that many NUL-terminated names
// from a COLS chunk.
func readColumnNames(b []byte) ([]string, error) {
	count, b, err := readInt32Count(b, "COLS")
	if err != nil {
		return nil, err
	}
	names := make([]string, count)
	for i := range names {
		names[i], b, err = readCString(b)
		if err != nil {
			return nil, fmt.Errorf("%w: COLS name %d: %v", ErrFormat, i, err)
		}
	}
	return names, nil
}

// readColumnTypesV0000 reads count int32 type codes, the layout version 0000
// datatables use: only int, float and string columns exist.
func readColumnTypesV0000(b []byte, count int) ([]Type, error) {
	types := make([]Type, count)
	for i := range types {
		if len(b) < 4 {
			return nil, fmt.Errorf("%w: TYPE code %d runs past the end of the chunk", ErrFormat, i)
		}
		switch binary.LittleEndian.Uint32(b) {
		case 0:
			types[i] = Int
		case 1:
			types[i] = Float
		case 2:
			types[i] = String
		default:
			return nil, fmt.Errorf("%w: TYPE code %d is not int, float or string", ErrFormat, i)
		}
		b = b[4:]
	}
	return types, nil
}

// readColumnTypesV0001 reads count NUL-terminated type specs, the layout
// version 0001 datatables use. A spec's first letter names the column's
// storage type; everything after it (an enum's members, a default value)
// only matters to a writer, not to reading the raw cells back.
func readColumnTypesV0001(b []byte, count int) ([]Type, error) {
	types := make([]Type, count)
	for i := range types {
		var spec string
		var err error
		spec, b, err = readCString(b)
		if err != nil {
			return nil, fmt.Errorf("%w: TYPE spec %d: %v", ErrFormat, i, err)
		}
		if spec == "" {
			return nil, fmt.Errorf("%w: TYPE spec %d is empty", ErrFormat, i)
		}
		switch spec[0] {
		case 'i', 'h', 'b', 'e', 'v':
			types[i] = Int
		case 'f':
			types[i] = Float
		case 's', 'c', 'p':
			types[i] = String
		default:
			return nil, fmt.Errorf("%w: TYPE spec %d has unsupported letter %q", ErrFormat, i, spec[0])
		}
	}
	return types, nil
}

// readRows reads the row count and that many records from a ROWS chunk, each
// cell packed according to its column's type in turn.
func readRows(b []byte, types []Type) ([]Row, error) {
	count, b, err := readInt32Count(b, "ROWS")
	if err != nil {
		return nil, err
	}
	rows := make([]Row, count)
	for r := range rows {
		row := make(Row, len(types))
		for c, t := range types {
			var cell any
			cell, b, err = readCell(b, t)
			if err != nil {
				return nil, fmt.Errorf("%w: row %d, column %d: %v", ErrFormat, r, c, err)
			}
			row[c] = cell
		}
		rows[r] = row
	}
	return rows, nil
}

// readCell reads one value of the given type from the front of b.
func readCell(b []byte, t Type) (any, []byte, error) {
	switch t {
	case Int:
		if len(b) < 4 {
			return nil, nil, errors.New("truncated int")
		}
		return int32(binary.LittleEndian.Uint32(b)), b[4:], nil
	case Float:
		if len(b) < 4 {
			return nil, nil, errors.New("truncated float")
		}
		return math.Float32frombits(binary.LittleEndian.Uint32(b)), b[4:], nil
	default:
		return readCString(b)
	}
}

// readInt32Count reads the little-endian count that leads a COLS or ROWS
// chunk. Every entry costs at least one byte, so a count past that bound
// cannot be honest; rejecting it up front keeps a corrupt chunk from sizing
// the allocation that follows.
func readInt32Count(b []byte, chunk string) (int, []byte, error) {
	if len(b) < 4 {
		return 0, nil, fmt.Errorf("%w: %s chunk is too short for a count", ErrFormat, chunk)
	}
	count := int32(binary.LittleEndian.Uint32(b))
	rest := b[4:]
	if count < 0 || int64(count) > int64(len(rest)) {
		return 0, nil, fmt.Errorf("%w: %s chunk claims %d entries, too many for its size", ErrFormat, chunk, count)
	}
	return int(count), rest, nil
}

// readCString reads a NUL-terminated string from the front of b, returning
// the bytes that follow it.
func readCString(b []byte) (string, []byte, error) {
	i := bytes.IndexByte(b, 0)
	if i < 0 {
		return "", nil, errors.New("unterminated string")
	}
	return string(b[:i]), b[i+1:], nil
}
