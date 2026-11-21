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

// conway stores which cells are alive as a bitboard: cell (x, y) is bit x&63
// of word y*wordsPerRow + x>>6. Generations are computed 64 cells at a time
// with bitwise adders, skipping words whose neighbourhood is empty. Colours
// and ages are flat arrays indexed by y*axisLength + x.
type conway struct {
	axisLength  int
	axisShift   int
	wrapMask    int
	wordsPerRow int
	rowShift    int // log2(wordsPerRow)
	// A world narrower than 64 cells keeps each row in the low bits of one
	// word. topBit is the row's last bit and rowMask its bits, so that one
	// formula shifts rows of every width (see NextGen).
	topBit  uint
	rowMask uint64

	alive  []uint64
	next   []uint64 // backing array for the next generation
	colour []uint32
	age    []uint16
	count  int
}

func NewConway(cfg ConwayConfig) Conway {
	axisLength := int(cfg.WorldSize())
	wordsPerRow := max(1, axisLength/64)
	rowMask := ^uint64(0)
	if axisLength < 64 {
		rowMask = 1<<axisLength - 1
	}

	return &conway{
		axisLength:  axisLength,
		axisShift:   bits.TrailingZeros(uint(axisLength)),
		wrapMask:    axisLength - 1,
		wordsPerRow: wordsPerRow,
		rowShift:    bits.TrailingZeros(uint(wordsPerRow)),
		topBit:      uint(min(axisLength, 64) - 1),
		rowMask:     rowMask,
		alive:       make([]uint64, axisLength*wordsPerRow),
		next:        make([]uint64, axisLength*wordsPerRow),
		colour:      make([]uint32, axisLength*axisLength),
		age:         make([]uint16, axisLength*axisLength),
	}
}

func (c *conway) CanSetCell(x, y uint16) bool {
	word, bit := c.bitIndex(int(x)&c.wrapMask, int(y)&c.wrapMask)
	return c.alive[word]>>bit&1 == 0
}

func (c *conway) Clear() {
	clear(c.alive)
	c.count = 0
}

