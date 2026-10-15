// Package objtpl decodes shared object templates, the files under object/ that
// define what the client knows about a game object: its name, appearance and
// the parameters particular to its kind, such as a draft schematic's slots and
// attribute ranges or a weapon's firing effect. It consumes a tree already
// parsed by iff; it does no byte-level container work of its own.
//
// A template is a chain of layers, one per class in the client's template
// hierarchy, most specific first: a draft schematic is an SDSC layer around an
// SITN layer around the SHOT layer every template ends in. Each layer is a
// list of named parameters.
package objtpl

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/madsboddum/swg-cli/iff"
)

// ErrFormat reports that the node tree is not an object template this package
// can read.
var ErrFormat = errors.New("objtpl: not a supported object template")

// Template is a decoded object template.
type Template struct {
	// Layers runs from the form at the root of the file down to SHOT.
	Layers []Layer
}

// Layer is one class's share of a template.
type Layer struct {
	// Type is the layer's form type, such as SDSC or SHOT.
	Type    string
	Version string
	// Parent is the template this layer inherits unset parameters from, empty
	// if it names none.
	Parent string
	Params []Param
}

// Kind names the kind of object a layer's form type describes, such as
// "draft schematic" for SDSC, or returns "" for a form type it does not know.
func Kind(formType string) string {
	return kinds[formType]
}

// kinds is every template layer the CU client's object/ tree holds, each named
// after the directory its templates live under.
var kinds = map[string]string{
	"CCLT": "cell",
	"RCCT": "resource container",
	"SBMK": "battlefield marker",
	"SBOT": "building",
	"SCNC": "construction contract",
	"SCOT": "creature",
	"SCOU": "counting",
	"SDSC": "draft schematic",
	"SFOT": "factory",
	"SGLD": "guild",
	"SGRP": "group",
	"SHOT": "object",
	"SIOT": "installation",
	"SITN": "intangible",
	"SJED": "jedi manager",
	"SMLE": "mission list entry",
	"SMSC": "manufacture schematic",
	"SMSD": "mission data",
	"SMSO": "mission",
	"SPLY": "player",
	"SSHP": "ship",
	"STAT": "static",
	"STOK": "token",
	"STOT": "tangible",
	"SUNI": "universe",
	"SWAY": "waypoint",
	"SWOT": "weapon",
}

// Param is one named parameter. Value is nil for a parameter the template
// leaves unset, which the client resolves through Parent.
type Param struct {
	Name  string
	Value Value
}

// Value is a parameter's value: a Bool, Int, IntRange, Float, FloatRange,
// String, StringID, Weighted, List or Struct.
type Value interface {
	String() string
}

// Bool is a flag parameter.
type Bool bool

func (b Bool) String() string { return strconv.FormatBool(bool(b)) }

// Int is an integer parameter. Enumerations, such as an object's game object
// type, are stored as one too.
type Int int32

func (i Int) String() string { return strconv.Itoa(int(i)) }

// IntRange is an integer the server picks between Min and Max, both included.
type IntRange struct{ Min, Max int32 }

func (r IntRange) String() string { return fmt.Sprintf("%d..%d", r.Min, r.Max) }

// Float is a floating point parameter.
type Float float32

func (f Float) String() string { return strconv.FormatFloat(float64(f), 'g', -1, 32) }

// FloatRange is a float the server picks between Min and Max.
type FloatRange struct{ Min, Max float32 }

func (r FloatRange) String() string { return Float(r.Min).String() + ".." + Float(r.Max).String() }

// String is a text parameter, most often the path of another file.
type String string

func (s String) String() string { return strconv.Quote(string(s)) }

// StringID names a string in a string table, the way the game writes one.
type StringID struct{ Table, Key string }

func (s StringID) String() string { return "@" + s.Table + ":" + s.Key }

// Choice is one entry of a Weighted value.
type Choice struct {
	Weight int32
	Value  Value
}

// Weighted is a value the server picks from Choices, each in proportion to
// its weight.
type Weighted []Choice

