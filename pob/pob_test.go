package pob_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/madsboddum/swg-cli/iff"
	"github.com/madsboddum/swg-cli/pob"
)

// fixture parses one of the files testdata/gen writes.
func fixture(t *testing.T, name string) *iff.Node {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	root, err := iff.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// find returns the first leaf chunk in the tree carrying the given tag, so a
// test can corrupt one field without walking the tree by index.
func find(t *testing.T, n *iff.Node, tag string) *iff.Node {
	t.Helper()
	if !n.IsForm() {
		if n.Tag == tag {
			return n
		}
		return nil
	}
	for _, c := range n.Children {
		if found := find(t, c, tag); found != nil {
			return found
		}
	}
	return nil
}

func TestDecodeReadsCellsAndPortals(t *testing.T) {
	layout, err := pob.Decode(fixture(t, "basic.pob"))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if layout.CRC != 0xdeadbeef {
		t.Errorf("CRC = %#x, want 0xdeadbeef", layout.CRC)
	}
	if len(layout.Portals) != 1 {
		t.Fatalf("got %d portals, want 1", len(layout.Portals))
	}
	if got, want := layout.Portals[0].Center(), (pob.Vector{X: 1, Y: 1.5, Z: 0}); got != want {
		t.Errorf("Center() = %+v, want %+v", got, want)
	}

	if len(layout.Cells) != 2 {
		t.Fatalf("got %d cells, want 2", len(layout.Cells))
	}
	shell := layout.Cells[0]
	if shell.Name != "shell" || shell.Appearance != "appearance/mesh/hut_r0.msh" {
		t.Errorf("Cells[0] = %+v", shell)
	}
	if shell.Floor != "" || shell.CanSeeParent || shell.Lights != 0 {
		t.Errorf("Cells[0] = %+v, want no floor, no parent visibility and no lights", shell)
	}
	want := pob.CellPortal{Portal: 0, Target: 1, Passable: true, Clockwise: true}
	if len(shell.Portals) != 1 || shell.Portals[0] != want {
		t.Errorf("Cells[0].Portals = %+v, want [%+v]", shell.Portals, want)
	}

	room := layout.Cells[1]
	if room.Floor != "appearance/collision/hut_r1_floor0.flr" || !room.CanSeeParent || room.Lights != 2 {
		t.Errorf("Cells[1] = %+v", room)
	}
	want = pob.CellPortal{Portal: 0, Target: 0, Door: "door_hut"}
	if len(room.Portals) != 1 || room.Portals[0] != want {
		t.Errorf("Cells[1].Portals = %+v, want [%+v]", room.Portals, want)
	}
}

func TestDecodeReadsIndexedPortalGeometry(t *testing.T) {
	layout, err := pob.Decode(fixture(t, "indexed.pob"))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if len(layout.Portals) != 1 {
		t.Fatalf("got %d portals, want 1", len(layout.Portals))
	}
	if got, want := len(layout.Portals[0].Vertices), 3; got != want {
		t.Fatalf("got %d vertices, want %d", got, want)
	}
	if got, want := layout.Portals[0].Center(), (pob.Vector{X: 1, Y: 1, Z: 0}); got != want {
		t.Errorf("Center() = %+v, want %+v", got, want)
	}

	if len(layout.Cells) != 2 || !layout.Cells[1].Portals[0].Disabled {
		t.Errorf("Cells[1].Portals = %+v, want the portal disabled", layout.Cells[1].Portals)
	}
}

func TestCenterOfAPortalWithoutVertices(t *testing.T) {
	if got := (pob.Portal{}).Center(); got != (pob.Vector{}) {
		t.Errorf("Center() = %+v, want the zero vector", got)
	}
}

func TestDecodeRejectsANonPortalObjectForm(t *testing.T) {
	root := &iff.Node{Tag: iff.FormTag, Type: "TEST"}
	if _, err := pob.Decode(root); !errors.Is(err, pob.ErrFormat) {
		t.Errorf("Decode of a non-PRTO form = %v, want ErrFormat", err)
	}
}

func TestDecodeRejectsAnUnsupportedVersion(t *testing.T) {
	root := &iff.Node{
		Tag:      iff.FormTag,
		Type:     "PRTO",
		Children: []*iff.Node{{Tag: iff.FormTag, Type: "0009"}},
	}
	if _, err := pob.Decode(root); !errors.Is(err, pob.ErrFormat) {
		t.Errorf("Decode of a version 0009 form = %v, want ErrFormat", err)
	}
}

func TestDecodeRejectsAPortalNamingGeometryThatIsNotThere(t *testing.T) {
	root := fixture(t, "basic.pob")
	// The geometry index leads a version 0004 cell portal after its passable
	// flag, so byte 1 is its low byte.
	find(t, root, "0004").Data[1] = 9

	if _, err := pob.Decode(root); !errors.Is(err, pob.ErrFormat) {
		t.Errorf("Decode with an out of range geometry index = %v, want ErrFormat", err)
	}
}

func TestDecodeRejectsACountThatDisagreesWithTheTree(t *testing.T) {
	root := fixture(t, "basic.pob")
	// The first DATA chunk is the portal object's own, portal count first.
	find(t, root, "DATA").Data[0] = 7

	if _, err := pob.Decode(root); !errors.Is(err, pob.ErrFormat) {
		t.Errorf("Decode with a DATA portal count of 7 = %v, want ErrFormat", err)
	}
}
