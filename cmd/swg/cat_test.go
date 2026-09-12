package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestCat(t *testing.T) {
	dir := catFixture(t)

	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{
			name: "string table is decoded",
			args: []string{"cat", "string/en/combat_effects.stf"},
			wantOut: "@combat_effects:cover|You take cover.\n" +
				"@combat_effects:disturbance|You feel a disturbance in the Force.\n",
		},
		{
			name:    "the locale directory is dropped but subdirectories are kept",
			args:    []string{"cat", "string/en/sub/deep.stf"},
			wantOut: "@sub/deep:label|Inventory\n",
		},
		{
			name:    "a table outside string keeps the rest of its path",
			args:    []string{"cat", "other/stray.stf"},
			wantOut: "@other/stray:label|Inventory\n",
		},
		{
			name: "a pattern concatenates every match",
			args: []string{"cat", "string/en/**.stf"},
			wantOut: "@combat_effects:cover|You take cover.\n" +
				"@combat_effects:disturbance|You feel a disturbance in the Force.\n" +
				"@sub/deep:label|Inventory\n",
		},
		{
			name:    "other files are written out as bytes",
			args:    []string{"cat", "texture/crate.dds"},
			wantOut: "raw crate bytes",
		},
		{
			name: "an IFF container is rendered as a tree, whatever its extension",
			args: []string{"cat", "appearance/crate.apt"},
			wantOut: "FORM TEST (17 bytes)\n" +
				"  NAME (5 bytes): 68 65 6c 6c 6f  hello\n",
		},
		{
			name: "a DTII datatable renders as tab-separated rows",
			args: []string{"cat", "datatable/npc.iff"},
			wantOut: "level\tname\n" +
				"5\twomp rat\n",
		},
		{
			name: "a malformed DTII falls back to the node tree",
			args: []string{"cat", "datatable/broken.iff"},
			wantOut: "FORM DTII (16 bytes)\n" +
				"  FORM 0001 (4 bytes)\n",
		},
		{
			name: "a portal object renders as cells and the portals between them",
			args: []string{"cat", "appearance/hut.pob"},
			wantOut: "cell\tname\tappearance\tfloor\tlights\n" +
				"0\tshell\tshell.msh\t\t0\n" +
				"1\troom\troom.msh\troom.flr\t1\n" +
				"\ncell\tportal\ttarget\tpassable\tdisabled\tclockwise\tdoor\tvertices\tx\ty\tz\n" +
				"0\t0\t1\ttrue\tfalse\ttrue\t\t4\t1.00\t1.50\t0.00\n" +
				"1\t0\t0\tfalse\tfalse\tfalse\tdoor_hut\t4\t1.00\t1.50\t0.00\n",
		},
		{
			name: "a malformed portal object falls back to the node tree",
			args: []string{"cat", "appearance/broken.pob"},
			wantOut: "FORM PRTO (16 bytes)\n" +
				"  FORM 0003 (4 bytes)\n",
		},
		{
			name: "a palette renders tab-separated when the output is not a terminal",
			args: []string{"cat", "palette/frog.pal"},
			wantOut: "index\thex\tr\tg\tb\n" +
				"0\t#5d3534\t93\t53\t52\n" +
				"1\t#ffffff\t255\t255\t255\n",
		},
		{
			name: "-color always adds a swatch and aligns the columns",
			args: []string{"cat", "-color", "always", "palette/frog.pal"},
			wantOut: "    index  hex       r   g   b\n" +
				"\x1b[48;2;93;53;52m  \x1b[0m      0  #5d3534  93  53  52\n" +
				"\x1b[48;2;255;255;255m  \x1b[0m      1  #ffffff 255 255 255\n",
		},
		{
			name:    "a malformed palette falls back to its bytes",
			args:    []string{"cat", "palette/broken.pal"},
			wantOut: "RIFF\x00\x00\x00\x00PAL nope",
		},
		{
			name:     "an unknown -color value",
			args:     []string{"cat", "-color", "sometimes", "palette/frog.pal"},
			wantCode: 2,
			wantErr:  `"sometimes" is not auto, always or never`,
		},
		{
			name:    "the winning source is the one read",
			args:    []string{"cat", "texture/patched.dds"},
			wantOut: "loose wins",
		},
		{
			name:    "several operands concatenate in order",
			args:    []string{"cat", "texture/crate.dds", "string/en/sub/deep.stf"},
			wantOut: "raw crate bytes@sub/deep:label|Inventory\n",
		},
		{
			name:     "a pattern matching nothing",
			args:     []string{"cat", "string/de/*.stf"},
			wantCode: 1,
			wantErr:  "no such path",
		},
		{
			name:     "a directory is not expanded",
			args:     []string{"cat", "string/en"},
			wantCode: 1,
			wantErr:  "no such path",
		},
		{
			name:     "malformed pattern",
			args:     []string{"cat", "string/[en"},
			wantCode: 2,
			wantErr:  "syntax error in pattern",
		},
		{
			name:     "a table that will not decode",
			args:     []string{"cat", "bad/broken.stf"},
			wantCode: 1,
			wantErr:  "not a supported .stf file",
		},
		{
			name:     "one operand of several fails",
			args:     []string{"cat", "texture/crate.dds", "gone.txt"},
			wantOut:  "raw crate bytes",
			wantCode: 1,
			wantErr:  "gone.txt",
		},
		{
			name:     "no operands",
			args:     []string{"cat"},
			wantCode: 2,
			wantErr:  "usage: swg cat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SWG_DIR", dir)
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, tt.wantCode, stderr.String())
			}
			if got := stdout.String(); got != tt.wantOut {
				t.Errorf("stdout = %q, want %q", got, tt.wantOut)
			}
			if tt.wantErr != "" && !strings.Contains(stderr.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantErr)
			}
		})
	}
}

