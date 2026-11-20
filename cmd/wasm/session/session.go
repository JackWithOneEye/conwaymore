// Package session connects the canvas worker to the engine that runs its
// game: the server's engine over a WebSocket for multiplayer games (Remote),
// or the engine worker's over a MessagePort for single-player games (Worker).
// Both speak the wire protocol, acks included, so the canvas worker handles
// them the same way.
package session

import "github.com/JackWithOneEye/conwaymore/internal/protocol"

type Session interface {
	// Send submits a client message to the game.
	Send(msg protocol.ClientMessage) error
	// Recv blocks until the game emits its next message, an encoded
	// protocol.Output or protocol.StateUpdate, and returns it. It returns an
	// error once the session has ended.
	Recv() ([]byte, error)
	// Close ends the session.
	Close()
}
