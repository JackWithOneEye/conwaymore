package engine

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JackWithOneEye/conwaymore/internal/conway"
	"github.com/JackWithOneEye/conwaymore/internal/protocol"
)

type EngineConfig interface {
	conway.ConwayConfig
}

type Engine interface {
	Output() <-chan []byte
	Playing() bool
	Speed() uint32
	Start()
	SubmitMessage(b []byte) error
}

type state = uint32

const (
	paused state = iota
	playing
)

type engine struct {
	ctx          context.Context
	conway       conway.Conway
	speed        atomic.Uint32 // ms
	speedChanged atomic.Bool
	state        atomic.Uint32
	mutex        sync.Mutex
	output       protocol.Output
	outputChan   chan []byte
	encodeBuffer []byte
}

func NewEngine(cfg EngineConfig, seed []byte, ctx context.Context) Engine {
	ws := cfg.WorldSize()
	e := &engine{
		ctx:        ctx,
		conway:     conway.NewConway(cfg),
		output:     protocol.Output{Cells: make([]protocol.Cell, ws/4)},
		outputChan: make(chan []byte, 2),
	}

	e.speed.Store(100)
	err := e.setSeed(seed)
	if err != nil {
		log.Printf("error setting seed: %s", err)
	}

	e.generateOutput()

	return e
}

func (e *engine) Output() <-chan []byte {
	return e.outputChan
}

func (e *engine) Playing() bool {
	return e.state.Load() == playing
}

func (e *engine) Speed() uint32 {
	return e.speed.Load()
}

func (e *engine) Start() {
	ticker := time.NewTicker(e.speedAsDuration())
	defer func() {
		ticker.Stop()
		close(e.outputChan)
	}()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			if e.state.Load() == playing {
				e.calcNextGen()
				e.generateOutput()
			}
			if e.speedChanged.Load() {
				ticker.Reset(e.speedAsDuration())
				e.speedChanged.Store(false)
			}
		}
	}
}

func (e *engine) SubmitMessage(b []byte) error {
	msg, err := protocol.DecodeClientMessage(b)
	if err != nil {
		return fmt.Errorf("decode error: %w", err)
	}

	switch t := msg.(type) {
	case *protocol.Command:
		err = e.handleCommand(t)
	case *protocol.SetCells:
		err = e.handleSetCells(t)
	case *protocol.SetSpeed:
		err = e.handleSetSpeed(t)
	}

	if err != nil {
		return fmt.Errorf("handle command error: %w", err)
	}

	e.generateOutput()

	return nil
}

func (e *engine) calcNextGen() {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.conway.NextGen()
}

func (e *engine) generateOutput() {
	e.mutex.Lock()
	cnt := e.conway.CellsCount()
	if uint(len(e.output.Cells)) < cnt {
		e.output.Cells = make([]protocol.Cell, cnt*2)
	}
	for i, cell := range e.conway.Cells() {
		x, y, colour, age := cell.Values()
		e.output.Cells[i].X = x
		e.output.Cells[i].Y = y
		e.output.Cells[i].Colour = colour
		e.output.Cells[i].Age = age
	}
	e.output.CellsCount = uint32(cnt)
	e.output.Playing = e.state.Load() == playing
	e.output.Speed = uint16(e.speed.Load())

	encodeSize := e.output.EncodeSize()
	if uint32(cap(e.encodeBuffer)) < encodeSize {
		e.encodeBuffer = make([]byte, encodeSize)
	}
	e.encodeBuffer = e.encodeBuffer[:encodeSize]
	e.output.Encode(e.encodeBuffer)
	out := append([]byte(nil), e.encodeBuffer...)
	e.mutex.Unlock()

	select {
	case e.outputChan <- out:
	default:
		log.Println("NOPE")
	}
}

func (e *engine) handleCommand(c *protocol.Command) error {
	switch c.Cmd {
	case protocol.Clear:
		e.mutex.Lock()
		e.conway.Clear()
		e.mutex.Unlock()
	case protocol.Next:
		if e.state.Load() == playing {
			return errors.New("cannot execute next command while playing")
		}
		e.calcNextGen()
	case protocol.Pause:
		if e.state.Load() == paused {
			return errors.New("already paused")
		}
		e.state.Store(paused)
	case protocol.Play:
		if e.state.Load() == playing {
			return errors.New("already playing")
		}
		e.state.Store(playing)
	case protocol.Randomise:
		e.mutex.Lock()
		e.conway.Randomise()
		e.mutex.Unlock()
	}

	return nil
}

func (e *engine) handleSetCells(sc *protocol.SetCells) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	for i := range sc.Cells {
		c := sc.Cells[i]
		if !e.conway.CanSetCell(c.X, c.Y) {
			return fmt.Errorf("cannot set cell at (%d, %d)", c.X, c.Y)
		}
	}
	for i := range sc.Cells {
		c := sc.Cells[i]
		e.conway.SetCell(c.X, c.Y, c.Colour, 0)
	}

	return nil
}

func (e *engine) handleSetSpeed(sp *protocol.SetSpeed) error {
	new := uint32(sp.Speed)
	old := e.speed.Swap(new)
	if new == old {
		return errors.New("speed has not changed")
	}
	e.speedChanged.Store(true)

	return nil
}

func (e *engine) setSeed(seed []byte) error {
	o := &protocol.Output{}
	err := o.Decode(seed)
	if err != nil {
		return err
	}
	if o.Playing {
		e.state.Store(playing)
	} else {
		e.state.Store(paused)
	}
	e.speed.Store(uint32(o.Speed))
	for i := range o.Cells {
		c := o.Cells[i]
		e.conway.SetCell(c.X, c.Y, c.Colour, c.Age)
	}
	return nil
}

func (e *engine) speedAsDuration() time.Duration {
	return time.Duration(e.speed.Load()) * time.Millisecond
}
