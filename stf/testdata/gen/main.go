// Command gen writes the synthetic .stf fixtures the stf package tests read.
// Run it with "go generate ./stf" after changing the fixtures.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"unicode/utf16"
)

const magic = 0x0000abcd

type entry struct {
	id    uint32
	key   string
	value string
}

// entries pin the layout: ids are neither ordered nor contiguous, the key block
// lists them in a different order than the value block, and one value carries
// characters outside ASCII to prove the strings are wide.
var entries = []entry{
	{id: 3, key: "disturbance", value: "You feel a disturbance in the Force."},
	{id: 1, key: "greeting", value: "Utinni!"},
	{id: 7, key: "credits", value: "Crédits — 500 cr"},
	{id: 4, key: "empty", value: ""},
}

// keyOrder is the order the key block lists the entries in, by index into
// entries.
var keyOrder = []int{1, 2, 0, 3}

func main() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	// go:generate runs from the package directory, one level above testdata.
	out := filepath.Join(dir, "testdata")

	for _, f := range []struct {
		name    string
		version byte
	}{{"basic.stf", 1}, {"version0.stf", 0}} {
		b := build(f.version)
		path := filepath.Join(out, f.name)
		if err := os.WriteFile(path, b, 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("wrote %s (%d bytes)\n", path, len(b))
	}
}

// build lays out a table: a header, then every value, then every key.
func build(version byte) []byte {
	var out bytes.Buffer
	write := func(v any) {
		if err := binary.Write(&out, binary.LittleEndian, v); err != nil {
			log.Fatal(err)
		}
	}

	write(uint32(magic))
	out.WriteByte(version)
	// The next id the client would hand out, left a step past the highest in use
	// so the tests cannot mistake it for the entry count.
	write(uint32(9))
	write(uint32(len(entries)))

	for _, e := range entries {
		u := utf16.Encode([]rune(e.value))
		write(e.id)
		// The checksum of the value. Version 0 tables leave it zero; swg ignores
		// it either way, so a recognisable constant pins the field's position.
		if version == 0 {
			write(uint32(0))
		} else {
			write(uint32(0xDEADBEEF))
		}
		write(uint32(len(u)))
		for _, c := range u {
			write(c)
		}
	}
	for _, i := range keyOrder {
		e := entries[i]
		write(e.id)
		write(uint32(len(e.key)))
		out.WriteString(e.key)
	}
	return out.Bytes()
}
