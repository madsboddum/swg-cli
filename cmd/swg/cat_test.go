package main

import (
	"bytes"
	"encoding/binary"
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

func TestHelpCatShowsTheLongUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"help", "cat"}, &stdout, &stderr); code != 0 {
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
