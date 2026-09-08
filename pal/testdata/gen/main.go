// Command gen writes the synthetic RIFF PAL fixtures the pal package tests
// read. Run it with "go generate ./pal" after changing the layout below.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

type entry struct{ r, g, b, flags uint8 }

func main() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	// go:generate runs from the package directory, one level above testdata.
	out := filepath.Join(dir, "testdata")

	write(filepath.Join(out, "basic.pal"), palette(3, []entry{
		{0x5d, 0x35, 0x34, 0},
		{0x6a, 0x3e, 0x3d, 0},
		{0xff, 0xff, 0xff, 0xff},
	}))

	// Three of the game's palettes declare fewer entries than they store and
	// understate the RIFF size to match. The extra entries are padding.
	padded := palette(1, []entry{
		{0x11, 0x22, 0x33, 0},
		{0x44, 0x55, 0x66, 0},
	})
	binary.LittleEndian.PutUint32(padded[4:], uint32(len(padded)-8-entrySize))
	write(filepath.Join(out, "padded.pal"), padded)
}

const entrySize = 4

// palette lays out a RIFF PAL file declaring count entries and storing all of
// entries, which lets a fixture carry more data than it declares.
func palette(count int, entries []entry) []byte {
	var data bytes.Buffer
	put(&data, uint16(0x0300))
	put(&data, uint16(count))
	for _, e := range entries {
		data.Write([]byte{e.r, e.g, e.b, e.flags})
	}

	var body bytes.Buffer
	body.WriteString("PAL ")
	body.WriteString("data")
	put(&body, uint32(4+count*entrySize))
	body.Write(data.Bytes())

	var buf bytes.Buffer
	buf.WriteString("RIFF")
	put(&buf, uint32(body.Len()))
	buf.Write(body.Bytes())
	return buf.Bytes()
}

func put(buf *bytes.Buffer, v any) {
	if err := binary.Write(buf, binary.LittleEndian, v); err != nil {
		log.Fatal(err)
	}
}

func write(path string, b []byte) {
	if err := os.WriteFile(path, b, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", path, len(b))
}
