package engine

import (
	"context"
	"testing"
	"time"

	"github.com/JackWithOneEye/conwaymore/internal/protocol"
	"github.com/stretchr/testify/require"
)

type worldSize uint

func (w worldSize) WorldSize() uint { return uint(w) }

// TestPauseStopsGenerationsAndPlayResumesThem drives the play state machine
// from a paused start: no generation runs until Play, none after Pause, and
// Play resumes them.
func TestPauseStopsGenerationsAndPlayResumesThem(t *testing.T) {
	// A blinker at 1 ms; its centre cell ages by one each generation.
	seed := (&protocol.Output{
		Cells:      []protocol.Cell{{X: 5, Y: 4, Colour: 1}, {X: 5, Y: 5, Colour: 1}, {X: 5, Y: 6, Colour: 1}},
		CellsCount: 3,
		Speed:      1,
	}).Encode(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e, err := NewEngine(worldSize(128), seed, ctx)
	require.NoError(t, err)
	<-e.Output() // the initial frame
	go e.Start()

	centreAge := func() uint16 {
		var o protocol.Output
		require.NoError(t, o.Decode(e.Snapshot()))
		for _, c := range o.Cells {
			if c.X == 5 && c.Y == 5 {
				return c.Age
			}
		}
		t.Fatal("the centre cell died")
		return 0
	}
	go func() { // drain frames as a room's broadcast loop does
		for range e.Output() {
		}
	}()
	command := func(cmd protocol.CommandType) {
		require.NoError(t, e.SubmitMessage((&protocol.Command{Cmd: cmd}).Encode()))
	}

	time.Sleep(30 * time.Millisecond)
	require.Equal(t, uint16(0), centreAge(), "a paused engine computed generations")

	command(protocol.Play)
	require.Eventually(t, func() bool { return centreAge() > 5 }, 2*time.Second, time.Millisecond)

	command(protocol.Pause)
	time.Sleep(10 * time.Millisecond) // a tick taken before the pause may still finish
	paused := centreAge()
	time.Sleep(30 * time.Millisecond)
	require.Equal(t, paused, centreAge(), "generations ran while paused")

	command(protocol.Play)
	require.Eventually(t, func() bool { return centreAge() > paused+5 }, 2*time.Second, time.Millisecond)
}
