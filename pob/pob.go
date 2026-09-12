// Package pob decodes PRTO portal objects, the interior layout the client
// loads for anything a player can walk inside: a building's cells, the portals
// punched between them and the doors that hang in those portals. The files
// carry a .pob extension and a PRTO form inside. It consumes a tree already
// parsed by iff; it does no byte-level container work of its own.
package pob

//go:generate go run ./testdata/gen

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/madsboddum/swg-cli/iff"
)

// FormType is the 4CC a portal object's root FORM carries.
const FormType = "PRTO"

// crcTag is the layout checksum's chunk. Three-letter tags are padded to four,
// so the trailing space is part of it.
const crcTag = "CRC "

// ErrFormat reports that the node tree is not a PRTO portal object this
// package can read.
var ErrFormat = errors.New("pob: not a supported PRTO portal object")

// Vector is a point in the building's own space, the coordinates the client
// places cell contents in.
type Vector struct {
	X, Y, Z float32
}

// Portal is one portal's geometry: the convex polygon of the opening between
// two cells. Both cells reference it, in opposite windings.
type Portal struct {
	Vertices []Vector
}

// Center is the average of the portal's vertices, which for a convex polygon
// falls inside the opening and is the handiest single coordinate for one. A
// portal with no vertices has a zero centre.
func (p Portal) Center() Vector {
	if len(p.Vertices) == 0 {
		return Vector{}
	}
	var sum Vector
	for _, v := range p.Vertices {
		sum.X += v.X
		sum.Y += v.Y
		sum.Z += v.Z
	}
	n := float32(len(p.Vertices))
	return Vector{X: sum.X / n, Y: sum.Y / n, Z: sum.Z / n}
}

// CellPortal is one cell's side of a portal: which geometry it uses, which
// cell lies through it, and what hangs in it.
type CellPortal struct {
	// Portal indexes Layout.Portals.
	Portal int
	// Target indexes Layout.Cells: the cell on the other side.
	Target int
	// Passable reports whether anything can move through the opening. An
	// impassable portal still lets the cell beyond it be seen.
	Passable bool
	// Disabled reports a portal the layout ships switched off.
	Disabled bool
	// Clockwise is the winding the geometry is read in from this side. The two
	// cells sharing a portal disagree on it, which is how each knows which face
	// of the polygon it is looking at.
	Clockwise bool
	// Door names the door appearance hanging in the portal, empty for an open
	// one.
	Door string
}

// Cell is one room of the building.
type Cell struct {
	Name       string
	Appearance string
	// Floor names the floor mesh the server walks NPCs across, empty if the
	// cell has none.
	Floor string
	// CanSeeParent reports whether the cell beyond the portal renders when this
	// one is the camera's.
	CanSeeParent bool
	Portals      []CellPortal
	// Lights is how many lights the cell holds. The lights themselves are not
	// decoded.
	Lights int
}

// Layout is a decoded portal object. Cell 0 is the shell the building presents
// to the world; the rest are the rooms inside it.
type Layout struct {
	Portals []Portal
	Cells   []Cell
	// CRC identifies the layout. The server hands it to clients to check that
	// both sides agree on the cell numbering.
	CRC uint32
}

