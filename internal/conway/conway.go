package conway

import (
	"iter"
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

func (e *conway) CanSetCell(x, y uint16) bool {
	for i := 0; i < int(e.numAliveCells); i++ {
		if e.output[i].x == x && e.output[i].y == y {
			return false
		}
	}
	return true
}

func (e *conway) Clear() {
	for i := range e.output {
		e.output[i].set(0, 0, 0, 0)
	}
	e.numAliveCells = 0
}

func (e *conway) Cells() iter.Seq[Cell] {
	return func(yield func(Cell) bool) {
		for i := range e.numAliveCells {
			if !yield(&e.output[i]) {
				return
			}
		}
	}
}

func (e *conway) CellsCount() uint {
	return uint(e.numAliveCells)
}

func (e *conway) NextGen() {
	var numAlive uint32 = 0
	neighbours := make(map[uint32]*cellNeighbour)
	for i := 0; i < int(e.numAliveCells); i++ {
		cell := &e.output[i]

		x := cell.x
		xLeft := (x - 1) & e.wrapMask
		xRight := (x + 1) & e.wrapMask

		y := cell.y
		yUp := (y - 1) & e.wrapMask
		yDown := (y + 1) & e.wrapMask

		setNeighbour(cell, xLeft, yUp, neighbours)
		setNeighbour(cell, x, yUp, neighbours)
		setNeighbour(cell, xRight, yUp, neighbours)

		setNeighbour(cell, xLeft, y, neighbours)
		setNeighbour(cell, xRight, y, neighbours)

		setNeighbour(cell, xLeft, yDown, neighbours)
		setNeighbour(cell, x, yDown, neighbours)
		setNeighbour(cell, xRight, yDown, neighbours)
	}

	for i := 0; i < int(e.numAliveCells); i++ {
		cell := &e.output[i]

		coord := makeCoord(cell.x, cell.y)
		n := neighbours[coord]
		if n != nil {
			cnt := n.count
			if cnt == 2 || cnt == 3 {
				n.survive(cell.colour, cell.age)
			}
		}
	}

	for _, n := range neighbours {
		if n.count == 3 {
			if !n.survivor {
				n.create()
			}
			cell := &e.output[numAlive]
			cell.set(n.x, n.y, n.colour, n.age)
			numAlive += 1
		}
	}

	e.numAliveCells = numAlive
}

func (e *conway) Randomise() {
	e.Clear()

	for x := range e.axisLength {
		for y := range e.axisLength {
			if rand.UintN(2) != 1 {
				continue
			}
			colour := rand.Uint32N(0xffffff)
			e.output[e.numAliveCells].set(uint16(x), uint16(y), colour, 0)
			e.numAliveCells += 1
		}
	}
}

func (e *conway) SetCell(x, y uint16, colour uint32, age uint16) {
	ac := &e.output[e.numAliveCells]
	ac.set(x, y, uint32(colour), age)
	e.numAliveCells += 1
}

func makeCoord(x, y uint16) uint32 {
	return (uint32(x) << 16) | uint32(y)
}

func setNeighbour(cell *aliveCell, x, y uint16, neighbours map[uint32]*cellNeighbour) {
	coord := makeCoord(x, y)
	n := neighbours[coord]
	if n != nil {
		n.increment(cell)
		return
	}
	neighbours[coord] = newCellNeighbour(cell, x, y)
}
