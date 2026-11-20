package session

import (
	"context"

	"github.com/JackWithOneEye/conwaymore/internal/protocol"
	"github.com/coder/websocket"
)

// readLimit is the largest server message accepted, 2^25 bytes.
const readLimit = 33554432

// Remote plays a game that runs on the server.
type Remote struct {
	ctx  context.Context
	conn *websocket.Conn
	// unacked is set once a message has been returned by Recv; the next Recv
	// acks it first, so the server's flow control counts it as processed.
	unacked bool
}

// Dial opens a WebSocket to the game's play endpoint.
func Dial(ctx context.Context, url string) (*Remote, error) {
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{})
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(readLimit)
	return &Remote{ctx: ctx, conn: conn}, nil
}

func (r *Remote) Send(msg protocol.ClientMessage) error {
	return r.conn.Write(r.ctx, websocket.MessageBinary, msg.Encode())
}

func (r *Remote) Recv() ([]byte, error) {
	if r.unacked {
		if err := r.Send(&protocol.FrameAck{}); err != nil {
			return nil, err
		}
		r.unacked = false
	}
	_, b, err := r.conn.Read(r.ctx)
	if err != nil {
		return nil, err
	}
	r.unacked = true
	return b, nil
}

func (r *Remote) Close() {
	r.conn.CloseNow()
}
