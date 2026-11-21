package engine

import (
	"bytes"
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
	// Observe says whether anyone receives Output. While no one does,
	// generations still run but their frames are not encoded; observing again
	// emits the current frame if one was skipped. Engines start observed.
	Observe(on bool)
	Output() <-chan []byte
	Playing() bool
	// Snapshot returns the current state as an encoded protocol.Output.
	Snapshot() []byte
	Speed() uint32
	Start()
	SubmitMessage(b []byte) error
}

// ErrStopped is returned for messages submitted after the engine stopped.
var ErrStopped = errors.New("engine stopped")

// overrunPause is how long the ticker loop sleeps after a tick whose work took
// longer than the tick period, before it takes the next tick.
const overrunPause = time.Millisecond

// playState is the playback state machine:
//
//	paused  --Play-->  playing
//	playing --Pause--> paused
//
// Play while playing and Pause while paused are rejected. Next is accepted only
// while paused; every other message is accepted in both states. Generations are
// computed on each tick while playing.
type playState uint32

const (
	paused playState = iota
	playing
)

// lifecycle is the engine's lifecycle state machine:
//
//	created --Start--> running --ctx done--> stopped
//
// A created engine has already emitted its initial frame, which the server
// reads before starting it. Entering stopped closes Output; after that, no
// message is emitted and SubmitMessage returns ErrStopped. Start is accepted
// only once.
type lifecycle uint32

const (
	created lifecycle = iota
	running
	stopped
)

type engine struct {
	ctx    context.Context
	conway conway.Conway
	mutex  sync.Mutex // guards conway, output and encodeBuffer

	play atomic.Uint32 // playState
	// unobserved is set while no one receives Output, and stale once a
	// generation has gone unencoded since.
	unobserved atomic.Bool
	stale      atomic.Bool
	lifecycle  atomic.Uint32 // lifecycle

	// speed is the tick period in ms. A change of speed or play state is
	// signalled on timingChanged, a one-slot channel: the ticker loop resets
	// the ticker to the current speed, or stops it while paused, as soon as it
	// takes the signal, so a change applies at once, and a change made while a
	// signal is already pending is picked up with it.
	speed         atomic.Uint32
	timingChanged chan struct{}

	output       protocol.Output
	encodeBuffer []byte

	// outputMu orders emitting against closing outputChan: emitters hold it
	// for reading, and the transition to stopped holds it for writing.
	outputMu   sync.RWMutex
	outputChan chan []byte
}

// NewEngine creates an engine for the world in seed, an encoded protocol.Output;
// an empty seed starts an empty, paused world. A seed that does not decode is
// an error, so that a damaged game is never replaced by an empty one.
func NewEngine(cfg EngineConfig, seed []byte, ctx context.Context) (Engine, error) {
	ws := cfg.WorldSize()
	e := &engine{
		ctx:           ctx,
		conway:        conway.NewConway(cfg),
		output:        protocol.Output{Cells: make([]protocol.Cell, 0, ws/4)},
		outputChan:    make(chan []byte, 2),
		timingChanged: make(chan struct{}, 1),
	}

	e.speed.Store(100)
	if len(seed) > 0 {
		if err := e.setSeed(seed); err != nil {
			return nil, fmt.Errorf("invalid seed: %w", err)
		}
	}

	e.generateOutput()

	return e, nil
}

func (e *engine) Observe(on bool) {
	e.unobserved.Store(!on)
	if on && e.stale.Swap(false) {
		e.generateOutput()
	}
}

func (e *engine) Snapshot() []byte {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.encodeLocked()
}

func (e *engine) Output() <-chan []byte {
	return e.outputChan
}

func (e *engine) Playing() bool {
	return playState(e.play.Load()) == playing
}

func (e *engine) Speed() uint32 {
	return e.speed.Load()
}

func (e *engine) Start() {
	if !e.lifecycle.CompareAndSwap(uint32(created), uint32(running)) {
		log.Println("engine already started")
		return
	}

	ticker := time.NewTicker(e.speedAsDuration())
	resetTicker := func() {
		if e.Playing() {
			ticker.Reset(e.speedAsDuration())
		} else {
			ticker.Stop() // a paused game has nothing to compute
		}
	}
	resetTicker()
	defer func() {
		ticker.Stop()
		e.outputMu.Lock()
		e.lifecycle.Store(uint32(stopped))
		close(e.outputChan)
		e.outputMu.Unlock()
	}()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-e.timingChanged:
			resetTicker()
		case <-ticker.C:
			start := time.Now()
			if e.Playing() {
				e.calcNextGen()
				if e.unobserved.Load() {
					e.stale.Store(true)
				} else {
					e.generateOutput()
				}
			}
			// After an overrun the next tick is already due, so the loop would
			// never block. In a single-threaded runtime such as WASM, where
			// nothing preempts a goroutine that never blocks, that starves every
			// other goroutine and the browser's event loop, so sleep briefly.
			if time.Since(start) >= e.speedAsDuration() {
				time.Sleep(overrunPause)
			}
		}
	}
}

