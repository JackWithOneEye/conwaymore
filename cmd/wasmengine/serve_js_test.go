//go:build js

package main

import (
	"syscall/js"
	"testing"
	"time"

	"github.com/JackWithOneEye/conwaymore/cmd/wasm/jsport"
	"github.com/JackWithOneEye/conwaymore/cmd/wasmengine/local"
	"github.com/JackWithOneEye/conwaymore/internal/protocol"
	"github.com/stretchr/testify/require"
)

// TestServeFlowControl plays a game over a real MessageChannel, as the canvas
// worker does, and checks the contract the canvas worker relies on: at most
// protocol.MaxInFlight messages arrive before it acks, each ack releases one
// more, and its commands reach the engine.
func TestServeFlowControl(t *testing.T) {
	// A blinker playing at 1 ms emits about a thousand frames a second.
	seed := (&protocol.Output{
		Cells:      []protocol.Cell{{X: 5, Y: 4, Colour: 1}, {X: 5, Y: 5, Colour: 1}, {X: 5, Y: 6, Colour: 1}},
		CellsCount: 3,
		Playing:    true,
		Speed:      1,
	}).Encode(nil)
	g := local.NewGame(128, seed)
	defer g.Close()

	channel := js.Global().Get("MessageChannel").New()
	go serve(g, jsport.New(channel.Get("port1"), clientBuffer))
	client := jsport.New(channel.Get("port2"), 16)
	defer client.Close()

	// received returns the messages that arrive within d.
	received := func(d time.Duration) [][]byte {
		var msgs [][]byte
		deadline := time.After(d)
		for {
			select {
			case b := <-client.Messages():
				msgs = append(msgs, b)
			case <-deadline:
				return msgs
			}
		}
	}
	ack := func() { client.Post((&protocol.FrameAck{}).Encode()) }

	require.Equal(t, protocol.MaxInFlight, len(received(300*time.Millisecond)), "messages before the first ack")
	ack()
	require.Equal(t, 1, len(received(300*time.Millisecond)), "messages released by one ack")

	// Pause, then ack everything until a message says the game is paused.
	client.Post((&protocol.Command{Cmd: protocol.Pause}).Encode())
	deadline := time.After(2 * time.Second)
	for paused := false; !paused; {
		ack()
		select {
		case b := <-client.Messages():
			playing, _, ok := protocol.PeekHeader(b)
			if protocol.IsStateUpdate(b) {
				su, err := protocol.DecodeStateUpdate(b)
				require.NoError(t, err)
				playing, ok = su.Playing, true
			}
			require.True(t, ok)
			paused = !playing
		case <-deadline:
			t.Fatal("the game did not report Pause")
		}
	}
}