func (w Weighted) String() string {
	var b bytes.Buffer
	for i, c := range w {
		if i > 0 {
			b.WriteString(" | ")
		}
		fmt.Fprintf(&b, "%s (weight %d)", valueString(c.Value), c.Weight)
	}
	return b.String()
}

// List is a sequence parameter. A template whose list has Append set adds
// Items to its parent's list instead of replacing it.
type List struct {
	Append bool
	Items  []Value
}

func (l List) String() string {
	var b bytes.Buffer
	b.WriteByte('[')
	for i, v := range l.Items {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(valueString(v))
	}
	b.WriteByte(']')
	return b.String()
}

// Struct is a parameter made of parameters of its own, such as one of a draft
// schematic's slots.
type Struct struct {
	Type   string
	Params []Param
}

func (s Struct) String() string {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, p := range s.Params {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(p.Name + "=" + valueString(p.Value))
	}
	b.WriteByte('}')
	return b.String()
}

// valueString renders v, writing an unset value as a dash.
func valueString(v Value) string {
	if v == nil {
		return "-"
	}
	return v.String()
}

// derivedTag is the form naming the template a layer inherits from.
const derivedTag = "DERV"

// countTag is the chunk opening a parameter list with how many follow.
const countTag = "PCNT"

// paramTag is the chunk each parameter is stored in.
const paramTag = "XXXX"

// Decode reads an object template out of root, the node tree of its file.
func Decode(root *iff.Node) (*Template, error) {
	t := &Template{}
	for n := root; n != nil; {
		layer, base, err := readLayer(n)
		if err != nil {
			return nil, err
		}
		t.Layers = append(t.Layers, layer)
		n = base
	}
	return t, nil
}

// readLayer reads one layer form: an optional DERV naming the parent template,
// the version form holding the parameters, then the next layer's form if there
// is one.
func readLayer(n *iff.Node) (Layer, *iff.Node, error) {
	if !n.IsForm() {
		return Layer{}, nil, fmt.Errorf("%w: %s is not a form", ErrFormat, n.Tag)
	}
	layer := Layer{Type: n.Type}
	children := n.Children

	if len(children) > 0 && children[0].IsForm() && children[0].Type == derivedTag {
		parent, err := readParent(children[0])
		if err != nil {
			return Layer{}, nil, err
		}
		layer.Parent = parent
		children = children[1:]
	}

	if len(children) == 0 || !children[0].IsForm() || !isVersion(children[0].Type) {
		return Layer{}, nil, fmt.Errorf("%w: %s form has no version form", ErrFormat, n.Type)
	}
	version := children[0]
	layer.Version = version.Type
	params, err := readParams(n.Type, version.Children)
	if err != nil {
		return Layer{}, nil, fmt.Errorf("%w: %s %s: %v", ErrFormat, n.Type, version.Type, err)
	}
	layer.Params = params
	children = children[1:]

	switch {
	case len(children) == 0:
		return layer, nil, nil
	case len(children) == 1 && children[0].IsForm():
		return layer, children[0], nil
	default:
		return Layer{}, nil, fmt.Errorf("%w: %s form has more than one base form", ErrFormat, n.Type)
	}
}

// readParent reads the template path a DERV form holds.
func readParent(n *iff.Node) (string, error) {
	if len(n.Children) != 1 || n.Children[0].IsForm() {
		return "", fmt.Errorf("%w: %s form does not hold a single chunk", ErrFormat, derivedTag)
	}
	r := &reader{b: n.Children[0].Data}
	parent := r.cstring()
	if r.err == nil && len(r.b) != 0 {
		r.fail("%d bytes after the parent path", len(r.b))
	}
	if r.err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrFormat, derivedTag, r.err)
	}
	return parent, nil
}

