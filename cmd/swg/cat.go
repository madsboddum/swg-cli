package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	gopath "path"
	"strings"

	"github.com/madsboddum/swg-cli/archive"
	"github.com/madsboddum/swg-cli/dtable"
	"github.com/madsboddum/swg-cli/iff"
	"github.com/madsboddum/swg-cli/objtpl"
	"github.com/madsboddum/swg-cli/pal"
	"github.com/madsboddum/swg-cli/pob"
	"github.com/madsboddum/swg-cli/stf"
)

const catUsage = `usage: swg cat [-dir directory] path...

Decode paths from the archives into readable text on standard output. An
IFF container prints as an indented node tree, a DTII datatable as a
tab-separated table, an object template as its parameters, a .stf string
table as one @file:key|value per line, a .pal palette as an index, hex and
rgb table and a .pob portal object as its cells and the portals between
them. Anything else is written out unchanged, as the bytes it holds. A path
present in several archives is read from the one that wins, loose files
first and then the highest numbered patch.

Patterns may use * and ? within a path segment and ** across segments, so
one invocation can concatenate a whole tree of files. Quote them, or the
shell will try to expand them first.

String tables are decoded, one entry per line as @file:key|value, the string
id the game writes, which makes a reverse lookup a matter of piping cat into
grep:

    swg cat 'string/en/**.stf' | grep -i "you feel a disturbance"

Files holding an IFF container, the FORM-based format underneath datatables,
object templates, appearances and more, are printed as an indented tree of
their nodes regardless of extension; the dispatch is on the FORM magic, not
the name.

A DTII datatable is instead printed one row per line, tab-separated, header
row first. A malformed or unrecognised DTII falls back to the node tree. Pipe
into "column -t -s $'\t'" for an aligned view on a terminal.

An object template, one of the files under object/, prints one layer of the
template at a time, the most specific first, each with the template it
derives from and one "name = value" line per parameter it sets. A list of
structs, such as a draft schematic's attribute ranges, prints one struct per
line beneath the list's name:

    swg cat object/draft_schematic/weapon/shared_pistol_blaster_cdef.iff

Parameters a template leaves unset are left out, since their value comes
from the parent. A template holding a parameter cat does not know falls
back to the node tree.

A portal object, the interior of a building a player can walk inside, prints
as two tables: one line per cell, then one line per portal out of a cell. A
portal's x, y and z are the centre of the opening in the building's own
coordinates, which is where the door between the two cells stands:

    swg cat appearance/thm_newbie_hall.pob

Palettes are printed one colour per line as index, #rrggbb and the three
channels, tab-separated, header row first. On a terminal each line is prefixed
with a swatch of the colour itself and the columns are aligned; redirect the
output, or pass -color never, for the tab-separated form.

-format hex skips all of the above and dumps the bytes instead, sixteen to a
line as an offset, hex and ASCII. An IFF container keeps its node tree, with
each chunk's bytes dumped in full beneath it and offsets counted from the
start of the chunk. It is the view to reach for when working out a form cat
does not decode yet.

  -dir directory
        directory holding the .tre archives; defaults to $SWG_DIR
  -color auto|always|never
        colourise palettes; auto, the default, means a terminal that has
        not set NO_COLOR
  -format pretty|hex
        pretty, the default, decodes each file by what it holds; hex dumps
        its bytes
`

func runCat(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cat", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, catUsage) }

	dir := fs.String("dir", "", "directory holding the .tre archives")
	when := fs.String("color", "auto", "colourise palettes: auto, always or never")
	format := fs.String("format", "pretty", "how to render: pretty or hex")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	color, err := colorize(*when, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "swg cat: %v\n", err)
		return 2
	}
	if *format != "pretty" && *format != "hex" {
		fmt.Fprintf(stderr, "swg cat: -format: %q is not pretty or hex\n", *format)
		return 2
	}
	operands := fs.Args()
	if len(operands) == 0 {
		fmt.Fprint(stderr, catUsage)
		return 2
	}

	root, err := archiveDir(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "swg cat: %v\n", err)
		return 2
	}

	stack, err := archive.Open(root)
	if err != nil {
		fmt.Fprintf(stderr, "swg cat: %v\n", err)
		return 1
	}
	defer func() { _ = stack.Close() }()

	return emitAll(stack, operands, stdout, stderr, color, *format)
}

