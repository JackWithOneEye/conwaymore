package conway

import (
	"iter"
	"math"
	"math/bits"
	"math/rand/v2"
)

type ConwayConfig interface {
	// WorldSize is the length of the world's axes. It must be a power of two.
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

// conway stores the world as flat arrays indexed by y*axisLength + x.
type conway struct {
	axisLength int
	axisShift  int
	wrapMask   int

	alive  []uint8 // 1 if alive, so neighbours can be counted by summing
	colour []uint32
	age    []uint16
	count  int

	// candidates are the cells that can change in the next generation: alive cells
	// (they age or die) and the neighbourhoods of cells that were born or died.
	candidates []int32
	queued     []bool  // whether a cell is in candidates
	spare      []int32 // backing array for the next candidates list
	changes    []change
}

// change is a birth or death, applied once the whole generation has been computed.
type change struct {
	idx    int32
	colour uint32
	born   bool
}

func NewConway(cfg ConwayConfig) Conway {
	axisLength := int(cfg.WorldSize())
	cellCount := axisLength * axisLength

	return &conway{
		axisLength: axisLength,
		axisShift:  bits.TrailingZeros(uint(axisLength)),
		wrapMask:   axisLength - 1,
		alive:      make([]uint8, cellCount),
		colour:     make([]uint32, cellCount),
		age:        make([]uint16, cellCount),
		queued:     make([]bool, cellCount),
	}
}

func (c *conway) CanSetCell(x, y uint16) bool {
	return c.alive[c.index(x, y)] == 0
}

func (c *conway) Clear() {
	clear(c.alive)
	c.count = 0
	for _, i := range c.candidates {
		c.queued[i] = false
	}
	c.candidates = c.candidates[:0]
}

// Cells yields the alive cells row by row.
func (c *conway) Cells() iter.Seq[Cell] {
	return func(yield func(Cell) bool) {
		for i, a := range c.alive {
			if a == 0 {
				continue
			}
			cell := Cell{
				X:      uint16(i & c.wrapMask),
				Y:      uint16(i >> c.axisShift),
				Colour: c.colour[i],
				Age:    c.age[i],
			}
			if !yield(cell) {
				return
			}
		}
	}
}

func (c *conway) CellsCount() uint {
	return uint(c.count)
}

func (c *conway) NextGen() {
	current := c.candidates
	for _, i := range current {
		c.queued[i] = false
	}
	c.candidates = c.spare[:0]
	c.changes = c.changes[:0]

	n, m := c.axisLength, c.wrapMask
	for _, ci := range current {
		i := int(ci)
		x, y := i&m, i>>c.axisShift
		xLeft, xRight := (x-1)&m, (x+1)&m
		up, row, down := ((y-1)&m)*n, y*n, ((y+1)&m)*n
		neighbours := [8]int{
			up + xLeft, up + x, up + xRight,
			row + xLeft, row + xRight,
			down + xLeft, down + x, down + xRight,
		}
		var numNeighbours uint8
		for _, j := range neighbours {
			numNeighbours += c.alive[j]
		}

		if c.alive[i] == 1 {
			if numNeighbours == 2 || numNeighbours == 3 {
				// Ages are not read by neighbours, so they can be updated in place.
				if c.age[i] < math.MaxUint16 {
					c.age[i]++
				}
				c.enqueue(ci)
			} else {
				c.changes = append(c.changes, change{idx: ci})
				c.enqueueNeighbourhood(x, y)
			}
		} else if numNeighbours == 3 {
			// Mix the colours of the three parents: red from the first, green from the second, blue from the third.
			var parents [3]uint32
			p := 0
			for _, j := range neighbours {
				if c.alive[j] == 1 {
					parents[p] = c.colour[j]
					p++
				}
			}
			colour := (parents[0] & 0xff0000) | (parents[1] & 0x00ff00) | (parents[2] & 0x0000ff)
			c.changes = append(c.changes, change{idx: ci, colour: colour, born: true})
			c.enqueueNeighbourhood(x, y)
		}
	}

	for _, ch := range c.changes {
		if ch.born {
			c.alive[ch.idx] = 1
			c.colour[ch.idx] = ch.colour
			c.age[ch.idx] = 0
			c.count++
		} else {
			c.alive[ch.idx] = 0
			c.count--
		}
	}
	c.spare = current
}

func (c *conway) Randomise() {
	c.Clear()

	for i := range c.alive {
		if rand.UintN(2) != 1 {
			continue
		}
		c.set(i, rand.Uint32N(0xffffff)+1, 0)
	}
}

// SetCell sets a cell alive. Coordinates outside the world wrap around.
func (c *conway) SetCell(x, y uint16, colour uint32, age uint16) {
	c.set(c.index(x, y), colour, age)
}

func (c *conway) set(i int, colour uint32, age uint16) {
	if c.alive[i] == 0 {
		c.alive[i] = 1
		c.count++
	}
	c.colour[i] = colour
	c.age[i] = age
	c.enqueueNeighbourhood(i&c.wrapMask, i>>c.axisShift)
}

func (c *conway) index(x, y uint16) int {
	return (int(y)&c.wrapMask)*c.axisLength + int(x)&c.wrapMask
}

func (c *conway) enqueue(i int32) {
	if !c.queued[i] {
		c.queued[i] = true
		c.candidates = append(c.candidates, i)
	}
}

func (c *conway) enqueueNeighbourhood(x, y int) {
	m := c.wrapMask
	for dy := -1; dy <= 1; dy++ {
		row := ((y + dy) & m) * c.axisLength
		for dx := -1; dx <= 1; dx++ {
			c.enqueue(int32(row + (x+dx)&m))
		}
	}
}
