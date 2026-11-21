// Package local runs a single-player game's engine in the engine worker.
package local

import (
	"context"
	"sync"

	"github.com/JackWithOneEye/conwaymore/internal/engine"
	"github.com/JackWithOneEye/conwaymore/internal/protocol"
)

type worldSize uint

func (w worldSize) WorldSize() uint { return uint(w) }

// Game runs an engine in this process and keeps its latest state.
type Game struct {
	eng    engine.Engine
	cancel context.CancelFunc

	// frames holds the newest message not yet taken. The pump replaces it with
	// each newer one, so a slow reader skips messages instead of falling
	// behind, as the server does for its subscribers.
	frames chan []byte

	mu   sync.Mutex
	last []byte // the newest full frame, with later state updates applied
}

// NewGame starts an engine for a world of the given size from seed, an
// encoded protocol.Output; an empty seed starts an empty, paused world.
func NewGame(size uint, seed []byte) (*Game, error) {
	ctx, cancel := context.WithCancel(context.Background())
	eng, err := engine.NewEngine(worldSize(size), seed, ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	g := &Game{
		eng:    eng,
		cancel: cancel,
		frames: make(chan []byte, 1),
	}
	go g.pump()
	go g.eng.Start()
	return g, nil
}

// pump moves the engine's messages to frames as soon as they are emitted, so
// that the engine, which drops frames while its output is full, never drops one.
func (g *Game) pump() {
	defer close(g.frames)
	for o := range g.eng.Output() {
		g.mu.Lock()
		if protocol.IsStateUpdate(o) {
			g.last = protocol.ApplyStateUpdate(g.last, o)
		} else {
			g.last = o
		}
		g.mu.Unlock()

		// Only pump sends on frames, so after the drain the send cannot block.
		var pending []byte
		select {
		case pending = <-g.frames:
		default:
		}
		g.frames <- protocol.Coalesce(pending, o)
	}
}

// Submit passes an encoded client message to the engine.
func (g *Game) Submit(b []byte) error {
	return g.eng.SubmitMessage(b)
}

// Frames returns the engine's messages, encoded protocol.Output and
// protocol.StateUpdate values, coalesced while not taken. It is closed once
// the game is closed.
func (g *Game) Frames() <-chan []byte {
	return g.frames
}

// Close stops the engine.
func (g *Game) Close() {
	g.cancel()
}

// Snapshot returns the game's current state as an encoded protocol.Output,
// the form in which the server stores it.
func (g *Game) Snapshot() []byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.last
}
