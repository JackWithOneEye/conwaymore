//go:build js

package session

import (
	"syscall/js"

	"github.com/JackWithOneEye/conwaymore/cmd/wasm/jsport"
	"github.com/JackWithOneEye/conwaymore/internal/protocol"
)

// Worker plays a single-player game that runs in the engine worker, over the
// MessagePort that connects the two workers.
type Worker struct {
	port *jsport.Port
	// unacked is set once a message has been returned by Recv; the next Recv
	// acks it first, so the engine worker's flow control counts it as processed.
	unacked bool
}

// NewWorker plays the game on the MessagePort port.
func NewWorker(port js.Value) *Worker {
	// The engine worker sends at most protocol.MaxInFlight messages before an ack.
	return &Worker{port: jsport.New(port, protocol.MaxInFlight)}
}

func (w *Worker) Send(msg protocol.ClientMessage) error {
	w.port.Post(msg.Encode())
	return nil
}

func (w *Worker) Recv() ([]byte, error) {
	if w.unacked {
		w.port.Post((&protocol.FrameAck{}).Encode())
		w.unacked = false
	}
	b := <-w.port.Messages()
	w.unacked = true
	return b, nil
}

func (w *Worker) Close() {
	w.port.Close()
}
