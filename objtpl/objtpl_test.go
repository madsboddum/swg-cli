package objtpl_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"

	"github.com/madsboddum/swg-cli/iff"
	"github.com/madsboddum/swg-cli/objtpl"
)

// chunk lays out a leaf chunk: a 4CC tag, its big-endian length, then data.
func chunk(tag string, data ...[]byte) []byte {
	body := bytes.Join(data, nil)
	out := []byte(tag)
	out = binary.BigEndian.AppendUint32(out, uint32(len(body)))
	return append(out, body...)
}

// form lays out a FORM chunk of the given type holding children.
func form(typ string, children ...[]byte) []byte {
	return chunk("FORM", append([][]byte{[]byte(typ)}, children...)...)
}

// le lays out values little-endian, the way parameter values are stored.
func le(vs ...any) []byte {
	var buf bytes.Buffer
	for _, v := range vs {
		if err := binary.Write(&buf, binary.LittleEndian, v); err != nil {
			panic(err)
		}
	}
	return buf.Bytes()
}

func cstr(s string) []byte { return append([]byte(s), 0) }

// param lays out a parameter chunk: the name, then the value's bytes.
func param(name string, value ...[]byte) []byte {
	return chunk("XXXX", append([][]byte{cstr(name)}, value...)...)
}

func count(n int32) []byte { return chunk("PCNT", le(n)) }

func derv(parent string) []byte { return form("DERV", chunk("XXXX", cstr(parent))) }

func decode(t *testing.T, b []byte) (*objtpl.Template, error) {
	t.Helper()
	root, err := iff.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return objtpl.Decode(root)
}

// schematic is a draft schematic: an SDSC layer with one slot and one
// attribute range, around an SITN layer setting nothing, around a SHOT layer
// with a name, a scale range and a parameter left unset.
func schematic() []byte {
	return form("SDSC",
		derv("object/draft_schematic/base/shared_base_draft_schematic.iff"),
		form("0003",
			count(3),
			// A list of structs is split across chunks: the list's own bytes and
			// each struct's tag precede the struct's form, and the chunk after
			// the last form is left empty.
			param("slots", le(byte(0), int32(1), byte(1)), []byte("SSIS")),
			form("SISS",
				count(2),
				param("name", []byte{1, 1}, cstr("craft_weapon_ingredients_n"), []byte{1}, cstr("frame_assembly")),
				param("hardpoint", []byte{1}, cstr("")),
			),
			chunk("XXXX"),
			param("attributes", le(byte(0), int32(1), byte(1)), []byte("ASSD")),
			form("DSSA",
				count(3),
				param("name", []byte{1, 1}, cstr("crafting"), []byte{1}, cstr("minDamage")),
				param("experiment", []byte{1, 1}, cstr(""), []byte{1}, cstr("")),
				param("value", le(byte(3), byte(' '), int32(17), int32(34))),
			),
			chunk("XXXX"),
			param("craftedSharedTemplate", []byte{1}, cstr("object/weapon/shared_pistol.iff")),
		),
		form("SITN",
			form("0000", count(0)),
			form("SHOT",
				form("0010",
					count(3),
					param("objectName", []byte{1, 1}, cstr("weapon_name"), []byte{1}, cstr("pistol")),
					param("scale", le(byte(3), byte(' '), float32(0.5), float32(1.5))),
					param("containerType", []byte{0, ' '}),
				),
			),
		),
	)
}