// emitAll writes every path each operand resolves to, reporting failures as it
// goes and returning the exit code they add up to.
func emitAll(stack *archive.Stack, operands []string, stdout, stderr io.Writer, color bool, format string) int {
	code := 0
	for _, operand := range operands {
		found, err := match(stack.Paths(), operand)
		if err != nil {
			fmt.Fprintf(stderr, "swg cat: %v\n", err)
			code = 2
			continue
		}
		if len(found) == 0 {
			fmt.Fprintf(stderr, "swg cat: %s: no such path\n", operand)
			code = 1
			continue
		}
		for _, p := range found {
			if err := emit(stack, p, stdout, color, format); err != nil {
				fmt.Fprintf(stderr, "swg cat: %v\n", err)
				code = 1
			}
		}
	}
	return code
}

// match resolves one cat argument against the sorted path index. Unlike ls,
// a path naming a directory is not expanded to its entries; use a pattern.
func match(paths []string, operand string) ([]string, error) {
	if !archive.HasMeta(operand) {
		for _, p := range paths {
			if p == operand {
				return []string{p}, nil
			}
		}
		return nil, nil
	}

	var out []string
	for _, p := range paths {
		ok, err := archive.MatchPath(operand, p)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, p)
		}
	}
	return out, nil
}

// emit writes one file, decoding it first if it is a string table, a palette
// or an IFF container. A format of hex skips the decoding and dumps the bytes,
// within the node tree if they make an IFF container.
// Why cat sniffs rather than being told the format: docs/decisions/0001-reading-sniffs-the-format.md
func emit(stack *archive.Stack, path string, stdout io.Writer, color bool, format string) error {
	b, err := stack.ReadFile(path)
	if err != nil {
		return err
	}

	if format == "hex" {
		// A malformed container degrades to a flat dump, so the bytes that
		// would not parse are still there to look at.
		if bytes.HasPrefix(b, []byte(iff.FormTag)) {
			if root, err := iff.Parse(b); err == nil {
				return printTree(stdout, root, 0, true)
			}
		}
		return hexdump(stdout, b, "")
	}

	// A malformed palette degrades to its bytes rather than failing, the way a
	// malformed DTII degrades to a node tree.
	if pal.HasMagic(b) {
		if palette, err := pal.Decode(b); err == nil {
			return printPalette(stdout, palette, color)
		}
	}

	if bytes.HasPrefix(b, []byte(iff.FormTag)) {
		return emitContainer(stdout, path, b)
	}

	if !strings.EqualFold(gopath.Ext(path), ".stf") {
		_, err = stdout.Write(b)
		return err
	}

	table, err := stf.Decode(b)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	id := stringID(path)
	for _, e := range table.Entries() {
		if _, err := fmt.Fprintf(stdout, "%s:%s|%s\n", id, e.Key, e.Value); err != nil {
			return err
		}
	}
	return nil
}

// emitContainer writes an IFF container, decoding it whole when its root form
// is one of the formats built on top of the container. A form this knows but
// cannot decode degrades to its node tree, the way a malformed palette
// degrades to its bytes.
func emitContainer(w io.Writer, path string, b []byte) error {
	root, err := iff.Parse(b)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if root.IsForm() {
		switch root.Type {
		case dtable.FormType:
			if table, err := dtable.Decode(root); err == nil {
				return printTable(w, table)
			}
		case pob.FormType:
			if layout, err := pob.Decode(root); err == nil {
				return printLayout(w, layout)
			}
		default:
			// Object templates come in dozens of form types, so they are known
			// by their shape rather than by a list of them.
			if t, err := objtpl.Decode(root); err == nil {
				return printTemplate(w, t)
			}
		}
	}
	return printTree(w, root, 0, false)
}

// printTemplate writes an object template one layer at a time, most specific
// first: a header naming the layer, its version and the template it derives
// from, then one parameter per line. A list of structs, such as a draft
// schematic's attributes, puts each struct on a line of its own beneath the
// list's name. Parameters left unset are skipped; they say only that the value
// comes from the parent.
// Why values print as stored rather than resolved: docs/decisions/0005-rendering-stays-close-to-the-bytes.md
func printTemplate(w io.Writer, t *objtpl.Template) error {
	for i, l := range t.Layers {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		header := l.Type
		if kind := objtpl.Kind(l.Type); kind != "" {
			header += " (" + kind + ")"
		}
		header += " " + l.Version
		if l.Parent != "" {
			header += ", derived from " + l.Parent
		}
		if _, err := fmt.Fprintln(w, header); err != nil {
			return err
		}

		set := 0
		for _, p := range l.Params {
			if unset(p.Value) {
				continue
			}
			set++
			if err := printParam(w, p); err != nil {
				return err
			}
		}
		// Said outright, so an empty layer does not read as output cut short.
		if set == 0 {
			if _, err := fmt.Fprintln(w, "  (nothing set)"); err != nil {
				return err
			}
		}
	}
	return nil
}

