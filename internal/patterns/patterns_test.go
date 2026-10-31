package patterns

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRLE(t *testing.T) {
	// 12x4 grid: a multi-digit run, a run-length "$" that skips an empty row,
	// a body wrapped across lines, and rows without trailing dead cells.
	p, err := parseRLE([]byte(`#N Test Pattern
#C a comment
x = 12, y = 4, rule = B3/S23
11bo$2o
2$b
3o!
`))
	require.NoError(t, err)

	assert.Equal(t, "Test Pattern", p.Name)
	assert.Equal(t, []PatternCell{
		{X: 11, Y: 0},
		{X: 0, Y: 1}, {X: 1, Y: 1},
		{X: 1, Y: 3}, {X: 2, Y: 3}, {X: 3, Y: 3},
	}, p.Cells)
	assert.Equal(t, uint16(6), p.CenterX)
	assert.Equal(t, uint16(2), p.CenterY)
}

func TestParseRLERejectsCellsOutsideHeaderBounds(t *testing.T) {
	_, err := parseRLE([]byte("x = 3, y = 1\n4o!\n"))
	assert.Error(t, err)
}
