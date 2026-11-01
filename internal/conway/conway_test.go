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