// printParam writes one parameter of a template layer that is set.
func printParam(w io.Writer, p objtpl.Param) error {
	structs, ok := structItems(p.Value)
	if !ok {
		_, err := fmt.Fprintf(w, "  %s = %s\n", p.Name, p.Value)
		return err
	}

	if _, err := fmt.Fprintf(w, "  %s:\n", p.Name); err != nil {
		return err
	}
	for _, s := range structs {
		fields := make([]string, 0, len(s.Params))
		for _, f := range s.Params {
			fields = append(fields, f.Name+"="+valueString(f.Value))
		}
		if _, err := fmt.Fprintf(w, "    %s\n", strings.Join(fields, " ")); err != nil {
			return err
		}
	}
	return nil
}

// unset reports whether v leaves the parameter to the parent template: no
// value at all, a list that appends nothing, or an array of unset elements.
func unset(v objtpl.Value) bool {
	switch v := v.(type) {
	case nil:
		return true
	case objtpl.List:
		if len(v.Items) == 0 {
			return v.Append
		}
		for _, item := range v.Items {
			if item != nil {
				return false
			}
		}
		return true
	}
	return false
}

// structItems returns the structs of a non-empty list made of nothing else.
func structItems(v objtpl.Value) ([]objtpl.Struct, bool) {
	l, ok := v.(objtpl.List)
	if !ok || len(l.Items) == 0 {
		return nil, false
	}
	structs := make([]objtpl.Struct, 0, len(l.Items))
	for _, item := range l.Items {
		s, ok := item.(objtpl.Struct)
		if !ok {
			return nil, false
		}
		structs = append(structs, s)
	}
	return structs, true
}

// valueString renders a struct field, which unlike a top-level parameter is
// shown even when unset, as a dash, so every struct in a list shows the same
// fields.
func valueString(v objtpl.Value) string {
	if v == nil {
		return "-"
	}
	return v.String()
}

// printTable writes a datatable one row per line, tab-separated, header row
// first, so the output pipes into cut, awk and friends the way .stf output
// does.
func printTable(w io.Writer, table *dtable.Table) error {
	names := make([]string, len(table.Columns))
	for i, c := range table.Columns {
		names[i] = c.Name
	}
	if _, err := fmt.Fprintln(w, strings.Join(names, "\t")); err != nil {
		return err
	}

	cells := make([]string, len(table.Columns))
	for _, row := range table.Rows {
		for i, v := range row {
			cells[i] = fmt.Sprint(v)
		}
		if _, err := fmt.Fprintln(w, strings.Join(cells, "\t")); err != nil {
			return err
		}
	}
	return nil
}

