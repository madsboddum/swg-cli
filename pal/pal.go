// Package pal reads .pal palettes, the colour tables Star Wars Galaxies uses
// to recolour customisable assets. They are ordinary Microsoft RIFF PAL files,
// the same layout Windows has written since 3.0, holding an indexed list of
// colours that customisation variables address by index.
package pal

//go:generate go run ./testdata/gen

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	// Magic is the tag every RIFF file opens with.
	Magic = "RIFF"
	// FormType is the 4CC naming the RIFF form as a palette. Note the trailing
	// space; RIFF tags are always four bytes.
	FormType = "PAL "

	dataTag = "data"
	// version is the only palette version Windows ever defined, and the only
	// one the game's files carry.
	version = 0x0300

	// headerSize covers RIFF, its size, the form type, the data tag, its size,
	// the version and the entry count.
	headerSize = 24
	entrySize  = 4
)

// ErrFormat reports that the file is not a palette this package can read.
var ErrFormat = errors.New("pal: not a supported .pal palette")

// Color is one entry of a palette.
type Color struct{ R, G, B uint8 }

// RGBA implements image/color.Color, so entries can be handed to the image
// packages without conversion. Palette entries are always opaque.
func (c Color) RGBA() (r, g, b, a uint32) {
	return uint32(c.R) * 0x101, uint32(c.G) * 0x101, uint32(c.B) * 0x101, 0xffff
}

// Hex renders the colour as #rrggbb.
func (c Color) Hex() string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// Palette is a decoded palette. Index order is meaningful: it is what the
// customisation data refers to.
type Palette struct {
	Colors []Color
}

// HasMagic reports whether b opens with a RIFF palette header, the cheap sniff
// a caller makes before committing to Decode.
func HasMagic(b []byte) bool {
	return len(b) >= 12 && string(b[0:4]) == Magic && string(b[8:12]) == FormType
}

// Decode reads a palette from b.
func Decode(b []byte) (*Palette, error) {
	if len(b) < headerSize {
		return nil, fmt.Errorf("%w: file is %d bytes, too short for a header", ErrFormat, len(b))
	}
	if !HasMagic(b) {
		return nil, fmt.Errorf("%w: tags are %q and %q, want %q and %q", ErrFormat, b[0:4], b[8:12], Magic, FormType)
	}
	// The RIFF size at b[4:8] goes unchecked on purpose: three of the game's
	// palettes understate it and are otherwise sound, so trusting it would
	// reject files the client itself reads.
	if got := string(b[12:16]); got != dataTag {
		return nil, fmt.Errorf("%w: chunk after the form type is %q, want %q", ErrFormat, got, dataTag)
	}

	dataSize := int(binary.LittleEndian.Uint32(b[16:]))
	if dataSize > len(b)-20 {
		return nil, fmt.Errorf("%w: data chunk claims %d bytes, the file has %d left", ErrFormat, dataSize, len(b)-20)
	}
	if got := binary.LittleEndian.Uint16(b[20:]); got != version {
		return nil, fmt.Errorf("%w: version is %#04x, want %#04x", ErrFormat, got, version)
	}

	// The count, not the data size, decides how many entries there are. Some
	// files carry padding entries past the count that the game ignores.
	count := int(binary.LittleEndian.Uint16(b[22:]))
	if end := headerSize + count*entrySize; end > len(b) {
		return nil, fmt.Errorf("%w: header claims %d entries, the file has room for %d", ErrFormat, count, (len(b)-headerSize)/entrySize)
	}

	p := &Palette{Colors: make([]Color, count)}
	for i := range p.Colors {
		e := b[headerSize+i*entrySize:]
		// e[3] is the PALETTEENTRY flags byte. It is uniform within a file and
		// carries no meaning here, so it is dropped rather than read as alpha.
		p.Colors[i] = Color{R: e[0], G: e[1], B: e[2]}
	}
	return p, nil
}
