package engine

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JackWithOneEye/conwaymore/internal/conway"
)

const (
	stateIdx      = 0
	speedIdx      = 1
	cellsCountIdx = 3
	cellsIdx      = 6
)

type EngineConfig interface {
	conway.ConwayConfig
}

type Engine interface {
	Output() <-chan []byte
	Playing() bool
	Speed() uint32
	Start()
	SubmitMessage(msg []byte) error
}

type state = uint32

const (
	paused state = iota
	playing
)

const bytesPerCell = 7

type engine struct {
	conway       conway.Conway
	speed        atomic.Uint32 // ms
	speedChanged atomic.Bool
	state        atomic.Uint32
	mutex        sync.RWMutex
	outputChan   chan []byte
}

/**
 * STATE|(SPEED|SPEED)|(X|X|Y|Y|R|G|B)|(...)|(...)|...
 */

func NewEngine(cfg EngineConfig, seed []byte) Engine {
	e := &engine{
		conway:     conway.NewConway(cfg),
		outputChan: make(chan []byte, 1),
	}

	e.speed.Store(100)
	e.setSeed(seed)

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
	ticker := time.NewTicker(time.Duration(e.speed.Load()) * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		if e.state.Load() == playing {
			e.mutex.Lock()
			e.conway.NextGen()
			e.mutex.Unlock()
			e.generateOutput()
		}
		if e.speedChanged.Load() {
			ticker.Reset(time.Duration(e.speed.Load()) * time.Millisecond)
			e.speedChanged.Store(false)
		}
	}
}

func (e *engine) SubmitMessage(msg []byte) error {
	msgLen := len(msg)
	if msgLen == 0 {
		return errors.New("empty message")
	}

	mType := uint(msg[msgType])

	gameChanged := false

	if mType&command == command {
		if msgLen < 2 {
			return errors.New("command value missing")
		}
		switch uint(msg[cmd]) {
		case next:
			if e.state.Load() == paused {
				e.mutex.Lock()
				e.conway.NextGen()
				e.mutex.Unlock()
			}
		case play:
			e.state.CompareAndSwap(paused, playing)
		case pause:
			e.state.CompareAndSwap(playing, paused)
		case clear:
			e.mutex.Lock()
			e.conway.Clear()
			e.mutex.Unlock()
		case randomise:
			e.mutex.Lock()
			e.conway.Randomise()
			e.mutex.Unlock()
		}
		gameChanged = true
	}

	if mType&setSpeed == setSpeed {
		if msgLen < 4 {
			return errors.New("speed value missing")
		}
		changed := e.setSpeed(msg[speed:])
		e.speedChanged.Store(changed)
		gameChanged = true
	}

	if mType&setCells == setCells {
		if msgLen < 5 {
			return errors.New("cells count missing")
		}
		count := uint(msg[cellsCount])
		lenCells := bytesPerCell * count
		if msgLen < int(lenCells) {
			return errors.New("cells missing")
		}
		func() {
			e.mutex.Lock()
			defer e.mutex.Unlock()

			type validCell struct {
				x, y uint16
				c    uint32
			}
			validCells := make([]validCell, count)
			vidx := 0
			for i := cells; i < cells+lenCells; i += bytesPerCell {
				x, y, colour := bytesToCellValues(msg, i)
				if !e.conway.CanSetCell(x, y) {
					return
				}
				validCells[vidx] = validCell{x, y, colour}
				vidx += 1
			}
			for i := range validCells {
				vc := validCells[i]
				e.conway.SetCell(vc.x, vc.y, vc.c)
			}
			gameChanged = true
		}()
	}

	if gameChanged {
		e.generateOutput()
	}

	return nil
}

func bytesToCellValues(data []byte, index uint) (x, y uint16, colour uint32) {
	x = ((uint16(data[index]) << 8) & 0xff00) | (uint16(data[index+1]) & 0x00ff)
	y = ((uint16(data[index+2]) << 8) & 0xff00) | (uint16(data[index+3]) & 0x00ff)
	colour = ((uint32(data[index+4]) << 16) & 0xff0000) |
		((uint32(data[index+5]) << 8) & 0x00ff00) |
		(uint32(data[index+6]) & 0x0000ff)

	return
}

func (e *engine) generateOutput() {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	cellsCount := e.conway.CellsCount()
	output := make([]byte, 1+2+3+cellsCount*bytesPerCell)

	output[stateIdx] = byte(e.state.Load())

	speed := e.speed.Load()
	output[speedIdx] = byte(speed >> 8)
	output[speedIdx+1] = byte(speed & 0x00ff)

	output[cellsCountIdx] = byte((cellsCount & 0x00ff0000) >> 16)
	output[cellsCountIdx+1] = byte((cellsCount & 0x0000ff00) >> 8)
	output[cellsCountIdx+2] = byte(cellsCount & 0x000000ff)

	i := cellsIdx
	for cell := range e.conway.Cells() {
		x, y, colour := cell.Values()
		output[i] = byte(x >> 8)
		i += 1
		output[i] = byte(x)
		i += 1
		output[i] = byte(y >> 8)
		i += 1
		output[i] = byte(y)
		i += 1

		output[i] = byte(colour >> 16)
		i += 1
		output[i] = byte(colour >> 8)
		i += 1
		output[i] = byte(colour)
		i += 1
	}

	e.outputChan <- output
}

func (e *engine) setSeed(seed []byte) {
	if len(seed) < cellsIdx {
		return
	}
	e.state.Store(uint32(seed[stateIdx]))
	_ = e.setSpeed(seed[speedIdx : speedIdx+2])
	cellsCount := (uint(seed[cellsCountIdx])<<16)&0xff0000 | (uint(seed[cellsCountIdx+1])<<8)&0xff00 | uint(seed[cellsCountIdx+2])&0xff
	var i uint = cellsIdx
	for ; i < (cellsIdx + cellsCount*bytesPerCell); i += bytesPerCell {
		x, y, colour := bytesToCellValues(seed, i)
		e.conway.SetCell(x, y, colour)
	}
}

func (e *engine) setSpeed(data []byte) (changed bool) {
	newSpeed := max(1, (uint32(data[0])<<8)|uint32(data[1]))
	old := e.speed.Swap(newSpeed)
	return newSpeed != old
}