// printLayout writes a portal object as two tab-separated tables, cells first
// and then the portals between them, each with a header row. Two tables rather
// than one because a cell with no portals would otherwise not be printed at
// all, and the pair reads as the graph the file describes.
func printLayout(w io.Writer, l *pob.Layout) error {
	if _, err := fmt.Fprint(w, "cell\tname\tappearance\tfloor\tlights\n"); err != nil {
		return err
	}
	for i, c := range l.Cells {
		if _, err := fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%d\n", i, c.Name, c.Appearance, c.Floor, c.Lights); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprint(w, "\ncell\tportal\ttarget\tpassable\tdisabled\tclockwise\tdoor\tvertices\tx\ty\tz\n"); err != nil {
		return err
	}
	for i, c := range l.Cells {
		for _, p := range c.Portals {
			// Decode has already checked the index, so the geometry is there.
			geometry := l.Portals[p.Portal]
			center := geometry.Center()
			_, err := fmt.Fprintf(w, "%d\t%d\t%d\t%t\t%t\t%t\t%s\t%d\t%.2f\t%.2f\t%.2f\n",
				i, p.Portal, p.Target, p.Passable, p.Disabled, p.Clockwise, p.Door,
				len(geometry.Vertices), center.X, center.Y, center.Z)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// colorize resolves the -color flag against the writer cat will print to.
func colorize(when string, stdout io.Writer) (bool, error) {
	switch when {
	case "always":
		return true, nil
	case "never":
		return false, nil
	case "auto":
		// NO_COLOR is honoured whatever its value, as its convention asks.
		_, mute := os.LookupEnv("NO_COLOR")
		return !mute && isTerminal(stdout), nil
	default:
		return false, fmt.Errorf("-color: %q is not auto, always or never", when)
	}
}

// isTerminal reports whether w writes to a character device, which is as close
// to a terminal test as the standard library gets.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// printPalette writes a palette a header row first and then one colour per
// line: the index, the colour as #rrggbb, then its three channels. The plain
// form is tab-separated so it pipes into cut and awk the way datatable output
// does; the coloured form prefixes a swatch and aligns the columns, which tabs
// cannot do once an escape sequence has thrown the column count off.
func printPalette(w io.Writer, p *pal.Palette, color bool) error {
	if !color {
		if _, err := fmt.Fprint(w, "index\thex\tr\tg\tb\n"); err != nil {
			return err
		}
		for i, c := range p.Colors {
			if _, err := fmt.Fprintf(w, "%d\t%s\t%d\t%d\t%d\n", i, c.Hex(), c.R, c.G, c.B); err != nil {
				return err
			}
		}
		return nil
	}

	// The four leading spaces stand in for the swatch column, which has no name.
	if _, err := fmt.Fprintf(w, "    %5s  %-7s %3s %3s %3s\n", "index", "hex", "r", "g", "b"); err != nil {
		return err
	}
	for i, c := range p.Colors {
		if _, err := fmt.Fprintf(w, "%s  %5d  %-7s %3d %3d %3d\n", swatch(c), i, c.Hex(), c.R, c.G, c.B); err != nil {
			return err
		}
	}
	return nil
}

// swatch renders two spaces filled with c, using the 24 bit background colour
// escape every terminal emulator worth the name has supported for a decade.
func swatch(c pal.Color) string {
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm  \x1b[0m", c.R, c.G, c.B)
}

// printTree writes n and its descendants as an indented tree: each node's
// tag and size, plus a short hex/ASCII preview of a leaf's payload, or with
// full set, the whole payload dumped beneath the leaf.
func printTree(w io.Writer, n *iff.Node, depth int, full bool) error {
	indent := strings.Repeat("  ", depth)
	if n.IsForm() {
		if _, err := fmt.Fprintf(w, "%sFORM %s (%d bytes)\n", indent, n.Type, n.Size); err != nil {
			return err
		}
		for _, c := range n.Children {
			if err := printTree(w, c, depth+1, full); err != nil {
				return err
			}
		}
		return nil
	}
	if full {
		if _, err := fmt.Fprintf(w, "%s%s (%d bytes)\n", indent, n.Tag, n.Size); err != nil {
			return err
		}
		return hexdump(w, n.Data, indent+"  ")
	}
	_, err := fmt.Fprintf(w, "%s%s (%d bytes): %s\n", indent, n.Tag, n.Size, preview(n.Data))
	return err
}

// hexdump writes b sixteen bytes to a line, each line indented and holding the
// offset, the bytes in hex in two groups of eight, then the bytes as ASCII
// with non-printable ones shown as a dot.
func hexdump(w io.Writer, b []byte, indent string) error {
	for off := 0; off < len(b); off += 16 {
		row := b[off:min(off+16, len(b))]

		var line strings.Builder
		fmt.Fprintf(&line, "%s%08x  ", indent, off)
		for i := range 16 {
			if i == 8 {
				line.WriteByte(' ')
			}
			if i < len(row) {
				fmt.Fprintf(&line, "%02x ", row[i])
			} else {
				line.WriteString("   ")
			}
		}
		line.WriteByte(' ')
		for _, c := range row {
			line.WriteByte(printable(c))
		}
		line.WriteByte('\n')

		if _, err := io.WriteString(w, line.String()); err != nil {
			return err
		}
	}
	return nil
}

// printable returns c if it is printable ASCII and a dot if not.
func printable(c byte) byte {
	if c >= 0x20 && c < 0x7f {
		return c
	}
	return '.'
}

// previewLen caps how many bytes of a leaf's payload are shown.
const previewLen = 16

// preview renders the first bytes of b as hex alongside their ASCII form,
// with non-printable bytes shown as a dot.
func preview(b []byte) string {
	shown := b
	truncated := len(b) > previewLen
	if truncated {
		shown = b[:previewLen]
	}

	hex := make([]string, len(shown))
	ascii := make([]byte, len(shown))
	for i, c := range shown {
		hex[i] = fmt.Sprintf("%02x", c)
		ascii[i] = printable(c)
	}

	s := strings.Join(hex, " ") + "  " + string(ascii)
	if truncated {
		s += "..."
	}
	return s
}

// stringID is the @file part of the prefix on a string table's lines. Tables
// live under string/<locale>/ and the client names them by what follows with
// the extension dropped, so @zone_n:yavin4 is a string id as the game's own
// data writes it.
func stringID(path string) string {
	rest := strings.TrimSuffix(path, gopath.Ext(path))
	if after, ok := strings.CutPrefix(rest, "string/"); ok {
		if i := strings.Index(after, "/"); i >= 0 {
			rest = after[i+1:]
		}
	}
	return "@" + rest
}
