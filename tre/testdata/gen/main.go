// Command gen writes the synthetic .tre archives the tre package tests read.
// Run it with "go generate ./tre" after changing the fixtures below.
package main

import (
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

const (
	compNone = 0
	compZlib = 2
)

type member struct {
	name string
	data []byte
	// checksum stands in for the name CRC real archives carry. swg ignores it,
	// so a recognisable constant is enough to pin the field's position.
	checksum uint32
}

// members are deliberately mixed: a short file, one that compresses well, and
// one holding bytes that are not valid UTF-8.
var members = []member{
	{name: "string/en/ui.stf", data: []byte("ui_zoom\x00Zoom\x00"), checksum: 0x11111111},
	{name: "appearance/mesh/crate.msh", data: bytes.Repeat([]byte("FORM"), 64), checksum: 0x22222222},
	{name: "texture/crate.dds", data: []byte{0xDD, 0x53, 0x00, 0xFF, 0xFE, 0x01}, checksum: 0x33333333},
}

func main() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	// go:generate runs from the package directory, one level above testdata.
	out := filepath.Join(dir, "testdata")

	for _, f := range []struct {
		name string
		comp uint32
	}{
		{"plain.tre", compNone},
		{"zlib.tre", compZlib},
	} {
		archive, err := build(members, f.comp)
		if err != nil {
			log.Fatal(err)
		}
		path := filepath.Join(out, f.name)
		if err := os.WriteFile(path, archive, 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("wrote %s (%d bytes)\n", path, len(archive))
	}
}

// build lays out a complete archive: header, member data, record index, name
// block, then one MD5 digest per member.
func build(ms []member, comp uint32) ([]byte, error) {
	var data, names bytes.Buffer
	records := make([][6]uint32, len(ms))

	for i, m := range ms {
		stored, err := pack(m.data, comp)
		if err != nil {
			return nil, err
		}
		// Real archives leave the stored size at zero for uncompressed members.
		storedSize := uint32(len(stored))
		if comp == compNone {
			storedSize = 0
		}
		records[i] = [6]uint32{
			m.checksum,
			uint32(len(m.data)),
			uint32(headerSize + data.Len()),
			comp,
			storedSize,
			uint32(names.Len()),
		}
		data.Write(stored)
		names.WriteString(m.name)
		names.WriteByte(0)
	}

	var index bytes.Buffer
	for _, r := range records {
		if err := binary.Write(&index, binary.LittleEndian, r); err != nil {
			return nil, err
		}
	}
	storedIndex, err := pack(index.Bytes(), comp)
	if err != nil {
		return nil, err
	}
	storedNames, err := pack(names.Bytes(), comp)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString("EERT")
	out.WriteString("5000")
	for _, v := range []uint32{
		uint32(len(ms)),
		uint32(headerSize + data.Len()),
		comp,
		uint32(len(storedIndex)),
		comp,
		uint32(len(storedNames)),
		uint32(names.Len()),
	} {
		if err := binary.Write(&out, binary.LittleEndian, v); err != nil {
			return nil, err
		}
	}
	out.Write(data.Bytes())
	out.Write(storedIndex)
	out.Write(storedNames)
	for _, m := range ms {
		sum := md5.Sum(m.data)
		out.Write(sum[:])
	}
	return out.Bytes(), nil
}

const headerSize = 36

func pack(b []byte, comp uint32) ([]byte, error) {
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
