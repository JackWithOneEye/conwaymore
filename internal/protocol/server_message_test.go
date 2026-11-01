package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateUpdateRoundTrip(t *testing.T) {
	for _, in := range []StateUpdate{
		{Playing: true, Speed: 1},
		{Playing: false, Speed: 1000},
		{Playing: true, Speed: 65535},
	} {
		out, err := DecodeStateUpdate(in.Encode())
		require.NoError(t, err)
		assert.Equal(t, in, *out)
	}
}

func TestStateUpdateIsDistinguishableFromOutput(t *testing.T) {
	for _, playing := range []bool{false, true} {
		o := Output{Playing: playing, Speed: 100, Cells: []Cell{{X: 1, Y: 2, Colour: 3, Age: 4}}, CellsCount: 1}
		assert.False(t, IsStateUpdate(o.Encode(nil)), "output frame (playing=%v)", playing)
	}
	su := StateUpdate{Playing: true, Speed: 42}
	assert.True(t, IsStateUpdate(su.Encode()))
}

func TestStateUpdateDecodeRejectsInvalidInput(t *testing.T) {
	_, err := DecodeStateUpdate([]byte{0, 1, 0, 100})
	assert.Error(t, err, "output frame")

	_, err = DecodeStateUpdate([]byte{StateUpdateMarker, 1})
	assert.Error(t, err, "truncated")

	_, err = DecodeStateUpdate([]byte{StateUpdateMarker, 1, 0, 100, 0})
	assert.Error(t, err, "too long")
}

func TestApplyStateUpdate(t *testing.T) {
	o := Output{Cells: []Cell{{X: 1, Y: 2, Colour: 0x010203, Age: 7}}, CellsCount: 1, Playing: false, Speed: 100}
	frame := o.Encode(nil)
	su := (&StateUpdate{Playing: true, Speed: 42}).Encode()

	patched := ApplyStateUpdate(frame, su)
	require.NotNil(t, patched)

	var out Output
	require.NoError(t, out.Decode(patched))
	assert.True(t, out.Playing)
	assert.Equal(t, uint16(42), out.Speed)
	assert.Equal(t, o.Cells, out.Cells, "cells must survive the header patch")
	assert.Equal(t, frame, o.Encode(nil), "the original frame must not be mutated")

	// A pending state update is superseded wholesale by a newer one.
	assert.Equal(t, su, ApplyStateUpdate((&StateUpdate{Playing: false, Speed: 1}).Encode(), su))

	// Malformed inputs are passed through unchanged.
	assert.Equal(t, frame, ApplyStateUpdate(frame, []byte{1, 2, 3}))
	assert.Equal(t, su, ApplyStateUpdate([]byte{0, 1}, su))
}

func TestFrameAckRoundTrip(t *testing.T) {
	ack := (&FrameAck{}).Encode()
	require.True(t, IsFrameAck(ack))

	msg, err := DecodeClientMessage(ack)
	require.NoError(t, err)
	assert.Equal(t, &FrameAck{}, msg)

	assert.False(t, IsFrameAck((&Command{Cmd: Next}).Encode()))
	assert.False(t, IsFrameAck([]byte{}))
}
