// Command gen writes the synthetic .toc and .tre fixtures the toc package
// tests read. Run it with "go generate ./toc" after changing the fixtures.
package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

const (
	compNone = 0
	compZlib = 2

	headerSize = 36
)

type member struct {
	name    string
	data    []byte
	archive int // index into archives
	comp    uint16
	// checksum stands in for the name CRC real tables carry. swg ignores it, so
	// a recognisable constant is enough to pin the field's position.
	checksum uint32
}

// archives are the version 6000 .tre files the table indexes: a header with a
// zeroed body, then member data end to end and nothing else.
var archives = []string{"patch_00.tre", "patch_01.tre"}

// members deliberately mix compression and span both archives, so a swapped
// archive index or size field shows up as a bad read.
var members = []member{
	{name: "string/en/ui.stf", data: []byte("ui_zoom\x00Zoom\x00"), archive: 0, comp: compNone, checksum: 0x11111111},
	{name: "appearance/mesh/crate.msh", data: bytes.Repeat([]byte("FORM"), 64), archive: 0, comp: compZlib, checksum: 0x22222222},
	{name: "texture/crate.dds", data: []byte{0xDD, 0x53, 0x00, 0xFF, 0xFE, 0x01}, archive: 1, comp: compNone, checksum: 0x33333333},
	{name: "string/en/skl_n.stf", data: bytes.Repeat([]byte("brawler\x00"), 32), archive: 1, comp: compZlib, checksum: 0x44444444},
}

func main() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	// go:generate runs from the package directory, one level above testdata.
	out := filepath.Join(dir, "testdata")

	table, blobs, err := build(archives, members)
	if err != nil {
		log.Fatal(err)
	}
	write := func(name string, b []byte) {
		path := filepath.Join(out, name)
		if err := os.WriteFile(path, b, 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("wrote %s (%d bytes)\n", path, len(b))
	}
	for i, name := range archives {
		write(name, blobs[i])
	}
	write("client.toc", table)
}

// build lays out the archives and the table indexing them.
func build(archives []string, ms []member) (table []byte, blobs [][]byte, err error) {
	bufs := make([]*bytes.Buffer, len(archives))
	for i := range bufs {
		b := &bytes.Buffer{}
		b.WriteString("EERT")
		b.WriteString("6000")
		b.Write(make([]byte, headerSize-8))
		bufs[i] = b
	}

	var index, names bytes.Buffer
	for _, m := range ms {
		stored, err := pack(m.data, m.comp)
		if err != nil {
			return nil, nil, err
		}
		dst := bufs[m.archive]
		rec := []any{
			m.comp,
			uint16(m.archive),
			m.checksum,
			uint32(len(m.name)),
			uint32(dst.Len()),
			uint32(len(m.data)),
			uint32(len(stored)),
		}
		for _, v := range rec {
			if err := binary.Write(&index, binary.LittleEndian, v); err != nil {
				return nil, nil, err
			}
		}
		dst.Write(stored)
		names.WriteString(m.name)
		names.WriteByte(0)
	}

	var archiveNames bytes.Buffer
	for _, name := range archives {
		archiveNames.WriteString(name)
		archiveNames.WriteByte(0)
	}

	var out bytes.Buffer
	out.WriteString(" COT")
	out.WriteString("1000")
	for _, v := range []uint32{
		0,
		uint32(len(ms)),
		uint32(index.Len()),
		uint32(names.Len()),
		uint32(names.Len()),
		uint32(len(archives)),
		uint32(archiveNames.Len()),
	} {
		if err := binary.Write(&out, binary.LittleEndian, v); err != nil {
			return nil, nil, err
		}
	}
	out.Write(archiveNames.Bytes())
	out.Write(index.Bytes())
	out.Write(names.Bytes())

	blobs = make([][]byte, len(bufs))
	for i, b := range bufs {
		blobs[i] = b.Bytes()
	}
	return out.Bytes(), blobs, nil
}

func pack(b []byte, comp uint16) ([]byte, error) {
	if comp == compNone {
		return b, nil
	}
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(b); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
