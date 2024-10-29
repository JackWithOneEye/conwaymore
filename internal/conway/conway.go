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
	Cells() iter.Seq[Cell]
	CellsCount() uint
	Clear()
	NextGen()
	Randomise()
	SetCell(x, y uint16, colour uint32, age uint16)
}

type conway struct {
	axisLength    uint
	wrapMask      uint16
	numCells      uint
	numAliveCells uint32
	output        []aliveCell
}

func NewConway(cfg ConwayConfig) Conway {
	axisLength := cfg.WorldSize()
	numCells := axisLength * axisLength

	return &conway{
		axisLength:    axisLength,
		wrapMask:      uint16(axisLength) - 1,
		numCells:      numCells,
		numAliveCells: 0,
		output:        make([]aliveCell, numCells),
	}
}

func (c *conway) CanSetCell(x, y uint16) bool {
	for i := 0; i < int(c.numAliveCells); i++ {
		cell := &c.output[i]
		if cell.x == x && cell.y == y {
			return false
		}
	}
	return true
}

func (c *conway) Clear() {
	c.numAliveCells = 0
}

func (c *conway) Cells() iter.Seq[Cell] {
	return func(yield func(Cell) bool) {
		for i := range c.numAliveCells {
			if !yield(&c.output[i]) {
				return
			}
		}
	}
}

func (c *conway) CellsCount() uint {
	return uint(c.numAliveCells)
}

func (c *conway) NextGen() {
	candidates := make(map[uint32]*cellCandidate, c.numAliveCells)

	for i := range c.numAliveCells {
		cell := &c.output[i]

		x := cell.x
		xLeft := (x - 1) & c.wrapMask
		xRight := (x + 1) & c.wrapMask

		y := cell.y
		yUp := (y - 1) & c.wrapMask
		yDown := (y + 1) & c.wrapMask

		setNeighbourAsCandidate(cell, xLeft, yUp, candidates)
		setNeighbourAsCandidate(cell, x, yUp, candidates)
		setNeighbourAsCandidate(cell, xRight, yUp, candidates)

		setNeighbourAsCandidate(cell, xLeft, y, candidates)
		setCellAsCandidate(cell, candidates)
		setNeighbourAsCandidate(cell, xRight, y, candidates)

		setNeighbourAsCandidate(cell, xLeft, yDown, candidates)
		setNeighbourAsCandidate(cell, x, yDown, candidates)
		setNeighbourAsCandidate(cell, xRight, yDown, candidates)
	}

	c.numAliveCells = 0
	for _, cand := range candidates {
		if cand.count < 2 || cand.count > 3 {
			continue
		}
		if !cand.alive {
			if cand.count == 2 {
				continue
			}
			cand.create()
		} else if cand.age < math.MaxUint16 {
			cand.age += 1
		}

		c.output[c.numAliveCells].set(cand.x, cand.y, cand.colour, cand.age)
		c.numAliveCells += 1
	}
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
			c.output[c.numAliveCells].set(x, y, colour, 0)
			c.numAliveCells += 1
		}
	}
}

func (c *conway) SetCell(x, y uint16, colour uint32, age uint16) {
	ac := &c.output[c.numAliveCells]
	ac.set(x, y, colour, age)
	c.numAliveCells += 1
}

func makeCoord(x, y uint16) uint32 {
	return (uint32(x) << 16) | uint32(y)
}

func setCellAsCandidate(cell *aliveCell, candidates map[uint32]*cellCandidate) {
	coord := makeCoord(cell.x, cell.y)
	cand, ok := candidates[coord]
	if ok {
		cand.markAsAlive(cell)
		return
	}
	candidates[coord] = &cellCandidate{
		x:               cell.x,
		y:               cell.y,
		colour:          cell.colour,
		age:             cell.age,
		alive:           true,
		aliveNeighbours: make([]aliveCell, 3),
		count:           0,
	}
}

func setNeighbourAsCandidate(cell *aliveCell, x, y uint16, candidates map[uint32]*cellCandidate) {
	coord := makeCoord(x, y)
	cand, ok := candidates[coord]
	if ok {
		cand.addNeighbour(cell)
		return
	}
	an := make([]aliveCell, 3)
	an[0].copy(cell)
	candidates[coord] = &cellCandidate{
		x:               x,
		y:               y,
		alive:           false,
		aliveNeighbours: an,
		count:           1,
	}
}
