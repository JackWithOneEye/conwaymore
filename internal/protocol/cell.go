package protocol

const bytesPerCell = 9

type Cell struct {
	X, Y, Age uint16
	Colour    uint32
}

func encodeCells(src []Cell, cellsCount uint32, dest []byte, destOffset uint) {
	dsti := destOffset
	for i := range cellsCount {
		cell := src[i]

		dest[dsti] = byte((cell.X >> 8) & 0xff)
		dsti += 1
		dest[dsti] = byte(cell.X & 0xff)
		dsti += 1

		dest[dsti] = byte((cell.Y >> 8) & 0xff)
		dsti += 1
		dest[dsti] = byte(cell.Y & 0xff)
		dsti += 1

		dest[dsti] = byte((cell.Colour >> 16) & 0xff)
		dsti += 1
		dest[dsti] = byte((cell.Colour >> 8) & 0xff)
		dsti += 1
		dest[dsti] = byte(cell.Colour & 0xff)
		dsti += 1

		dest[dsti] = byte((cell.Age >> 8) & 0xff)
		dsti += 1
		dest[dsti] = byte(cell.Age & 0xff)
		dsti += 1
	}
}

func decodeCells(src []byte, dest []Cell, srcOffset uint) {
	srci := srcOffset
	for i := range dest {
		x := (uint16(src[srci]) << 8) & 0xff00
		srci += 1
		x = x | uint16(src[srci])
		srci += 1

		y := (uint16(src[srci]) << 8) & 0xff00
		srci += 1
		y = y | uint16(src[srci])
		srci += 1

		c := (uint32(src[srci]) << 16) & 0xff0000
		srci += 1
		c = c | ((uint32(src[srci]) << 8) & 0xff00)
		srci += 1
		c = c | uint32(src[srci])
		srci += 1

		a := (uint16(src[srci]) << 8) & 0xff00
		srci += 1
		a = a | uint16(src[srci])
		srci += 1

		dest[i] = Cell{X: x, Y: y, Colour: c, Age: a}
	}
}
