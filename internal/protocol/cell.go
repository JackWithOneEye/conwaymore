package protocol

import "encoding/binary"

const bytesPerCell = 9

type Cell struct {
	X, Y, Age uint16
	Colour    uint32
}

// encodeCells writes cells in the fixed 9 bytes per cell format: x, y, colour (3 bytes), age.
func encodeCells(src []Cell, dest []byte) {
	for i, cell := range src {
		d := dest[i*bytesPerCell : (i+1)*bytesPerCell]
		binary.BigEndian.PutUint16(d[0:], cell.X)
		binary.BigEndian.PutUint16(d[2:], cell.Y)
		d[4], d[5], d[6] = byte(cell.Colour>>16), byte(cell.Colour>>8), byte(cell.Colour)
		binary.BigEndian.PutUint16(d[7:], cell.Age)
	}
}

func decodeCells(src []byte, dest []Cell) {
	for i := range dest {
		s := src[i*bytesPerCell : (i+1)*bytesPerCell]
		dest[i] = Cell{
			X:      binary.BigEndian.Uint16(s[0:]),
			Y:      binary.BigEndian.Uint16(s[2:]),
			Colour: uint32(s[4])<<16 | uint32(s[5])<<8 | uint32(s[6]),
			Age:    binary.BigEndian.Uint16(s[7:]),
		}
	}
}