// Decode reads a portal object out of root, the node tree of a FORM PRTO.
func Decode(root *iff.Node) (*Layout, error) {
	if !root.IsForm() || root.Type != FormType {
		return nil, fmt.Errorf("%w: root is not a %s form", ErrFormat, FormType)
	}
	if len(root.Children) != 1 || !root.Children[0].IsForm() {
		return nil, fmt.Errorf("%w: expected a single nested form", ErrFormat)
	}

	version := root.Children[0]
	switch version.Type {
	case "0000", "0001", "0002", "0003", "0004":
	default:
		return nil, fmt.Errorf("%w: unsupported portal object version %q", ErrFormat, version.Type)
	}

	data, err := chunkChild(version, "DATA")
	if err != nil {
		return nil, err
	}
	portalCount, rest, err := readInt32(data.Data)
	if err != nil {
		return nil, fmt.Errorf("%w: DATA portal count: %v", ErrFormat, err)
	}
	cellCount, _, err := readInt32(rest)
	if err != nil {
		return nil, fmt.Errorf("%w: DATA cell count: %v", ErrFormat, err)
	}

	prts, err := formChild(version, "PRTS")
	if err != nil {
		return nil, err
	}
	cels, err := formChild(version, "CELS")
	if err != nil {
		return nil, err
	}

	layout := &Layout{}
	if layout.Portals, err = readPortals(prts); err != nil {
		return nil, err
	}
	if layout.Cells, err = readCells(cels); err != nil {
		return nil, err
	}
	if int(portalCount) != len(layout.Portals) {
		return nil, fmt.Errorf("%w: DATA claims %d portals, PRTS holds %d", ErrFormat, portalCount, len(layout.Portals))
	}
	if int(cellCount) != len(layout.Cells) {
		return nil, fmt.Errorf("%w: DATA claims %d cells, CELS holds %d", ErrFormat, cellCount, len(layout.Cells))
	}
	if err := checkIndices(layout); err != nil {
		return nil, err
	}

	// Version 0000 predates the checksum, and the path graph that may sit
	// between the cells and it is left to whoever needs it.
	if crc, err := chunkChild(version, crcTag); err == nil {
		if len(crc.Data) < 4 {
			return nil, fmt.Errorf("%w: %s chunk is too short for a checksum", ErrFormat, crcTag)
		}
		layout.CRC = binary.LittleEndian.Uint32(crc.Data)
	}
	return layout, nil
}

// checkIndices rejects a layout whose portals point at geometry or cells that
// are not there, so callers can index both lists on the strength of a decode.
func checkIndices(l *Layout) error {
	for i, cell := range l.Cells {
		for j, p := range cell.Portals {
			if p.Portal < 0 || p.Portal >= len(l.Portals) {
				return fmt.Errorf("%w: cell %d portal %d names geometry %d of %d", ErrFormat, i, j, p.Portal, len(l.Portals))
			}
			if p.Target < 0 || p.Target >= len(l.Cells) {
				return fmt.Errorf("%w: cell %d portal %d names cell %d of %d", ErrFormat, i, j, p.Target, len(l.Cells))
			}
		}
	}
	return nil
}

// readPortals reads the portal geometry out of a PRTS form. Up to version
// 0003 each portal is a PRTL chunk holding the polygon's vertices; from 0004
// it is an indexed triangle list, whose vertices are the same polygon and
// whose indices only fan it into triangles.
func readPortals(prts *iff.Node) ([]Portal, error) {
	portals := make([]Portal, 0, len(prts.Children))
	for i, c := range prts.Children {
		var (
			p   Portal
			err error
		)
		switch {
		case !c.IsForm() && c.Tag == "PRTL":
			p, err = readPortalChunk(c.Data)
		case c.IsForm() && c.Type == "IDTL":
			p, err = readIndexedTriangleList(c)
		default:
			err = errors.New("expected a PRTL chunk or an IDTL form")
		}
		if err != nil {
			return nil, fmt.Errorf("%w: portal %d: %v", ErrFormat, i, err)
		}
		portals = append(portals, p)
	}
	return portals, nil
}

// readPortalChunk reads a vertex count and that many vertices.
func readPortalChunk(b []byte) (Portal, error) {
	count, b, err := readInt32(b)
	if err != nil {
		return Portal{}, err
	}
	if count < 0 || int64(count)*vectorSize > int64(len(b)) {
		return Portal{}, fmt.Errorf("claims %d vertices, too many for its size", count)
	}
	vertices := make([]Vector, count)
	for i := range vertices {
		if vertices[i], b, err = readVector(b); err != nil {
			return Portal{}, err
		}
	}
	return Portal{Vertices: vertices}, nil
}

