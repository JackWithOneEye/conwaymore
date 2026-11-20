//go:build js

// The engine worker runs a single-player game in the browser, on its own
// thread so that a slow generation never holds up drawing. The page starts it
// next to the canvas worker and connects the two with a MessageChannel: the
// canvas worker plays the game over its port with the wire protocol, as it
// plays a multiplayer game over a WebSocket, and the page talks to this worker
// only to start the game and to take snapshots for saving.
package main

import (
	"log"
	"syscall/js"

	"github.com/JackWithOneEye/conwaymore/cmd/wasm/jsport"
	"github.com/JackWithOneEye/conwaymore/cmd/wasmengine/local"
	"github.com/JackWithOneEye/conwaymore/internal/protocol"
)

// Messages from the page (EngineWorkerMessageType in cmd/web/frontend/types/enums.js).
const (
	msgInit = iota
	msgSnapshot
)

// Events to the page (EngineWorkerEventType in cmd/web/frontend/types/enums.js).
const (
	evtReady = iota
	evtSnapshot
)

// clientBuffer is how many of the canvas worker's messages may wait to be
// handled; it sends one ack per frame and a command per user input.
const clientBuffer = 64

// game is set by the init message, before the page asks for snapshots.
var game *local.Game

func main() {
	global := js.Global()
	started := make(chan *jsport.Port, 1)

	onMessage := js.FuncOf(func(this js.Value, args []js.Value) any {
		data := args[0].Get("data")
		switch data.Get("type").Int() {
		case msgInit:
			if game != nil {
				log.Println("engine worker: already initialised")
				return nil
			}
			seed := jsport.FromJS(data.Get("seed"))
			game = local.NewGame(uint(data.Get("worldSize").Int()), seed)
			log.Printf("single-player game started (%d bytes of state)", len(seed))
			started <- jsport.New(data.Get("port"), clientBuffer)
		case msgSnapshot:
			if game == nil {
				log.Println("engine worker: snapshot before init")
				return nil
			}
			seed := jsport.ToJS(game.Snapshot())
			global.Call("postMessage", map[string]any{"type": evtSnapshot, "seed": seed}, []any{seed.Get("buffer")})
		default:
			log.Printf("engine worker: unknown message type %v", data.Get("type"))
		}
		return nil
	})
	defer onMessage.Release()

	global.Call("addEventListener", "message", onMessage)
	global.Call("postMessage", map[string]any{"type": evtReady})

	serve(game, <-started)
}

// serve plays g with the client on port, with the server's flow control: the
// client acks every message, at most protocol.MaxInFlight messages wait for
// an ack, and meanwhile g coalesces newer ones. So the engine never waits for
// the client, and the client never falls behind the engine. serve returns once
// g is closed.
func serve(g *local.Game, port *jsport.Port) {
	credits := make(chan struct{}, protocol.MaxInFlight)
	for range protocol.MaxInFlight {
		credits <- struct{}{}
	}

	go func() {
		for b := range port.Messages() {
			if protocol.IsFrameAck(b) {
				select {
				case credits <- struct{}{}:
				default: // an ack for nothing in flight
				}
				continue
			}
			if err := g.Submit(b); err != nil {
				log.Printf("client message produced an error: %s", err)
			}
		}
	}()

	for {
		<-credits
		b, ok := <-g.Frames()
		if !ok {
			return
		}
		port.Post(b)
	}
}
