package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputRoundTrip(t *testing.T) {
	in := Output{
		Cells: []Cell{
			{X: 3, Y: 0, Colour: 0x123456, Age: 0},
			{X: 4, Y: 0, Colour: 0xffffff, Age: 127},
			{X: 0, Y: 1023, Colour: 0x000001, Age: 65535}, // jump across rows
			{X: 1, Y: 5, Colour: 0xabcdef, Age: 128},      // out of order
		},
		CellsCount: 4,
		Playing:    false,
		Speed:      300,
	}
	encoded := in.Encode(nil)

	// Decode into an output holding a longer, different state to check that it is fully overwritten.
	out := Output{Cells: make([]Cell, 10), CellsCount: 10, Playing: true, Speed: 1}
	require.NoError(t, out.Decode(encoded))
	assert.Equal(t, in, out)
}

func TestOutputDecodeRejectsTruncatedInput(t *testing.T) {
	o := Output{Cells: []Cell{{X: 1, Y: 2, Colour: 0x010203, Age: 300}}, CellsCount: 1}
	encoded := o.Encode(nil)

	for l := cellsOffset; l < len(encoded); l++ {
		assert.Error(t, new(Output).Decode(encoded[:l]), "length %d", l)
	}
}