// readIndexedTriangleList reads the vertices of an IDTL form. The chunk has no
// count of its own; its length is the count.
func readIndexedTriangleList(n *iff.Node) (Portal, error) {
	if len(n.Children) != 1 || !n.Children[0].IsForm() {
		return Portal{}, errors.New("expected a single nested form")
	}
	vert, err := chunkChild(n.Children[0], "VERT")
	if err != nil {
		return Portal{}, err
	}
	if len(vert.Data)%vectorSize != 0 {
		return Portal{}, fmt.Errorf("VERT chunk of %d bytes is not whole vertices", len(vert.Data))
	}

	b := vert.Data
	vertices := make([]Vector, len(b)/vectorSize)
	for i := range vertices {
		if vertices[i], b, err = readVector(b); err != nil {
			return Portal{}, err
		}
	}
	return Portal{Vertices: vertices}, nil
}

// readCells reads the cells out of a CELS form.
func readCells(cels *iff.Node) ([]Cell, error) {
	cells := make([]Cell, 0, len(cels.Children))
	for i, c := range cels.Children {
		cell, err := readCell(c)
		if err != nil {
			return nil, fmt.Errorf("%w: cell %d: %v", ErrFormat, i, err)
		}
		cells = append(cells, cell)
	}
	return cells, nil
}

// readCell reads one CELL form. Its parts are found by tag rather than by
// position, which steps over the collision extent a version 0005 cell carries
// between its data and its portals: that form comes in half a dozen shapes and
// none of them says anything about the layout.
func readCell(n *iff.Node) (Cell, error) {
	if !n.IsForm() || n.Type != "CELL" {
		return Cell{}, errors.New("expected a CELL form")
	}
	if len(n.Children) != 1 || !n.Children[0].IsForm() {
		return Cell{}, errors.New("expected a single nested form")
	}

	version := n.Children[0]
	switch version.Type {
	case "0001", "0002", "0003", "0004", "0005":
	default:
		return Cell{}, fmt.Errorf("unsupported cell version %q", version.Type)
	}

	data, err := chunkChild(version, "DATA")
	if err != nil {
		return Cell{}, err
	}
	cell, portalCount, err := readCellData(data.Data, version.Type)
	if err != nil {
		return Cell{}, err
	}

	for _, c := range version.Children {
		if !c.IsForm() || c.Type != "PRTL" {
			continue
		}
		portal, err := readCellPortal(c)
		if err != nil {
			return Cell{}, fmt.Errorf("portal %d: %v", len(cell.Portals), err)
		}
		cell.Portals = append(cell.Portals, portal)
	}
	if int(portalCount) != len(cell.Portals) {
		return Cell{}, fmt.Errorf("DATA claims %d portals, the cell holds %d", portalCount, len(cell.Portals))
	}

	// Versions before 0003 have no lights at all.
	if lght, err := chunkChild(version, "LGHT"); err == nil {
		count, _, err := readInt32(lght.Data)
		if err != nil {
			return Cell{}, fmt.Errorf("LGHT count: %v", err)
		}
		cell.Lights = int(count)
	}
	return cell, nil
}

// readCellData reads a cell's DATA chunk, returning the cell along with how
// many portals it claims. A cell only carries its own name from version 0004,
// and a floor from 0002.
func readCellData(b []byte, version string) (Cell, int32, error) {
	var (
		cell Cell
		err  error
	)
	count, b, err := readInt32(b)
	if err != nil {
		return cell, 0, fmt.Errorf("DATA portal count: %v", err)
	}
	if cell.CanSeeParent, b, err = readBool(b); err != nil {
		return cell, 0, fmt.Errorf("DATA parent visibility: %v", err)
	}
	if version >= "0004" {
		if cell.Name, b, err = readCString(b); err != nil {
			return cell, 0, fmt.Errorf("DATA name: %v", err)
		}
	}
	if cell.Appearance, b, err = readCString(b); err != nil {
		return cell, 0, fmt.Errorf("DATA appearance: %v", err)
	}
	if version >= "0002" {
		var hasFloor bool
		if hasFloor, b, err = readBool(b); err != nil {
			return cell, 0, fmt.Errorf("DATA floor flag: %v", err)
		}
		if hasFloor {
			if cell.Floor, _, err = readCString(b); err != nil {
				return cell, 0, fmt.Errorf("DATA floor: %v", err)
			}
		}
	}
	return cell, count, nil
}

