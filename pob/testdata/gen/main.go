// Command gen writes the synthetic PRTO fixtures the pob package tests read.
// Run it with "go generate ./pob" after changing the layouts below.
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
	write(filepath.Join(dir, "testdata", "basic.pob"), basic())
	write(filepath.Join(dir, "testdata", "indexed.pob"), indexed())
}

// basic is a version 0003 portal object, the shape most of the shipped files
// take: one portal polygon written as a PRTL chunk, and two version 0005 cells
// joined through it, one of them behind a door.
func basic() []byte {
	quad := prtl(
		vector{0, 0, 0},
		vector{2, 0, 0},
		vector{2, 3, 0},
		vector{0, 3, 0},
	)

	return form("PRTO",
		form("0003",
			chunk("DATA", counts(1, 2)),
			form("PRTS", quad),
			form("CELS",
				form("CELL",
					form("0005",
						chunk("DATA", cellData(1, false, "shell", "appearance/mesh/hut_r0.msh", "")),
						form("NULL"),
						form("PRTL", chunk("0004", portal0004(true, 0, true, 1, ""))),
						chunk("LGHT", lights(0)),
					),
				),
				form("CELL",
					form("0005",
						chunk("DATA", cellData(1, true, "room", "appearance/mesh/hut_r1.msh", "appearance/collision/hut_r1_floor0.flr")),
						form("NULL"),
						form("PRTL", chunk("0004", portal0004(false, 0, false, 0, "door_hut"))),
						chunk("LGHT", lights(2)),
					),
				),
			),
			chunk("CRC ", le(uint32(0xdeadbeef))),
		),
	)
}

// indexed is a version 0004 portal object, where the portal geometry is an
// indexed triangle list rather than a bare polygon and the cell portals carry
// the disabled flag version 0005 added.
func indexed() []byte {
	triangle := form("IDTL",
		form("0000",
			chunk("VERT", vertices(vector{0, 0, 0}, vector{3, 0, 0}, vector{0, 3, 0})),
			chunk("INDX", le(int32(0), int32(1), int32(2))),
		),
	)

	return form("PRTO",
		form("0004",
			chunk("DATA", counts(1, 2)),
			form("PRTS", triangle),
			form("CELS",
				form("CELL",
					form("0005",
						chunk("DATA", cellData(1, false, "shell", "appearance/mesh/shed_r0.msh", "")),
						form("PRTL", chunk("0005", portal0005(false, true, 0, true, 1, ""))),
						chunk("LGHT", lights(0)),
					),
				),
				form("CELL",
					form("0005",
						chunk("DATA", cellData(1, true, "room", "appearance/mesh/shed_r1.msh", "")),
						form("PRTL", chunk("0005", portal0005(true, true, 0, false, 0, ""))),
						chunk("LGHT", lights(0)),
					),
				),
			),
			chunk("CRC ", le(uint32(1))),
		),
	)
}

type vector struct{ x, y, z float32 }

// counts lays out the portal object's DATA chunk: how many portals the PRTS
// form holds and how many cells the CELS form does.
func counts(portals, cells int32) []byte {
	return le(portals, cells)
}

// prtl lays out one portal's geometry chunk: a vertex count, then the polygon.
func prtl(vs ...vector) []byte {
	var buf bytes.Buffer
	buf.Write(le(int32(len(vs))))
	buf.Write(vertices(vs...))
	return chunk("PRTL", buf.Bytes())
}

// vertices lays out vectors as three little-endian float32s apiece.
func vertices(vs ...vector) []byte {
	var buf bytes.Buffer
	for _, v := range vs {
		buf.Write(le(math.Float32bits(v.x), math.Float32bits(v.y), math.Float32bits(v.z)))
	}
	return buf.Bytes()
}

// cellData lays out a version 0005 cell's DATA chunk. An empty floor name is
// written as the flag saying the cell has none.
func cellData(portals int32, canSeeParent bool, name, appearance, floor string) []byte {
	var buf bytes.Buffer
	buf.Write(le(portals))
	buf.WriteByte(flag(canSeeParent))
	cstring(&buf, name)
	cstring(&buf, appearance)
	buf.WriteByte(flag(floor != ""))
	if floor != "" {
		cstring(&buf, floor)
	}
	return buf.Bytes()
}

// portal0004 lays out a version 0004 cell portal, which ends in the flag and
// transform placing a door in the opening.
func portal0004(passable bool, geometry int32, clockwise bool, target int32, door string) []byte {
	var buf bytes.Buffer
	buf.WriteByte(flag(passable))
	buf.Write(le(geometry))
	buf.WriteByte(flag(clockwise))
	buf.Write(le(target))
	cstring(&buf, door)
	buf.WriteByte(flag(door != ""))
	buf.Write(identity())
	return buf.Bytes()
}

// portal0005 lays out a version 0005 cell portal: a version 0004 one behind
// the disabled flag.
func portal0005(disabled, passable bool, geometry int32, clockwise bool, target int32, door string) []byte {
	var buf bytes.Buffer
	buf.WriteByte(flag(disabled))
	buf.Write(portal0004(passable, geometry, clockwise, target, door))
	return buf.Bytes()
}

// lights lays out a LGHT chunk: a count, then that many light records. A light
// is a type byte, two colours, a transform and three attenuation floats, 93
// bytes that nothing here reads, so they are left zeroed.
func lights(n int) []byte {
	return append(le(int32(n)), make([]byte, 93*n)...)
}

// identity lays out a door hardpoint transform: three rows of four floats,
// each row's last float its translation.
func identity() []byte {
	rows := [3][4]float32{
		{1, 0, 0, 0},
		{0, 1, 0, 0},
		{0, 0, 1, 0},
	}
	var buf bytes.Buffer
	for _, row := range rows {
		for _, v := range row {
			buf.Write(le(math.Float32bits(v)))
		}
	}
	return buf.Bytes()
}

func flag(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func cstring(buf *bytes.Buffer, s string) {
	buf.WriteString(s)
	buf.WriteByte(0)
}

// le lays out values little-endian, the byte order everything inside a chunk
// uses; only the chunk headers themselves are big-endian.
func le(vs ...any) []byte {
	var buf bytes.Buffer
	for _, v := range vs {
		if err := binary.Write(&buf, binary.LittleEndian, v); err != nil {
			log.Fatal(err)
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

func write(path string, b []byte) {
	if err := os.WriteFile(path, b, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", path, len(b))
}
