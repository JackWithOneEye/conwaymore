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

func (e *conway) CanSetCell(x, y uint16) bool {
	for i := 0; i < int(e.numAliveCells); i++ {
		cell := &e.output[i]
		if cell.x == x && cell.y == y {
			return false
		}
	}
	return true
}

func (e *conway) Clear() {
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
	candidates := make(map[uint32]*cellCandidate, e.numAliveCells)

	for i := 0; i < int(e.numAliveCells); i++ {
		cell := &e.output[i]

		x := cell.x
		xLeft := (x - 1) & e.wrapMask
		xRight := (x + 1) & e.wrapMask

		y := cell.y
		yUp := (y - 1) & e.wrapMask
		yDown := (y + 1) & e.wrapMask

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

	var numAlive uint32 = 0
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

		cell := &e.output[numAlive]
		cell.set(cand.x, cand.y, cand.colour, cand.age)
		numAlive += 1
	}

	e.numAliveCells = numAlive
}

func (e *conway) Randomise() {
	e.Clear()

	al := uint16(e.axisLength)
	for x := range al {
		for y := range al {
			if rand.UintN(2) != 1 {
				continue
			}
			colour := rand.Uint32N(0xffffff) + 1
			e.output[e.numAliveCells].set(x, y, colour, 0)
			e.numAliveCells += 1
		}
	}
}

func (e *conway) SetCell(x, y uint16, colour uint32, age uint16) {
	ac := &e.output[e.numAliveCells]
	ac.set(x, y, colour, age)
	e.numAliveCells += 1
}

func makeCoord(x, y uint16) uint32 {
	return (uint32(x) << 16) | uint32(y)
}

func setCellAsCandidate(cell *aliveCell, candidates map[uint32]*cellCandidate) {
	coord := makeCoord(cell.x, cell.y)
	cand := candidates[coord]
	if cand != nil {
		cand.colour = cell.colour
		cand.age = cell.age
		cand.alive = true
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
	cand := candidates[coord]
	if cand != nil {
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
