package conway

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

type config struct{ worldSize uint }

func (c config) WorldSize() uint { return c.worldSize }

// mixes returns the colours a cell born from parents may get: each parent
// passes on one of red, green, and blue.
func mixes(parents []uint32) []uint32 {
	var out []uint32
	for _, o := range [6][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		out = append(out, parents[o[0]]&0xff0000|parents[o[1]]&0x00ff00|parents[o[2]]&0x0000ff)
	}
	return out
}

func TestNextGenMixesParentColoursAndAgesSurvivors(t *testing.T) {
	c := NewConway(config{worldSize: 8})
	// Vertical blinker; each new cell's parents are the top, middle and bottom cell.
	parents := []uint32{0x123456, 0xabcdef, 0x987654}
	c.SetCell(1, 0, parents[0], 0)
	c.SetCell(1, 1, parents[1], 5)
	c.SetCell(1, 2, parents[2], 0)

	c.NextGen()

	cells := slices.Collect(c.Cells())
	assert.Len(t, cells, 3)
	assert.Equal(t, uint(3), c.CellsCount())
	for _, cell := range cells {
		switch cell.X {
		case 1:
			assert.Equal(t, Cell{X: 1, Y: 1, Colour: 0xabcdef, Age: 6}, cell, "the centre survives and ages")
		default:
			assert.Contains(t, []uint16{0, 2}, cell.X)
			assert.Equal(t, uint16(1), cell.Y)
			assert.Equal(t, uint16(0), cell.Age)
			assert.Contains(t, mixes(parents), cell.Colour)
		}
	}
}

// TestBirthsUseEveryParentOrder checks that the order in which parents pass on
// red, green, and blue is random: over many births, every order occurs.
func TestBirthsUseEveryParentOrder(t *testing.T) {
	c := NewConway(config{worldSize: 8})
	parents := []uint32{0x112233, 0x445566, 0x778899}
	seen := map[uint32]int{}
	for range 600 {
		c.Clear()
		for i, p := range parents {
			c.SetCell(1, uint16(i), p, 0)
		}
		c.NextGen()
		for cell := range c.Cells() {
			if cell.X == 0 {
				seen[cell.Colour]++
			}
		}
	}
	assert.ElementsMatch(t, mixes(parents), slices.Collect(maps.Keys(seen)))
	for colour, n := range seen {
		assert.Greater(t, n, 50, "order for %06x is rare", colour) // about 100 each
	}
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
	born   map[int][]uint32
}

// nextGen computes the next generation. A born cell's colour depends on a
// random order, so it records the colours each may get in born, and the caller
// adopts the implementation's choice once it has checked it.
func (r *reference) nextGen() {
	n := r.n
	r.born = map[int][]uint32{}
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
				r.born[i] = mixes(parents)
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

			got := slices.Collect(c.Cells())
			for _, cell := range got {
				i := int(cell.Y)*n + int(cell.X)
				if options, ok := ref.born[i]; ok {
					if !assert.Contains(t, options, cell.Colour, "world %d, generation %d, cell %v", n, gen, cell) {
						return
					}
					ref.colour[i] = cell.Colour
				}
			}
			var want []Cell
			for i, a := range ref.alive {
				if a {
					want = append(want, Cell{X: uint16(i % n), Y: uint16(i / n), Colour: ref.colour[i], Age: ref.age[i]})
				}
			}
			if !assert.Equal(t, want, got, "world %d, generation %d", n, gen) {
				return
			}
			assert.Equal(t, uint(len(want)), c.CellsCount())
		}
	}
}
