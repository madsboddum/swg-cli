// Command gen writes the synthetic IFF fixture the iff package tests read.
// Run it with "go generate ./iff" after changing the layout below.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	// go:generate runs from the package directory, one level above testdata.
	out := filepath.Join(dir, "testdata", "nested.iff")

	// A root FORM of type "TEST" holding a leaf, then a nested FORM of type
	// "CHLD" holding a leaf of its own.
	root := form("TEST",
		chunk("NAME", []byte("hello")),
		form("CHLD",
			chunk("DATA", []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}),
		),
	)

	if err := os.WriteFile(out, root, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", out, len(root))
}

// chunk lays out a leaf chunk: a 4CC tag, its big-endian length, then data.
func chunk(tag string, data []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString(tag)
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(data))); err != nil {
		log.Fatal(err)
	}
	buf.Write(data)
	return buf.Bytes()
}

// form lays out a FORM chunk: the FORM tag, the length of what follows, the
// 4CC form type, then the concatenated bytes of its children.
func form(typ string, children ...[]byte) []byte {
	var body bytes.Buffer
	body.WriteString(typ)
	for _, c := range children {
		body.Write(c)
	}
	return chunk("FORM", body.Bytes())
}
