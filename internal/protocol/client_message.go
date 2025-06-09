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
)

type ClientMessage interface {
	Encode() []byte
	decode([]byte) error
}

func DecodeClientMessage(b []byte) (ClientMessage, error) {
	var msg ClientMessage
	switch b[0] {
	case byte(command):
		msg = &Command{}
	case byte(setCells):
		msg = &SetCells{}
	case byte(setSpeed):
		msg = &SetSpeed{}
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
	encodeCells(sc.Cells, uint32(sc.Count), b, 3)
	return b
}

func (sc *SetCells) decode(b []byte) error {
	l := len(b)
	if l < 3 {
		return errors.New("[SetCells] too short")
	}

	sc.Count = ((uint16(b[1]) << 8) & 0xff00) | uint16(b[2])

	if l < int(3+sc.Count*bytesPerCell) {
		return errors.New("[SetCells] byte length does not match cells count")
	}

	sc.Cells = make([]Cell, sc.Count)
	decodeCells(b, sc.Cells, 3)
	return nil
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