// isVersion reports whether a form type is a version number, four digits.
func isVersion(typ string) bool {
	for _, c := range []byte(typ) {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(typ) == 4
}

// readParams reads the PCNT chunk at the front of children and the parameters
// it counts. owner is the form type the parameters belong to, which decides
// how each one's value is laid out.
func readParams(owner string, children []*iff.Node) ([]Param, error) {
	if len(children) == 0 || children[0].IsForm() || children[0].Tag != countTag {
		return nil, fmt.Errorf("no %s chunk", countTag)
	}
	r := &reader{b: children[0].Data}
	count := r.int32()
	if r.err != nil {
		return nil, fmt.Errorf("%s: %v", countTag, r.err)
	}

	r = &reader{nodes: children[1:]}
	var params []Param
	for i := int32(0); i < count; i++ {
		r.nextChunk()
		name := r.cstring()
		if r.err != nil {
			return nil, fmt.Errorf("parameter %d: %v", i, r.err)
		}
		decode, ok := schema[owner][name]
		if !ok {
			return nil, fmt.Errorf("unknown parameter %q", name)
		}
		v := decode(r)
		if r.err == nil && len(r.b) != 0 {
			r.fail("%d bytes left over", len(r.b))
		}
		if r.err != nil {
			return nil, fmt.Errorf("%s: %v", name, r.err)
		}
		params = append(params, Param{Name: name, Value: v})
	}
	if len(r.nodes) != 0 {
		return nil, fmt.Errorf("%s claims %d parameters, more follow", countTag, count)
	}
	return params, nil
}

// reader walks a parameter's bytes. A parameter holding structs is not one
// chunk but several, broken by the form each struct is stored in, so the
// reader carries the sibling nodes still to come as well as the bytes of the
// chunk it is in. The first failure sticks and every later read returns zero,
// which lets a decoder read a whole value and check once at the end.
type reader struct {
	nodes []*iff.Node
	b     []byte
	err   error
}

func (r *reader) fail(format string, args ...any) {
	if r.err == nil {
		r.err = fmt.Errorf(format, args...)
	}
}

// take returns the next n bytes of the current chunk.
func (r *reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if len(r.b) < n {
		r.fail("truncated value")
		return nil
	}
	v := r.b[:n]
	r.b = r.b[n:]
	return v
}

func (r *reader) byte() byte {
	if b := r.take(1); b != nil {
		return b[0]
	}
	return 0
}

func (r *reader) int32() int32 {
	if b := r.take(4); b != nil {
		return int32(binary.LittleEndian.Uint32(b))
	}
	return 0
}

func (r *reader) float32() float32 {
	if b := r.take(4); b != nil {
		return math.Float32frombits(binary.LittleEndian.Uint32(b))
	}
	return 0
}

func (r *reader) cstring() string {
	if r.err != nil {
		return ""
	}
	i := bytes.IndexByte(r.b, 0)
	if i < 0 {
		r.fail("unterminated string")
		return ""
	}
	s := string(r.b[:i])
	r.b = r.b[i+1:]
	return s
}

// nextChunk moves on to the next parameter chunk.
func (r *reader) nextChunk() {
	if r.err != nil {
		return
	}
	if len(r.nodes) == 0 || r.nodes[0].IsForm() || r.nodes[0].Tag != paramTag {
		r.fail("expected a %s chunk", paramTag)
		return
	}
	r.b = r.nodes[0].Data
	r.nodes = r.nodes[1:]
}

// form takes the form that follows the current chunk, which must be used up,
// and moves on to the chunk after it, where the parameter carries on.
func (r *reader) form() *iff.Node {
	if r.err != nil {
		return nil
	}
	if len(r.b) != 0 {
		r.fail("%d bytes before a nested form", len(r.b))
		return nil
	}
	if len(r.nodes) == 0 || !r.nodes[0].IsForm() {
		r.fail("expected a nested form")
		return nil
	}
	n := r.nodes[0]
	r.nodes = r.nodes[1:]
	r.nextChunk()
	return n
}

// Data types, the byte every value opens with.
const (
	dataNone     = 0
	dataSingle   = 1
	dataWeighted = 2
	dataRange    = 3
)

// decoder reads one value. It returns nil for an unset one.
type decoder func(r *reader) Value

// numeric reads the data type and delta of an integer or float value. The
// delta lets a template adjust its parent's value rather than replace it;
// every template the client ships replaces, so the others are refused rather
// than guessed at.
func numeric(r *reader) byte {
	typ, delta := r.byte(), r.byte()
	if r.err == nil && delta != ' ' {
		r.fail("unsupported delta %q", delta)
	}
	return typ
}

func integer(r *reader) Value {
	switch typ := numeric(r); typ {
	case dataNone:
		return nil
	case dataSingle:
		return Int(r.int32())
	case dataRange:
		return IntRange{Min: r.int32(), Max: r.int32()}
	case dataWeighted:
		return weighted(r, integer)
	default:
		r.fail("unsupported integer data type %d", typ)
		return nil
	}
}

func float(r *reader) Value {
	switch typ := numeric(r); typ {
	case dataNone:
		return nil
	case dataSingle:
		return Float(r.float32())
	case dataRange:
		return FloatRange{Min: r.float32(), Max: r.float32()}
	case dataWeighted:
		return weighted(r, float)
	default:
		r.fail("unsupported float data type %d", typ)
		return nil
	}
}

// weighted reads a count and that many weight and value pairs, each value
// carrying its own data type.
func weighted(r *reader, elem decoder) Value {
	var w Weighted
	for n := r.int32(); n > 0 && r.err == nil; n-- {
		weight := r.int32()
		w = append(w, Choice{Weight: weight, Value: elem(r)})
	}
	return w
}

// single reads the data type of a value that has no range or weighted form,
// reporting whether a value follows.
func single(r *reader) bool {
	switch typ := r.byte(); typ {
	case dataNone:
		return false
	case dataSingle:
		return true
	default:
		r.fail("unsupported data type %d", typ)
		return false
	}
}

func boolean(r *reader) Value {
	if !single(r) {
		return nil
	}
	return Bool(r.byte() != 0)
}

func text(r *reader) Value {
	if !single(r) {
		return nil
	}
	return String(r.cstring())
}

// stringID reads a table and a key, each stored as a string value of its own.
func stringID(r *reader) Value {
	if !single(r) {
		return nil
	}
	part := func() string {
		if s, ok := text(r).(String); ok {
			return string(s)
		}
		return ""
	}
	table := part()
	return StringID{Table: table, Key: part()}
}

// structure reads a struct: its form type, stored as a little-endian 4CC, then
// the form holding its parameters.
func structure(r *reader) Value {
	if !single(r) {
		return nil
	}
	tag := r.take(4)
	if tag == nil {
		return nil
	}
	typ := string([]byte{tag[3], tag[2], tag[1], tag[0]})
	n := r.form()
	if n == nil {
		return nil
	}
	if n.Type != typ {
		r.fail("struct %s stored in a %s form", typ, n.Type)
		return nil
	}
	params, err := readParams(typ, n.Children)
	if err != nil {
		r.fail("%s: %v", typ, err)
		return nil
	}
	return Struct{Type: typ, Params: params}
}

// listOf decodes a list: the append flag, a count, then the items.
func listOf(elem decoder) decoder {
	return func(r *reader) Value {
		l := List{Append: r.byte() != 0}
		for n := r.int32(); n > 0 && r.err == nil; n-- {
			l.Items = append(l.Items, elem(r))
		}
		return l
	}
}

// arrayOf decodes a fixed-size array, which has a count but no append flag.
func arrayOf(elem decoder) decoder {
	return func(r *reader) Value {
		var l List
		for n := r.int32(); n > 0 && r.err == nil; n-- {
			l.Items = append(l.Items, elem(r))
		}
		return l
	}
}

// schema says how each form's parameters are laid out. The bytes alone do not
// tell an integer from a float, nor an empty string from a false flag, so a
// parameter missing from here fails the decode rather than being guessed at.
// It covers every parameter the CU client's templates hold.
var schema map[string]map[string]decoder

// init fills schema rather than its declaration doing so, since structure
// reaches back into schema and Go rejects the cycle in an initializer.
//
//nolint:funlen // one flat table of parameters; splitting it by form would only scatter it.
func init() {
	schema = map[string]map[string]decoder{
		"SHOT": {
			"objectName":                     stringID,
			"detailedDescription":            stringID,
			"lookAtText":                     stringID,
			"snapToTerrain":                  boolean,
			"containerType":                  integer,
			"containerVolumeLimit":           integer,
			"tintPalette":                    text,
			"slotDescriptorFilename":         text,
			"arrangementDescriptorFilename":  text,
			"appearanceFilename":             text,
			"portalLayoutFilename":           text,
			"clientDataFile":                 text,
			"collisionMaterialFlags":         integer,
			"collisionMaterialPassFlags":     integer,
			"collisionMaterialBlockFlags":    integer,
			"collisionActionFlags":           integer,
			"collisionActionPassFlags":       integer,
			"collisionActionBlockFlags":      integer,
			"scale":                          float,
			"gameObjectType":                 integer,
			"sendToClient":                   boolean,
			"scaleThresholdBeforeExtentTest": float,
			"clearFloraRadius":               float,
			"surfaceType":                    integer,
			"noBuildRadius":                  float,
			"onlyVisibleInTools":             boolean,
			"locationReservationRadius":      float,
			"forceNoCollision":               boolean,
		},
		"STOT": {
			"paletteColorCustomizationVariables": listOf(structure),
			"rangedIntCustomizationVariables":    listOf(structure),
			"constStringCustomizationVariables":  listOf(structure),
			"socketDestinations":                 listOf(integer),
			"structureFootprintFileName":         text,
			"useStructureFootprintOutline":       boolean,
			"targetable":                         boolean,
			"certificationsRequired":             listOf(text),
			"customizationVariableMapping":       listOf(structure),
			"clientVisabilityFlag":               integer,
			"clearFloraRadius":                   float,
			"surfaceType":                        integer,
		},
		"PCCV": {
			"variableName":        text,
			"palettePathName":     text,
			"defaultPaletteIndex": integer,
		},
		"RICV": {
			"variableName":      text,
			"minValueInclusive": integer,
			"defaultValue":      integer,
			"maxValueExclusive": integer,
		},
		"CVMM": {
			"sourceVariable":    text,
			"dependentVariable": text,
		},
		"SCOT": {
			"gender":                integer,
			"niche":                 integer,
			"species":               integer,
			"race":                  integer,
			"acceleration":          arrayOf(float),
			"speed":                 arrayOf(float),
			"turnRate":              arrayOf(float),
			"animationMapFilename":  text,
			"slopeModAngle":         float,
			"slopeModPercent":       float,
			"waterModPercent":       float,
			"stepHeight":            float,
			"collisionHeight":       float,
			"collisionRadius":       float,
			"movementDatatable":     text,
			"postureAlignToTerrain": arrayOf(boolean),
			"swimHeight":            float,
			"warpTolerance":         float,
			"collisionOffsetX":      float,
			"collisionOffsetZ":      float,
			"collisionLength":       float,
			"cameraHeight":          float,
		},
		"SBOT": {
			"terrainModificationFileName": text,
			"interiorLayoutFileName":      text,
		},
		"SBMK": {
			"numberOfPoles": integer,
			"radius":        float,
		},
		"SDSC": {
			"slots":                 listOf(structure),
			"attributes":            listOf(structure),
			"craftedSharedTemplate": text,
		},
		"SISS": {
			"name":      stringID,
			"hardpoint": text,
		},
		"DSSA": {
			"name":       stringID,
			"experiment": stringID,
			"value":      integer,
		},
		"SSHP": {
			"cockpitFilename":        text,
			"hasWings":               boolean,
			"playerControlled":       boolean,
			"interiorLayoutFileName": text,
		},
		"SWOT": {
			"weaponEffect":      text,
			"weaponEffectIndex": integer,
			"attackType":        integer,
		},
	}
}
