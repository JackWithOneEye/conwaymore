package conway

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

type config struct{ worldSize uint }

func (c config) WorldSize() uint { return c.worldSize }

func TestNextGenMixesParentColoursAndAgesSurvivors(t *testing.T) {
	c := NewConway(config{worldSize: 8})
	// Vertical blinker; each new cell's parents are, in neighbour order, the top, middle and bottom cell.
	c.SetCell(1, 0, 0x123456, 0)
	c.SetCell(1, 1, 0xabcdef, 5)
	c.SetCell(1, 2, 0x987654, 0)

	c.NextGen()

	assert.ElementsMatch(t, []Cell{
		{X: 0, Y: 1, Colour: 0x12cd54, Age: 0},
		{X: 1, Y: 1, Colour: 0xabcdef, Age: 6},
		{X: 2, Y: 1, Colour: 0x12cd54, Age: 0},
	}, slices.Collect(c.Cells()))
	assert.Equal(t, uint(3), c.CellsCount())
}

func TestNextGenWrapsAroundWorldEdges(t *testing.T) {
	c := NewConway(config{worldSize: 8})
	// Glider heading down-right, straddling the bottom-right corner.
	for _, p := range [][2]uint16{{7, 6}, {0, 7}, {6, 0}, {7, 0}, {0, 0}} {
		c.SetCell(p[0], p[1], 1, 0)
	}

	for range 4 {
		c.NextGen()
	}

	var got [][2]uint16
	for cell := range c.Cells() {
		got = append(got, [2]uint16{cell.X, cell.Y})
	}
	assert.ElementsMatch(t, [][2]uint16{{0, 7}, {1, 0}, {7, 1}, {0, 1}, {1, 1}}, got)
	assert.Equal(t, uint(5), c.CellsCount())
}

// reference is a cell-by-cell implementation of the rules, to check the
// bitboard against: it counts each cell's neighbours directly.
type reference struct {
	n      int
	alive  []bool
	colour []uint32
	age    []uint16
}

func (r *reference) nextGen() {
	n := r.n
	alive := make([]bool, n*n)
	for y := range n {
		for x := range n {
			i := y*n + x
			var parents []uint32
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					j := (y+dy+n)%n*n + (x+dx+n)%n
					if (dx != 0 || dy != 0) && r.alive[j] {
						parents = append(parents, r.colour[j])
					}
				}
			}
			switch {
			case r.alive[i] && (len(parents) == 2 || len(parents) == 3):
				alive[i] = true
				r.age[i] = min(r.age[i]+1, 65535)
			case !r.alive[i] && len(parents) == 3:
				alive[i] = true
				r.age[i] = 0
				defer func() { // after every neighbour has read the old colours
					r.colour[i] = parents[0]&0xff0000 | parents[1]&0x00ff00 | parents[2]&0x0000ff
				}()
			}
		}
	}
	r.alive = alive
}

// TestNextGenMatchesReference runs random worlds of every row layout (rows
// within one word, a row of exactly one word, and rows across several words)
// and sparse worlds whose patterns cross word boundaries.
func TestNextGenMatchesReference(t *testing.T) {
	// Sparse worlds whose cells sit next to empty words, which NextGen skips
	// only when their neighbourhood is empty: a blinker on each side of the
	// first word boundary, and a glider crossing the world's edge.
	boundaries := func(c Conway, n int) {
		for _, p := range [][2]int{{63, 9}, {63, 10}, {63, 11}, {64, 30}, {64, 31}, {64, 32}} {
			c.SetCell(uint16(p[0]), uint16(p[1]), uint32(p[1])<<16|uint32(p[0]), 0)
		}
		for _, p := range [][2]int{{n - 1, 50}, {0, 51}, {n - 2, 52}, {n - 1, 52}, {0, 52}} {
			c.SetCell(uint16(p[0]), uint16(p[1]), 0xabcdef^uint32(p[0]), 0)
		}
	}
	type world struct {
		n     int
		setup func(Conway, int)
	}
	var worlds []world
	for _, n := range []int{8, 32, 64, 128, 256} {
		worlds = append(worlds, world{n, func(c Conway, _ int) { c.Randomise() }})
	}
	for _, n := range []int{64, 128, 256} {
		worlds = append(worlds, world{n, boundaries})
	}

	for _, w := range worlds {
		n := w.n
		c := NewConway(config{worldSize: uint(n)})
		w.setup(c, n)
		ref := &reference{n: n, alive: make([]bool, n*n), colour: make([]uint32, n*n), age: make([]uint16, n*n)}
		for cell := range c.Cells() {
			i := int(cell.Y)*n + int(cell.X)
			ref.alive[i], ref.colour[i] = true, cell.Colour
		}

		for gen := range 60 {
			c.NextGen()
			ref.nextGen()

			var want []Cell
			for i, a := range ref.alive {
				if a {
					want = append(want, Cell{X: uint16(i % n), Y: uint16(i / n), Colour: ref.colour[i], Age: ref.age[i]})
				}
			}
			got := slices.Collect(c.Cells())
			if !assert.Equal(t, want, got, "world %d, generation %d", n, gen) {
				return
			}
			assert.Equal(t, uint(len(want)), c.CellsCount())
		}
	}
}