func (e *engine) SubmitMessage(b []byte) error {
	if lifecycle(e.lifecycle.Load()) == stopped {
		return ErrStopped
	}

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
	case *protocol.FrameAck:
		// Acks are handled by the server; ignore one should it arrive here.
		return nil
	}

	if err != nil {
		return fmt.Errorf("handle command error: %w", err)
	}

	// A speed change does not alter any cell, so a lightweight state update is
	// enough; every other successful command can change cells and needs a frame.
	if _, ok := msg.(*protocol.SetSpeed); ok {
		e.generateStateUpdate()
	} else {
		e.generateOutput()
	}

	return nil
}

func (e *engine) calcNextGen() {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.conway.NextGen()
}

func (e *engine) generateOutput() {
	e.mutex.Lock()
	out := e.encodeLocked()
	e.mutex.Unlock()

	e.emit(out, false)
}

// encodeLocked encodes the current state into a new slice. e.mutex must be held.
func (e *engine) encodeLocked() []byte {
	e.output.Cells = e.output.Cells[:0]
	for c := range e.conway.Cells() {
		e.output.Cells = append(e.output.Cells, protocol.Cell{X: c.X, Y: c.Y, Colour: c.Colour, Age: c.Age})
	}
	e.output.CellsCount = uint32(len(e.output.Cells))
	e.output.Playing = e.Playing()
	e.output.Speed = uint16(e.speed.Load())

	// The encoded size is only known after encoding, so encode into a reused buffer.
	// Every output gets its own exact-size copy because it is shared with all listeners.
	e.encodeBuffer = e.output.Encode(e.encodeBuffer[:0])
	return bytes.Clone(e.encodeBuffer)
}

// generateStateUpdate broadcasts only the play state and speed, for changes
// that leave every cell untouched.
func (e *engine) generateStateUpdate() {
	su := &protocol.StateUpdate{
		Playing: e.Playing(),
		Speed:   uint16(e.speed.Load()),
	}

	// Unlike a full frame, a state update must not be dropped: in a paused game
	// no later message would ever deliver the new state. The hub drains the
	// channel immediately, so this never blocks for long.
	e.emit(su.Encode(), true)
}

// emit sends b on Output unless the engine has stopped. When the channel is
// full, it drops b, or with wait set, waits until there is room or the engine's
// context is done.
func (e *engine) emit(b []byte, wait bool) {
	e.outputMu.RLock()
	defer e.outputMu.RUnlock()
	if lifecycle(e.lifecycle.Load()) == stopped {
		return
	}

	if wait {
		select {
		case e.outputChan <- b:
		case <-e.ctx.Done():
		}
		return
	}
	select {
	case e.outputChan <- b:
	default: // the consumer still holds older frames; this one is superseded by the next
	}
}

func (e *engine) handleCommand(c *protocol.Command) error {
	switch c.Cmd {
	case protocol.Clear:
		e.mutex.Lock()
		e.conway.Clear()
		e.mutex.Unlock()
	case protocol.Next:
		if e.Playing() {
			return errors.New("cannot execute next command while playing")
		}
		e.calcNextGen()
	case protocol.Pause:
		if !e.play.CompareAndSwap(uint32(playing), uint32(paused)) {
			return errors.New("already paused")
		}
		e.signalTiming()
	case protocol.Play:
		if !e.play.CompareAndSwap(uint32(paused), uint32(playing)) {
			return errors.New("already playing")
		}
		e.signalTiming()
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
	if sp.Speed == 0 {
		return errors.New("speed must be at least 1 ms")
	}
	new := uint32(sp.Speed)
	old := e.speed.Swap(new)
	if new == old {
		return errors.New("speed has not changed")
	}
	e.signalTiming()
	return nil
}

// signalTiming tells the ticker loop that the speed or play state changed.
func (e *engine) signalTiming() {
	select {
	case e.timingChanged <- struct{}{}:
	default: // a change is already signalled; the ticker loop reads the newest state
	}
}

func (e *engine) setSeed(seed []byte) error {
	o := &protocol.Output{}
	err := o.Decode(seed)
	if err != nil {
		return err
	}
	if o.Playing {
		e.play.Store(uint32(playing))
	} else {
		e.play.Store(uint32(paused))
	}
	if o.Speed > 0 {
		e.speed.Store(uint32(o.Speed))
	}
	for i := range o.Cells {
		c := o.Cells[i]
		e.conway.SetCell(c.X, c.Y, c.Colour, c.Age)
	}
	return nil
}

func (e *engine) speedAsDuration() time.Duration {
	return time.Duration(e.speed.Load()) * time.Millisecond
}
