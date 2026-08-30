package dtable_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/madsboddum/swg-cli/dtable"
	"github.com/madsboddum/swg-cli/iff"
)

// fixture mirrors the table built by testdata/gen: a version 0001 DTII with
// an int, a float and a string column, and two rows of data.
func fixture(t *testing.T) *iff.Node {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "basic.iff"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := iff.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestDecodeReadsColumnsAndRows(t *testing.T) {
	table, err := dtable.Decode(fixture(t))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	wantColumns := []dtable.Column{
		{Name: "level", Type: dtable.Int},
		{Name: "weight", Type: dtable.Float},
		{Name: "name", Type: dtable.String},
	}
	if len(table.Columns) != len(wantColumns) {
		t.Fatalf("Columns = %+v, want %+v", table.Columns, wantColumns)
	}
	for i, want := range wantColumns {
		if table.Columns[i] != want {
			t.Errorf("Columns[%d] = %+v, want %+v", i, table.Columns[i], want)
		}
	}

	if len(table.Rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(table.Rows))
	}
	row := table.Rows[0]
	if row[0] != int32(1) || row[1] != float32(2.5) || row[2] != "womp rat" {
		t.Errorf("Rows[0] = %+v, want [1 2.5 womp rat]", row)
	}
	row = table.Rows[1]
	if row[0] != int32(2) || row[1] != float32(-1) || row[2] != "krayt dragon" {
		t.Errorf("Rows[1] = %+v, want [2 -1 krayt dragon]", row)
	}
}

func TestDecodeRejectsANonDatatableForm(t *testing.T) {
	root := &iff.Node{Tag: iff.FormTag, Type: "TEST"}
	if _, err := dtable.Decode(root); !errors.Is(err, dtable.ErrFormat) {
		t.Errorf("Decode of a non-DTII form = %v, want ErrFormat", err)
	}
}

func TestDecodeRejectsAMissingChunk(t *testing.T) {
	root := &iff.Node{
		Tag:  iff.FormTag,
		Type: "DTII",
		Children: []*iff.Node{
			{Tag: iff.FormTag, Type: "0001"},
		},
	}
	if _, err := dtable.Decode(root); !errors.Is(err, dtable.ErrFormat) {
		t.Errorf("Decode with no COLS chunk = %v, want ErrFormat", err)
	}
}
