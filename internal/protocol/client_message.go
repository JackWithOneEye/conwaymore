package protocol

import (
	"errors"
	"fmt"
)

type clientMessageType uint8

const (
	command clientMessageType = iota
	setCells
	setSpeed
	frameAck
)

type ClientMessage interface {
	Encode() []byte
	decode([]byte) error
}

func DecodeClientMessage(b []byte) (ClientMessage, error) {
	if len(b) == 0 {
		return nil, errors.New("empty client message")
	}
	var msg ClientMessage
	switch b[0] {
	case byte(command):
		msg = &Command{}
	case byte(setCells):
		msg = &SetCells{}
	case byte(setSpeed):
		msg = &SetSpeed{}
	case byte(frameAck):
		msg = &FrameAck{}
	default:
		return nil, fmt.Errorf("unknown client message type: %d", b[0])
	}
	err := msg.decode(b)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

type CommandType uint8

const (
	Next CommandType = iota
	Play
	Pause
	Clear
	Randomise
)

type Command struct {
	Cmd CommandType
}

func (c *Command) Encode() []byte {
	b := make([]byte, 2)
	b[0] = byte(command)
	b[1] = byte(c.Cmd)
	return b
}

func (c *Command) decode(b []byte) error {
	if len(b) < 2 {
		return errors.New("[Command] too short")
	}
	c.Cmd = CommandType(b[1])
	return nil
}

type SetCells struct {
	Count uint16
	Cells []Cell
}

func (sc *SetCells) Encode() []byte {
	b := make([]byte, 3+len(sc.Cells)*bytesPerCell)
	b[0] = byte(setCells)
	b[1] = byte((sc.Count >> 8) & 0xff)
	b[2] = byte(sc.Count & 0xff)
	encodeCells(sc.Cells[:sc.Count], b[3:])
	return b
}

func (sc *SetCells) decode(b []byte) error {
	l := len(b)
	if l < 3 {
		return errors.New("[SetCells] too short")
	}

	sc.Count = ((uint16(b[1]) << 8) & 0xff00) | uint16(b[2])

	if l < 3+int(sc.Count)*bytesPerCell {
		return errors.New("[SetCells] byte length does not match cells count")
	}

	sc.Cells = make([]Cell, sc.Count)
	decodeCells(b[3:], sc.Cells)
	return nil
}

// FrameAck confirms receipt of a single server message. Clients send one ack
// per received message so the server can keep at most MaxInFlight frames in
// flight per client instead of queueing frames that may be stale on arrival.
type FrameAck struct{}

// MaxInFlight caps how many messages a game may send a client before the
// client acks the oldest of them. A sender keeps the newest message it cannot
// send yet and replaces it with each newer one (see Coalesce), so a slow
// client falls behind by at most MaxInFlight messages plus that one.
const MaxInFlight = 2

// Window applies the frame-ack flow control to one client's delivery loop:
// at most MaxInFlight sent messages wait for an ack. The zero value is ready.
type Window struct {
	inFlight int
}

// Gate returns frames while another message may be sent and nil otherwise,
// so that a select receiving from it waits for an ack instead.
func (w *Window) Gate(frames <-chan []byte) <-chan []byte {
	if w.inFlight < MaxInFlight {
		return frames
	}
	return nil
}

// Sent records a message sent to the client.
func (w *Window) Sent() {
	w.inFlight++
}

// Acked records an ack from the client; an ack for nothing in flight is ignored.
func (w *Window) Acked() {
	if w.inFlight > 0 {
		w.inFlight--
	}
}

func (f *FrameAck) Encode() []byte {
	return []byte{byte(frameAck)}
}

func (f *FrameAck) decode(b []byte) error {
	if len(b) != 1 {
		return errors.New("[FrameAck] too long")
	}
	return nil
}

// IsFrameAck reports whether b is a frame acknowledgement.
func IsFrameAck(b []byte) bool {
	return len(b) == 1 && b[0] == byte(frameAck)
}

type SetSpeed struct {
	Speed uint16
}

func (sp *SetSpeed) Encode() []byte {
	b := make([]byte, 3)
	b[0] = byte(setSpeed)
	b[1] = byte((sp.Speed >> 8) & 0xff)
	b[2] = byte(sp.Speed & 0xff)
	return b
}

func (sp *SetSpeed) decode(b []byte) error {
	if len(b) < 3 {
		return errors.New("[SetSpeed] too short")
	}
	sp.Speed = ((uint16(b[1]) << 8) & 0xff00) | uint16(b[2])
	return nil
}
