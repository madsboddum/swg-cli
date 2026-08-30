// Package iff parses the IFF container, the generic nested-chunk format
// underneath most Star Wars Galaxies data files: datatables, object
// templates, appearances, shaders and more. This package only builds the
// node tree; interpreting what a given form type means is left to packages
// built on top of it.
package iff

//go:generate go run ./testdata/gen

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// FormTag is the fixed tag a container node carries. Its real identity is
// the 4CC that follows, held in Node.Type.
const FormTag = "FORM"

// chunkHeaderSize is the tag plus the big-endian length that precedes every
// chunk, form or leaf alike.
const chunkHeaderSize = 8

// ErrFormat reports that the bytes are not a container this package can read.
var ErrFormat = errors.New("iff: not a supported IFF container")

// Node is one chunk of the tree. A FORM node has Tag set to FormTag, Type set
// to its 4CC form type, and Children holding its nested nodes. A leaf node
// has its own 4CC in Tag, Type empty, and Data holding its raw payload.
type Node struct {
	Tag      string
	Type     string
	Size     int
	Children []*Node
	Data     []byte
}

// IsForm reports whether n is a container node rather than a leaf chunk.
func (n *Node) IsForm() bool { return n.Tag == FormTag }

// Parse reads b as a single top-level IFF chunk, usually a FORM. It rejects
// truncated chunks, chunks whose declared length overruns what remains, and
// any bytes left over once the chunk has been read.
func Parse(b []byte) (*Node, error) {
	n, rest, err := parseChunk(b)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("%w: %d bytes after the top-level chunk", ErrFormat, len(rest))
	}
	return n, nil
}

// parseChunk reads one chunk from the front of b, returning it along with
// whatever follows it.
func parseChunk(b []byte) (*Node, []byte, error) {
	if len(b) < chunkHeaderSize {
		return nil, nil, fmt.Errorf("%w: %d bytes, too short for a chunk header", ErrFormat, len(b))
	}
	tag := string(b[:4])
	size := binary.BigEndian.Uint32(b[4:8])
	body := b[chunkHeaderSize:]
	if uint64(size) > uint64(len(body)) {
		return nil, nil, fmt.Errorf("%w: %s chunk claims %d bytes, only %d remain", ErrFormat, tag, size, len(body))
	}
	data, rest := body[:size], body[size:]

	if tag != FormTag {
		return &Node{Tag: tag, Size: int(size), Data: data}, rest, nil
	}

	if len(data) < 4 {
		return nil, nil, fmt.Errorf("%w: FORM chunk of %d bytes, too short for a type", ErrFormat, len(data))
	}
	children, err := parseChildren(data[4:])
	if err != nil {
		return nil, nil, err
	}
	return &Node{Tag: FormTag, Type: string(data[:4]), Size: int(size), Children: children}, rest, nil
}

// parseChildren reads chunks from b until it is exhausted.
func parseChildren(b []byte) ([]*Node, error) {
	var children []*Node
	for len(b) > 0 {
		n, rest, err := parseChunk(b)
		if err != nil {
			return nil, err
		}
		children = append(children, n)
		b = rest
	}
	return children, nil
}
