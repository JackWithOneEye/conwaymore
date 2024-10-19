package protocol

import "errors"

type Output struct {
	Cells      []*Cell
	CellsCount uint32
	Playing    bool
	Speed      uint16
}

func (o *Output) Encode() []byte {
	cellsCount := uint32(len(o.Cells))
	b := make([]byte, 1+2+3+cellsCount*bytesPerCell)

	if o.Playing {
		b[0] = 1
	}

	b[1] = byte(o.Speed >> 8)
	b[2] = byte(o.Speed & 0x00ff)

	b[3] = byte((cellsCount & 0xff0000) >> 16)
	b[4] = byte((cellsCount & 0xff00) >> 8)
	b[5] = byte(cellsCount & 0xff)

	encodeCells(o.Cells, b, 6)

	return b
}

func (o *Output) Decode(b []byte) error {
	l := len(b)
	if l < 6 {
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

	o.Cells = make([]*Cell, o.CellsCount)

	decodeCells(b, o.Cells, 6)

	return nil
}
