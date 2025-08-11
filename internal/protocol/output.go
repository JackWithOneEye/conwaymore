package protocol

import (
	"errors"
)

const cellsOffset = 6

type Output struct {
	Cells      []Cell
	CellsCount uint32
	Playing    bool
	Speed      uint16
}

func (o *Output) Encode(b []byte) {
	// b := make([]byte, cellsOffset+cellsCount*bytesPerCell)

	if o.Playing {
		b[0] = 1
	} else {
		b[0] = 0
	}

	b[1] = byte(o.Speed >> 8)
	b[2] = byte(o.Speed & 0x00ff)

	b[3] = byte((o.CellsCount & 0xff0000) >> 16)
	b[4] = byte((o.CellsCount & 0xff00) >> 8)
	b[5] = byte(o.CellsCount & 0xff)

	encodeCells(o.Cells, o.CellsCount, b, 6)
}

func (o *Output) EncodeSize() uint32 {
	return cellsOffset + o.CellsCount*bytesPerCell
}

func (o *Output) Decode(b []byte) error {
	l := len(b)
	if l < cellsOffset {
		return errors.New("too short")
	}
	if b[0] == 1 {
		o.Playing = true
	}
	o.Speed = (uint16(b[1]) << 8) | uint16(b[2])
	o.CellsCount = (uint32(b[3]) << 16) | (uint32(b[4]) << 8) | uint32(b[5])

	if l < int(6+o.CellsCount*bytesPerCell) {
		return errors.New("byte length deos not match cells count")
	}

	o.Cells = make([]Cell, o.CellsCount)

	decodeCells(b, o.Cells, cellsOffset)

	return nil
}