func TestCatDirFlagBeatsEnvironment(t *testing.T) {
	dir := catFixture(t)
	t.Setenv("SWG_DIR", t.TempDir())

	var stdout, stderr bytes.Buffer
	if code := run([]string{"cat", "-dir", dir, "texture/crate.dds"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr.String())
	}
	if got := stdout.String(); got != "raw crate bytes" {
		t.Errorf("stdout = %q", got)
	}
}

func TestCatWithoutDirectoryConfigured(t *testing.T) {
	t.Setenv("SWG_DIR", "")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"cat", "a.stf"}, &stdout, &stderr); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "SWG_DIR") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestColorizeAutoFollowsTheWriterAndNoColor(t *testing.T) {
	tty, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Skipf("no character device to test against: %v", err)
	}
	defer func() { _ = tty.Close() }()

	set := func(v string) *string { return &v }

	tests := []struct {
		name string
		when string
		w    io.Writer
		// noColor is the value NO_COLOR holds, or nil for unset. An empty
		// NO_COLOR still mutes, so the two cannot share a representation.
		noColor *string
		want    bool
	}{
		{name: "auto on a character device", when: "auto", w: tty, want: true},
		{name: "auto on a pipe", when: "auto", w: &bytes.Buffer{}},
		{name: "auto with NO_COLOR set", when: "auto", w: tty, noColor: set("1")},
		{name: "auto with NO_COLOR empty", when: "auto", w: tty, noColor: set("")},
		{name: "always on a pipe", when: "always", w: &bytes.Buffer{}, want: true},
		{name: "never on a character device", when: "never", w: tty},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setenv first either way, so the cleanup it registers restores
			// whatever the environment held.
			t.Setenv("NO_COLOR", "")
			if tt.noColor == nil {
				if err := os.Unsetenv("NO_COLOR"); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Setenv("NO_COLOR", *tt.noColor)
			}

			got, err := colorize(tt.when, tt.w)
			if err != nil {
				t.Fatalf("colorize: %v", err)
			}
			if got != tt.want {
				t.Errorf("colorize(%q) = %v, want %v", tt.when, got, tt.want)
			}
		})
	}
}

func TestHelpCatShowsTheLongUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"cat", "--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "@file:key|value") {
		t.Errorf("stdout = %q, want the output format documented", stdout.String())
	}
}

