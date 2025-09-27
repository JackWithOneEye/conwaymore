package conway

import (
	"iter"
	"math"
	"math/rand/v2"
)

type ConwayConfig interface {
	WorldSize() uint
}

type Conway interface {
	CanSetCell(x, y uint16) bool
	Cells() iter.Seq2[uint, Cell]
	CellsCount() uint
	Clear()
	NextGen()
	Randomise()
	SetCell(x, y uint16, colour uint32, age uint16)
}

type conway struct {
	axisLength uint
	wrapMask   uint16
	aliveCells *swapSet[aliveCell]
	candidates *swapSet[uint16]
}

func NewConway(cfg ConwayConfig) Conway {
	axisLength := cfg.WorldSize()

	return &conway{
		axisLength: axisLength,
		wrapMask:   uint16(axisLength) - 1,
		aliveCells: newSwapSet[aliveCell](axisLength),
		candidates: newSwapSet[uint16](axisLength),
	}
}

func (c *conway) CanSetCell(x, y uint16) bool {
	_, ok := c.aliveCells.get(x, y)
	return !ok
}

func (c *conway) Clear() {
	c.aliveCells.clearAll()
	c.candidates.clearAll()
}

func (c *conway) Cells() iter.Seq2[uint, Cell] {
	return func(yield func(uint, Cell) bool) {
		var i uint
		for _, ac := range c.aliveCells.values() {
			if !yield(i, &ac) {
				return
			}
			i += 1
		}
	}
}

func (c *conway) CellsCount() uint {
	return uint(c.aliveCells.size())
}

func (c *conway) NextGen() {
	c.aliveCells.clearNext()
	c.candidates.clearNext()
	for key := range c.aliveCells.values() {
		c.candidates.addNextByKey(key, 0)
	}

	var aliveNeighbours [8]aliveCell
	var anIdx uint
	findAlive := func(x, y uint16) int {
		if ac, ok := c.aliveCells.get(x, y); ok {
			aliveNeighbours[anIdx] = ac
			anIdx += 1
			return 1
		}
		return 0
	}

	getNumNeighbours := func(xLeft, x, xRight, yUp, y, yDown uint16) int {
		numNeighbours := findAlive(xLeft, yUp) +
			findAlive(x, yUp) +
			findAlive(xRight, yUp) +
			findAlive(xLeft, y)
		if numNeighbours == 4 {
			return numNeighbours
		}
		numNeighbours += findAlive(xRight, y)
		if numNeighbours == 4 {
			return numNeighbours
		}
		numNeighbours += findAlive(xLeft, yDown)
		if numNeighbours == 4 {
			return numNeighbours
		}
		numNeighbours += findAlive(x, yDown)
		if numNeighbours == 4 {
			return numNeighbours
		}
		return numNeighbours + findAlive(xRight, yDown)
	}

	for x, y := range c.candidates.iter() {
		xLeft, xRight, yUp, yDown := c.getAdjacent(x, y)

		anIdx = 0
		numNeighbours := getNumNeighbours(xLeft, x, xRight, yUp, y, yDown)
		ac, alive := c.aliveCells.get(x, y)

		addCands := false
		if alive {
			if numNeighbours == 2 || numNeighbours == 3 {
				if ac.age < math.MaxUint16 {
					ac.age += 1
				}
				c.aliveCells.addNext(x, y, ac)
			} else {
				addCands = true
			}
		} else if numNeighbours == 3 {
			colour := (aliveNeighbours[0].colour & 0xff0000) | (aliveNeighbours[1].colour & 0x00ff00) | (aliveNeighbours[2].colour & 0x0000ff)
			c.aliveCells.addNext(x, y, aliveCell{x, y, colour, 0})
			addCands = true
		}
		if addCands {
			c.candidates.addNext(xLeft, yUp, 0)
			c.candidates.addNext(x, yUp, 0)
			c.candidates.addNext(xRight, yUp, 0)
			c.candidates.addNext(xLeft, y, 0)
			c.candidates.addNext(x, y, 0)
			c.candidates.addNext(xRight, y, 0)
			c.candidates.addNext(xLeft, yDown, 0)
			c.candidates.addNext(x, yDown, 0)
			c.candidates.addNext(xRight, yDown, 0)
		}
	}

	c.aliveCells.swap()
	c.candidates.swap()
}

func (c *conway) Randomise() {
	c.Clear()

	al := uint16(c.axisLength)
	for x := range al {
		for y := range al {
			if rand.UintN(2) != 1 {
				continue
			}
			colour := rand.Uint32N(0xffffff) + 1
			c.aliveCells.add(x, y, aliveCell{x, y, colour, 0})
			c.addCandidates(x, y)
		}
	}
}

func (c *conway) SetCell(x, y uint16, colour uint32, age uint16) {
	c.aliveCells.add(x, y, aliveCell{x, y, colour, age})
	c.addCandidates(x, y)
}

func (c *conway) addCandidates(x, y uint16) {
	xLeft, xRight, yUp, yDown := c.getAdjacent(x, y)
	c.candidates.add(xLeft, yUp, 0)
	c.candidates.add(x, yUp, 0)
	c.candidates.add(xRight, yUp, 0)
	c.candidates.add(xLeft, y, 0)
	c.candidates.add(x, y, 0)
	c.candidates.add(xRight, y, 0)
	c.candidates.add(xLeft, yDown, 0)
	c.candidates.add(x, yDown, 0)
	c.candidates.add(xRight, yDown, 0)
}

func (c *conway) getAdjacent(x, y uint16) (xLeft, xRight, yUp, yDown uint16) {
	xLeft = (x - 1) & c.wrapMask
	xRight = (x + 1) & c.wrapMask
	yUp = (y - 1) & c.wrapMask
	yDown = (y + 1) & c.wrapMask
	return
}
