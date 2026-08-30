// Command gen writes the synthetic DTII fixture the dtable package tests
// read. Run it with "go generate ./dtable" after changing the layout below.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	// go:generate runs from the package directory, one level above testdata.
	out := filepath.Join(dir, "testdata", "basic.iff")

	// A version 0001 table of one column per storage type: an int, a float
	// and a string, with two rows of data.
	root := form("DTII",
		form("0001",
			chunk("COLS", cstrings("level", "weight", "name")),
			chunk("TYPE", concat("i", "f", "s")),
			chunk("ROWS", rows(
				row{int32(1), float32(2.5), "womp rat"},
				row{int32(2), float32(-1), "krayt dragon"},
			)),
		),
	)

	if err := os.WriteFile(out, root, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", out, len(root))
}

type row []any

// cstrings lays out a COLS chunk body: a little-endian count, then that many
// NUL-terminated strings.
func cstrings(s ...string) []byte {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, uint32(len(s))); err != nil {
		log.Fatal(err)
	}
	buf.Write(concat(s...))
	return buf.Bytes()
}

// concat lays out a TYPE chunk body: NUL-terminated strings with no leading
// count, the column count already known from COLS.
func concat(s ...string) []byte {
	var buf bytes.Buffer
	for _, v := range s {
		buf.WriteString(v)
		buf.WriteByte(0)
	}
	return buf.Bytes()
}

// rows lays out a ROWS chunk body: a little-endian row count, then each
// row's cells packed as a little-endian int32, a little-endian float32 or a
// NUL-terminated string, according to its column's type.
func rows(rs ...row) []byte {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, uint32(len(rs))); err != nil {
		log.Fatal(err)
	}
	for _, r := range rs {
		for _, v := range r {
			switch v := v.(type) {
			case int32:
				if err := binary.Write(&buf, binary.LittleEndian, v); err != nil {
					log.Fatal(err)
				}
			case float32:
				if err := binary.Write(&buf, binary.LittleEndian, math.Float32bits(v)); err != nil {
					log.Fatal(err)
				}
			case string:
				buf.WriteString(v)
				buf.WriteByte(0)
			default:
				log.Fatalf("unsupported cell type %T", v)
			}
		}
	}
	return buf.Bytes()
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
