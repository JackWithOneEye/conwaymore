package local

import (
	"testing"
	"time"

	"github.com/JackWithOneEye/conwaymore/internal/protocol"
	"github.com/stretchr/testify/require"
)

// recvFrame returns the next full frame, skipping state updates.
func recvFrame(t *testing.T, g *Game) protocol.Output {
	t.Helper()
	for {
		b, ok := <-g.Frames()
		require.True(t, ok, "the game ended")
		if protocol.IsStateUpdate(b) {
			continue
		}
		var o protocol.Output
		require.NoError(t, o.Decode(b))
		return o
	}
}

// TestGameRunsFromSeedAndSnapshotsIt runs a saved game, as the engine worker
// does for a single-player game, and checks that its snapshot is the state to
// store: starting a new engine from it continues the same game.
func TestGameRunsFromSeedAndSnapshotsIt(t *testing.T) {
	// A vertical blinker at the world's right edge, so stepping must wrap.
	seed := (&protocol.Output{
		Cells: []protocol.Cell{
			{X: 0, Y: 10, Colour: 0xff0000},
			{X: 0, Y: 11, Colour: 0x00ff00},
			{X: 0, Y: 12, Colour: 0x0000ff},
		},
		CellsCount: 3,
		Speed:      300,
	}).Encode(nil)

	l, err := NewGame(128, seed)
	require.NoError(t, err)
	defer l.Close()

	first := recvFrame(t, l)
	require.Equal(t, uint16(300), first.Speed)
	require.False(t, first.Playing)
	require.Len(t, first.Cells, 3)

	require.NoError(t, l.Submit((&protocol.Command{Cmd: protocol.Next}).Encode()))
	horizontal := recvFrame(t, l)
	cells := map[[2]uint16]protocol.Cell{}
	for _, c := range horizontal.Cells {
		cells[[2]uint16{c.X, c.Y}] = c
	}
	require.Len(t, cells, 3)
	require.Contains(t, cells, [2]uint16{127, 11}, "the blinker wraps across the left edge")
	require.Contains(t, cells, [2]uint16{1, 11})
	require.Equal(t, uint16(1), cells[[2]uint16{0, 11}].Age, "the centre cell survives and ages")
	require.Equal(t, uint32(0xffffff), cells[[2]uint16{1, 11}].Colour, "born cells mix red, green, and blue parents")

	require.NoError(t, l.Submit((&protocol.SetSpeed{Speed: 40}).Encode()))
	b := <-l.Frames()
	require.True(t, protocol.IsStateUpdate(b))

	// The snapshot holds the stepped cells and the new speed.
	snap := l.Snapshot()
	var o protocol.Output
	require.NoError(t, o.Decode(snap))
	require.Equal(t, uint16(40), o.Speed)
	require.ElementsMatch(t, horizontal.Cells, o.Cells)

	// Reloading the snapshot continues the game where it stopped.
	l2, err := NewGame(128, snap)
	require.NoError(t, err)
	defer l2.Close()
	reloaded := recvFrame(t, l2)
	require.Equal(t, uint16(40), reloaded.Speed)
	require.ElementsMatch(t, horizontal.Cells, reloaded.Cells)
}

// TestGamePlaysOnItsOwn checks that a playing game advances without any
// client input, on the engine's own ticker.
func TestGamePlaysOnItsOwn(t *testing.T) {
	seed := (&protocol.Output{
		Cells:      []protocol.Cell{{X: 5, Y: 4, Colour: 1}, {X: 5, Y: 5, Colour: 1}, {X: 5, Y: 6, Colour: 1}},
		CellsCount: 3,
		Playing:    true,
		Speed:      5,
	}).Encode(nil)
	l, err := NewGame(128, seed)
	require.NoError(t, err)
	defer l.Close()

	recvFrame(t, l)
	deadline := time.After(2 * time.Second)
	frames := 0
	for frames < 10 {
		select {
		case <-deadline:
			t.Fatalf("only %d frames in 2s while playing", frames)
		default:
		}
		recvFrame(t, l)
		frames++
	}

	l.Close()
	for range l.Frames() {
		// Frames is closed once the game is closed.
	}
}