func TestDecodeReadsEveryLayer(t *testing.T) {
	got, err := decode(t, schematic())
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	want := &objtpl.Template{Layers: []objtpl.Layer{
		{
			Type:    "SDSC",
			Version: "0003",
			Parent:  "object/draft_schematic/base/shared_base_draft_schematic.iff",
			Params: []objtpl.Param{
				{Name: "slots", Value: objtpl.List{Items: []objtpl.Value{
					objtpl.Struct{Type: "SISS", Params: []objtpl.Param{
						{Name: "name", Value: objtpl.StringID{Table: "craft_weapon_ingredients_n", Key: "frame_assembly"}},
						{Name: "hardpoint", Value: objtpl.String("")},
					}},
				}}},
				{Name: "attributes", Value: objtpl.List{Items: []objtpl.Value{
					objtpl.Struct{Type: "DSSA", Params: []objtpl.Param{
						{Name: "name", Value: objtpl.StringID{Table: "crafting", Key: "minDamage"}},
						{Name: "experiment", Value: objtpl.StringID{}},
						{Name: "value", Value: objtpl.IntRange{Min: 17, Max: 34}},
					}},
				}}},
				{Name: "craftedSharedTemplate", Value: objtpl.String("object/weapon/shared_pistol.iff")},
			},
		},
		{Type: "SITN", Version: "0000"},
		{
			Type:    "SHOT",
			Version: "0010",
			Params: []objtpl.Param{
				{Name: "objectName", Value: objtpl.StringID{Table: "weapon_name", Key: "pistol"}},
				{Name: "scale", Value: objtpl.FloatRange{Min: 0.5, Max: 1.5}},
				{Name: "containerType", Value: nil},
			},
		},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Decode =\n%#v\nwant\n%#v", got, want)
	}
}

func TestDecodeReadsArraysAndWeightedValues(t *testing.T) {
	b := form("SCOT",
		form("0012",
			count(3),
			param("gender", le(byte(2), byte(' '), int32(2),
				int32(50), byte(1), byte(' '), int32(0),
				int32(50), byte(1), byte(' '), int32(1))),
			param("speed", le(int32(2), byte(1), byte(' '), float32(6), byte(0), byte(' '))),
			param("postureAlignToTerrain", le(int32(2), byte(1), byte(1), byte(0))),
		),
		form("STOT", form("0007", count(0)), form("SHOT", form("0008", count(0)))),
	)
	got, err := decode(t, b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	want := []objtpl.Param{
		{Name: "gender", Value: objtpl.Weighted{{Weight: 50, Value: objtpl.Int(0)}, {Weight: 50, Value: objtpl.Int(1)}}},
		{Name: "speed", Value: objtpl.List{Items: []objtpl.Value{objtpl.Float(6), nil}}},
		{Name: "postureAlignToTerrain", Value: objtpl.List{Items: []objtpl.Value{objtpl.Bool(true), nil}}},
	}
	if !reflect.DeepEqual(got.Layers[0].Params, want) {
		t.Errorf("params =\n%#v\nwant\n%#v", got.Layers[0].Params, want)
	}
}

func TestKind(t *testing.T) {
	if got := objtpl.Kind("SDSC"); got != "draft schematic" {
		t.Errorf(`Kind("SDSC") = %q, want "draft schematic"`, got)
	}
	if got := objtpl.Kind("ZZZZ"); got != "" {
		t.Errorf(`Kind("ZZZZ") = %q, want ""`, got)
	}
}

func TestValueStrings(t *testing.T) {
	tests := []struct {
		v    objtpl.Value
		want string
	}{
		{objtpl.Bool(true), "true"},
		{objtpl.Int(-3), "-3"},
		{objtpl.IntRange{Min: 17, Max: 34}, "17..34"},
		{objtpl.Float(0.85), "0.85"},
		{objtpl.FloatRange{Min: 0.5, Max: 1.5}, "0.5..1.5"},
		{objtpl.String(`a "b"`), `"a \"b\""`},
		{objtpl.StringID{Table: "crafting", Key: "minDamage"}, "@crafting:minDamage"},
		{objtpl.Weighted{{Weight: 50, Value: objtpl.Int(0)}, {Weight: 25, Value: nil}}, "0 (weight 50) | - (weight 25)"},
		{objtpl.List{Items: []objtpl.Value{objtpl.Float(2), nil}}, "[2, -]"},
		{objtpl.Struct{Type: "SISS", Params: []objtpl.Param{{Name: "hardpoint", Value: objtpl.String("")}}}, `{hardpoint=""}`},
	}
	for _, tt := range tests {
		if got := tt.v.String(); got != tt.want {
			t.Errorf("%#v.String() = %q, want %q", tt.v, got, tt.want)
		}
	}
}

func TestDecodeRejects(t *testing.T) {
	// shot wraps parameters in a SHOT layer claiming n of them.
	shot := func(n int32, params ...[]byte) []byte {
		return form("SHOT", form("0010", append([][]byte{count(n)}, params...)...))
	}

	tests := []struct {
		name string
		b    []byte
	}{
		{"a form that is not a template", form("MESH", chunk("DATA"))},
		{"a version form with no parameter count", form("SHOT", form("0010"))},
		{"a parameter no schema knows", shot(1, param("mystery", []byte{0}))},
		{"bytes after a value", shot(1, param("snapToTerrain", []byte{1, 1, 1}))},
		{"a truncated value", shot(1, param("scale", []byte{1, ' ', 0}))},
		{"a count short of the parameters", shot(0, param("snapToTerrain", []byte{0}))},
		{"a count beyond the parameters", shot(2, param("snapToTerrain", []byte{0}))},
		{"a delta that adjusts the parent", shot(1, param("scale", le(byte(1), byte('+'), float32(1))))},
		{"a die roll", shot(1, param("containerType", le(byte(4), byte(' '), int32(1), int32(6), int32(0))))},
		{"two base forms", form("STOT", form("0007", count(0)), form("SHOT"), form("SHOT"))},
		{"a struct in a form of another type", form("SDSC",
			form("0003",
				count(1),
				param("slots", le(byte(0), int32(1), byte(1)), []byte("SSIS")),
				form("DSSA", count(0)),
				chunk("XXXX"),
			),
		)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decode(t, tt.b); !errors.Is(err, objtpl.ErrFormat) {
				t.Errorf("Decode error = %v, want ErrFormat", err)
			}
		})
	}
}