// readCellPortal reads one PRTL form of a cell. Each version adds a field to
// the front or the back of the one before it.
func readCellPortal(n *iff.Node) (CellPortal, error) {
	if len(n.Children) != 1 || n.Children[0].IsForm() {
		return CellPortal{}, errors.New("expected a single version chunk")
	}

	var (
		portal  CellPortal
		version = n.Children[0].Tag
		b       = n.Children[0].Data
		err     error
	)
	switch version {
	case "0001", "0002", "0003", "0004", "0005":
	default:
		return portal, fmt.Errorf("unsupported portal version %q", version)
	}

	if version >= "0005" {
		if portal.Disabled, b, err = readBool(b); err != nil {
			return portal, fmt.Errorf("disabled flag: %v", err)
		}
	}
	// Version 0001 has no flag because nothing was impassable yet.
	portal.Passable = true
	if version >= "0002" {
		if portal.Passable, b, err = readBool(b); err != nil {
			return portal, fmt.Errorf("passable flag: %v", err)
		}
	}

	var geometry int32
	if geometry, b, err = readInt32(b); err != nil {
		return portal, fmt.Errorf("geometry index: %v", err)
	}
	portal.Portal = int(geometry)
	if portal.Clockwise, b, err = readBool(b); err != nil {
		return portal, fmt.Errorf("winding flag: %v", err)
	}

	var target int32
	if target, b, err = readInt32(b); err != nil {
		return portal, fmt.Errorf("target cell: %v", err)
	}
	portal.Target = int(target)

	if version >= "0003" {
		// The door's hardpoint transform follows the name from version 0004.
		// It places the door within the opening, which the portal's own
		// geometry already locates, so it is left undecoded.
		if portal.Door, _, err = readCString(b); err != nil {
			return portal, fmt.Errorf("door: %v", err)
		}
	}
	return portal, nil
}

// chunkChild returns the leaf chunk named tag among n's children.
func chunkChild(n *iff.Node, tag string) (*iff.Node, error) {
	for _, c := range n.Children {
		if !c.IsForm() && c.Tag == tag {
			return c, nil
		}
	}
	return nil, fmt.Errorf("%w: %s form has no %s chunk", ErrFormat, n.Type, tag)
}

// formChild returns the form of the given type among n's children.
func formChild(n *iff.Node, typ string) (*iff.Node, error) {
	for _, c := range n.Children {
		if c.IsForm() && c.Type == typ {
			return c, nil
		}
	}
	return nil, fmt.Errorf("%w: %s form has no %s form", ErrFormat, n.Type, typ)
}

// vectorSize is what one vertex costs: three little-endian float32s.
const vectorSize = 12

// readVector reads one vertex from the front of b.
func readVector(b []byte) (Vector, []byte, error) {
	if len(b) < vectorSize {
		return Vector{}, nil, errors.New("truncated vertex")
	}
	v := Vector{
		X: math.Float32frombits(binary.LittleEndian.Uint32(b)),
		Y: math.Float32frombits(binary.LittleEndian.Uint32(b[4:])),
		Z: math.Float32frombits(binary.LittleEndian.Uint32(b[8:])),
	}
	return v, b[vectorSize:], nil
}

// readInt32 reads a little-endian int32 from the front of b.
func readInt32(b []byte) (int32, []byte, error) {
	if len(b) < 4 {
		return 0, nil, errors.New("truncated int")
	}
	return int32(binary.LittleEndian.Uint32(b)), b[4:], nil
}

// readBool reads a one-byte flag from the front of b.
func readBool(b []byte) (bool, []byte, error) {
	if len(b) < 1 {
		return false, nil, errors.New("truncated flag")
	}
	return b[0] != 0, b[1:], nil
}

// readCString reads a NUL-terminated string from the front of b, returning the
// bytes that follow it.
func readCString(b []byte) (string, []byte, error) {
	i := bytes.IndexByte(b, 0)
	if i < 0 {
		return "", nil, errors.New("unterminated string")
	}
	return string(b[:i]), b[i+1:], nil
}