// catFixture builds an archive directory holding string tables, a file that is
// not one, and a loose file shadowing an archive member.
func catFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeTestArchive(t, filepath.Join(dir, "base.tre"), map[string]string{
		"string/en/combat_effects.stf": string(stfBytes(t, []stfEntry{
			{"disturbance", "You feel a disturbance in the Force."},
			{"cover", "You take cover."},
		})),
		"string/en/sub/deep.stf": string(stfBytes(t, []stfEntry{{"label", "Inventory"}})),
		"other/stray.stf":        string(stfBytes(t, []stfEntry{{"label", "Inventory"}})),
		"bad/broken.stf":         "not a string table",
		"texture/crate.dds":      "raw crate bytes",
		"texture/patched.dds":    "archive loses",
		"appearance/crate.apt":   string(iffBytes(t, "TEST", "NAME", "hello")),
		"datatable/npc.iff":      string(dtableBytes(t)),
		"datatable/broken.iff":   string(form("DTII", form("0001"))),
		"appearance/hut.pob":     string(pobBytes(t)),
		"appearance/broken.pob":  string(form("PRTO", form("0003"))),
		"palette/frog.pal": string(palBytes(t, []palEntry{
			{0x5d, 0x35, 0x34},
			{0xff, 0xff, 0xff},
		})),
		"palette/broken.pal": "RIFF\x00\x00\x00\x00PAL nope",
	})

	loose := filepath.Join(dir, "texture", "patched.dds")
	if err := os.MkdirAll(filepath.Dir(loose), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loose, []byte("loose wins"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// iffBytes builds a FORM of the given type holding one leaf chunk.
func iffBytes(t *testing.T, formType, leafTag, leafData string) []byte {
	t.Helper()

	var body bytes.Buffer
	body.WriteString(formType)
	body.WriteString(leafTag)
	if err := binary.Write(&body, binary.BigEndian, uint32(len(leafData))); err != nil {
		t.Fatal(err)
	}
	body.WriteString(leafData)

	var out bytes.Buffer
	out.WriteString("FORM")
	if err := binary.Write(&out, binary.BigEndian, uint32(body.Len())); err != nil {
		t.Fatal(err)
	}
	out.Write(body.Bytes())
	return out.Bytes()
}

// chunk lays out a leaf chunk: a 4CC tag, its big-endian length, then data.
func chunk(tag string, data []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString(tag)
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(data))); err != nil {
		panic(err)
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

// dtableBytes builds a version 0001 DTII datatable of one int column and one
// string column, holding a single row.
func dtableBytes(t *testing.T) []byte {
	t.Helper()

	cstrings := func(s ...string) []byte {
		var buf bytes.Buffer
		if err := binary.Write(&buf, binary.LittleEndian, uint32(len(s))); err != nil {
			t.Fatal(err)
		}
		for _, v := range s {
			buf.WriteString(v)
			buf.WriteByte(0)
		}
		return buf.Bytes()
	}

	var rows bytes.Buffer
	if err := binary.Write(&rows, binary.LittleEndian, uint32(1)); err != nil {
		t.Fatal(err)
	}
	if err := binary.Write(&rows, binary.LittleEndian, int32(5)); err != nil {
		t.Fatal(err)
	}
	rows.WriteString("womp rat")
	rows.WriteByte(0)

	return form("DTII",
		form("0001",
			chunk("COLS", cstrings("level", "name")),
			chunk("TYPE", []byte("i\x00s\x00")),
			chunk("ROWS", rows.Bytes()),
		),
	)
}

// pobBytes builds a version 0003 PRTO portal object: one portal polygon and
// two cells joined through it, the second behind a door.
func pobBytes(t *testing.T) []byte {
	t.Helper()

	le := func(vs ...any) []byte {
		var buf bytes.Buffer
		for _, v := range vs {
			if err := binary.Write(&buf, binary.LittleEndian, v); err != nil {
				t.Fatal(err)
			}
		}
		return buf.Bytes()
	}
	// quad is a two by three opening in the x/y plane, so its centre is 1, 1.5, 0.
	quad := le(int32(4),
		float32(0), float32(0), float32(0),
		float32(2), float32(0), float32(0),
		float32(2), float32(3), float32(0),
		float32(0), float32(3), float32(0))

	// cell is a version 0005 DATA chunk: portal count, parent visibility, name,
	// appearance, then the floor behind a flag.
	cell := func(name, appearance, floor string) []byte {
		var buf bytes.Buffer
		buf.Write(le(int32(1), byte(0)))
		buf.WriteString(name + "\x00" + appearance + "\x00")
		if floor == "" {
			buf.WriteByte(0)
			return buf.Bytes()
		}
		buf.WriteByte(1)
		buf.WriteString(floor + "\x00")
		return buf.Bytes()
	}

	// lights is a LGHT chunk: a count, then that many light records of 93
	// bytes, which cat counts rather than decodes.
	lights := func(n int) []byte {
		return append(le(int32(n)), make([]byte, 93*n)...)
	}

	// portal is a version 0004 cell portal: passable, geometry, winding, target,
	// door, then the door's hardpoint flag and transform, which cat ignores.
	portal := func(passable byte, clockwise byte, target int32, door string) []byte {
		var buf bytes.Buffer
		buf.Write(le(passable, int32(0), clockwise, target))
		buf.WriteString(door + "\x00")
		buf.WriteByte(0)
		buf.Write(make([]byte, 48))
		return buf.Bytes()
	}

	return form("PRTO",
		form("0003",
			chunk("DATA", le(int32(1), int32(2))),
			form("PRTS", chunk("PRTL", quad)),
			form("CELS",
				form("CELL", form("0005",
					chunk("DATA", cell("shell", "shell.msh", "")),
					form("PRTL", chunk("0004", portal(1, 1, 1, ""))),
					chunk("LGHT", lights(0)),
				)),
				form("CELL", form("0005",
					chunk("DATA", cell("room", "room.msh", "room.flr")),
					form("PRTL", chunk("0004", portal(0, 0, 0, "door_hut"))),
					chunk("LGHT", lights(1)),
				)),
			),
			chunk("CRC ", le(uint32(1))),
		),
	)
}

// palEntry is one colour of a palette fixture.
type palEntry struct{ r, g, b uint8 }

// palBytes builds a RIFF PAL palette holding entries.
func palBytes(t *testing.T, entries []palEntry) []byte {
	t.Helper()

	var data bytes.Buffer
	write := func(v any) {
		if err := binary.Write(&data, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	write(uint16(0x0300))
	write(uint16(len(entries)))
	for _, e := range entries {
		data.Write([]byte{e.r, e.g, e.b, 0})
	}

	var body bytes.Buffer
	body.WriteString("PAL data")
	if err := binary.Write(&body, binary.LittleEndian, uint32(data.Len())); err != nil {
		t.Fatal(err)
	}
	body.Write(data.Bytes())

	var out bytes.Buffer
	out.WriteString("RIFF")
	if err := binary.Write(&out, binary.LittleEndian, uint32(body.Len())); err != nil {
		t.Fatal(err)
	}
	out.Write(body.Bytes())
	return out.Bytes()
}

// stfEntry is one key and value of a string table fixture.
type stfEntry struct{ key, value string }

// stfBytes builds a version 1 string table holding entries.
func stfBytes(t *testing.T, entries []stfEntry) []byte {
	t.Helper()

	var out bytes.Buffer
	write := func(v any) {
		if err := binary.Write(&out, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}

	write(uint32(0x0000abcd))
	out.WriteByte(1)
	write(uint32(len(entries) + 1)) // next id the client would hand out
	write(uint32(len(entries)))

	for i, e := range entries {
		u := utf16.Encode([]rune(e.value))
		write(uint32(i + 1))
		write(uint32(0)) // value checksum, unused
		write(uint32(len(u)))
		for _, c := range u {
			write(c)
		}
	}
	for i, e := range entries {
		write(uint32(i + 1))
		write(uint32(len(e.key)))
		out.WriteString(e.key)
	}
	return out.Bytes()
}