// Cells yields the alive cells row by row.
func (c *conway) Cells() iter.Seq[Cell] {
	return func(yield func(Cell) bool) {
		for wi, w := range c.alive {
			for ; w != 0; w &= w - 1 {
				i := c.cellIndex(wi, bits.TrailingZeros64(w))
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
}

func (c *conway) CellsCount() uint {
	return uint(c.count)
}

func (c *conway) NextGen() {
	n, wpr, m := c.axisLength, c.wordsPerRow, c.wrapMask
	top, mask := c.topBit, c.rowMask
	count := 0

	for y := range n {
		up := c.alive[((y-1)&m)*wpr:][:wpr]
		row := c.alive[y*wpr:][:wpr]
		down := c.alive[((y+1)&m)*wpr:][:wpr]
		out := c.next[y*wpr:][:wpr]

		for i := range wpr {
			l, r := i-1, i+1
			if l < 0 {
				l = wpr - 1
			}
			if r == wpr {
				r = 0
			}
			ul, u, ur := up[l], up[i], up[r]
			ml, w, mr := row[l], row[i], row[r]
			dl, d, dr := down[l], down[i], down[r]
			if ul|u|ur|ml|w|mr|dl|d|dr == 0 {
				out[i] = 0
				continue
			}

			// Bit-sliced neighbour count: s0 and s1 are its two low bits, s2
			// is set from four neighbours on. A west neighbour of bit j is bit
			// j-1, so shift left, taking bit 0 from the left word's top bit.
			var s0, s1, s2 uint64
			s0, s1, s2 = add(s0, s1, s2, (u<<1|ul>>top)&mask)
			s0, s1, s2 = add(s0, s1, s2, u)
			s0, s1, s2 = add(s0, s1, s2, (u>>1|ur<<top)&mask)
			s0, s1, s2 = add(s0, s1, s2, (w<<1|ml>>top)&mask)
			s0, s1, s2 = add(s0, s1, s2, (w>>1|mr<<top)&mask)
			s0, s1, s2 = add(s0, s1, s2, (d<<1|dl>>top)&mask)
			s0, s1, s2 = add(s0, s1, s2, d)
			s0, s1, s2 = add(s0, s1, s2, (d>>1|dr<<top)&mask)
			// Alive next with three neighbours, or with two if alive now.
			nw := s1 &^ s2 & (s0 | w)
			out[i] = nw
			count += bits.OnesCount64(nw)

			// Neither loop reads what the other writes: parents are alive now,
			// so they are never cells being born.
			for born := nw &^ w; born != 0; born &= born - 1 {
				x := i<<6 | bits.TrailingZeros64(born)
				idx := y*n + x
				c.colour[idx] = c.parentColour(x, y)
				c.age[idx] = 0
			}
			for survived := nw & w; survived != 0; survived &= survived - 1 {
				idx := y*n + (i<<6 | bits.TrailingZeros64(survived))
				if c.age[idx] < math.MaxUint16 {
					c.age[idx]++
				}
			}
		}
	}

	c.alive, c.next = c.next, c.alive
	c.count = count
}

// add adds the bits of v to the bit-sliced counters s0, s1, s2.
func add(s0, s1, s2, v uint64) (uint64, uint64, uint64) {
	c0 := s0 & v
	s0 ^= v
	c1 := s1 & c0
	s1 ^= c0
	return s0, s1, s2 | c1
}

// parentColour mixes the colours of the three alive neighbours of (x, y), in
// row order: red from the first, green from the second, blue from the third.
func (c *conway) parentColour(x, y int) uint32 {
	m := c.wrapMask
	rows := [3]int{(y - 1) & m, y, (y + 1) & m}
	cols := [3]int{(x - 1) & m, x, (x + 1) & m}
	var parents [3]uint32
	p := 0
	for ri, ny := range rows {
		for ci, nx := range cols {
			if ri == 1 && ci == 1 {
				continue
			}
			word, bit := c.bitIndex(nx, ny)
			if c.alive[word]>>bit&1 == 1 {
				parents[p] = c.colour[ny*c.axisLength+nx]
				p++
				if p == 3 {
					return (parents[0] & 0xff0000) | (parents[1] & 0x00ff00) | (parents[2] & 0x0000ff)
				}
			}
		}
	}
	panic("conway: a cell was born without three parents")
}

func (c *conway) Randomise() {
	c.count = 0
	for wi := range c.alive {
		w := rand.Uint64() & c.rowMask
		c.alive[wi] = w
		c.count += bits.OnesCount64(w)
		for ; w != 0; w &= w - 1 {
			i := c.cellIndex(wi, bits.TrailingZeros64(w))
			c.colour[i] = rand.Uint32N(0xffffff) + 1
			c.age[i] = 0
		}
	}
}

// SetCell sets a cell alive. Coordinates outside the world wrap around.
func (c *conway) SetCell(x, y uint16, colour uint32, age uint16) {
	cx, cy := int(x)&c.wrapMask, int(y)&c.wrapMask
	word, bit := c.bitIndex(cx, cy)
	if c.alive[word]>>bit&1 == 0 {
		c.alive[word] |= 1 << bit
		c.count++
	}
	i := cy*c.axisLength + cx
	c.colour[i] = colour
	c.age[i] = age
}

// bitIndex returns the word and bit of cell (x, y), which must lie in the world.
func (c *conway) bitIndex(x, y int) (word int, bit uint) {
	return y*c.wordsPerRow + x>>6, uint(x & 63)
}

// cellIndex returns the colour and age index of bit bit of word wi.
func (c *conway) cellIndex(wi, bit int) int {
	return (wi>>c.rowShift)<<c.axisShift + (wi&(c.wordsPerRow-1))<<6 + bit
}
