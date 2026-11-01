package protocol

import (
	"encoding/binary"
	"errors"
	"math"
	"slices"
)

const cellsOffset = 6

type Output struct {
	Cells      []Cell
	CellsCount uint32
	Playing    bool
	Speed      uint16
}

// Encode appends the encoded output to b and returns the extended buffer.
//
// After a 6 byte header (playing, speed, cells count), the cells are stored column by column
// so that the WebSocket compression finds more repetition:
//   - positions: one uvarint per cell, the distance of y<<16|x to the previous cell's
//   - colours: 3 bytes per cell
//   - ages: one uvarint per cell
//
// Cells sorted by y, then x, give the smallest distances; other orders still decode correctly.
func (o *Output) Encode(b []byte) []byte {
	var playing byte
	if o.Playing {
		playing = 1
	}
	b = append(b, playing, byte(o.Speed>>8), byte(o.Speed), byte(o.CellsCount>>16), byte(o.CellsCount>>8), byte(o.CellsCount))

	cells := o.Cells[:o.CellsCount]
	var prev uint32
	for _, c := range cells {
		pos := uint32(c.Y)<<16 | uint32(c.X)
		b = binary.AppendUvarint(b, uint64(pos-prev))
		prev = pos
	}
	for _, c := range cells {
		b = append(b, byte(c.Colour>>16), byte(c.Colour>>8), byte(c.Colour))
	}
	for _, c := range cells {
		b = binary.AppendUvarint(b, uint64(c.Age))
	}
	return b
}

// Decode decodes b into o, reusing the capacity of o.Cells.
func (o *Output) Decode(b []byte) error {
	if len(b) < cellsOffset {
		return errors.New("too short")
	}
	o.Playing = b[0] == 1
	o.Speed = binary.BigEndian.Uint16(b[1:])
	o.CellsCount = uint32(b[3])<<16 | uint32(b[4])<<8 | uint32(b[5])
	n := int(o.CellsCount)
	o.Cells = slices.Grow(o.Cells[:0], n)[:n]

	b = b[cellsOffset:]
	var pos uint32
	for i := range o.Cells {
		dist, l := binary.Uvarint(b)
		if l <= 0 {
			return errors.New("invalid cell position")
		}
		b = b[l:]
		pos += uint32(dist)
		o.Cells[i].X = uint16(pos)
		o.Cells[i].Y = uint16(pos >> 16)
	}
	if len(b) < 3*n {
		return errors.New("byte length does not match cells count")
	}
	for i := range o.Cells {
		o.Cells[i].Colour = uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
		b = b[3:]
	}
	for i := range o.Cells {
		age, l := binary.Uvarint(b)
		if l <= 0 || age > math.MaxUint16 {
			return errors.New("invalid cell age")
		}
		b = b[l:]
		o.Cells[i].Age = uint16(age)
	}
	return nil
}
