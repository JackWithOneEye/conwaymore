//go:build js
// +build js

package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"syscall/js"

	"github.com/JackWithOneEye/conwaymore/cmd/wasm/canvas"
	"github.com/JackWithOneEye/conwaymore/cmd/wasm/session"
	"github.com/JackWithOneEye/conwaymore/internal/patterns"
	"github.com/JackWithOneEye/conwaymore/internal/protocol"
	"github.com/coder/websocket"
)

// sessionParams say where the game runs: in the engine worker, reached over
// enginePort, for a single-player game, else on the server at wsPath.
type sessionParams struct {
	enginePort js.Value
	wsPath     string
}

var (
	initialised = false

	// pendingFrame is the newest full frame not yet drawn. It is decoded only
	// when drawn, so frames that arrive faster than the display refreshes cost
	// almost nothing, and each is acked as soon as it arrives.
	pendingFrame []byte
	// frame is reused so that decoding overwrites the previous frame's cells
	// instead of allocating new ones; cellsCache aliases frame.Cells.
	frame      protocol.Output
	cellsCache []protocol.Cell = nil

	initCh = make(chan sessionParams, 1)
	// sess is set once, before any message is sent on it; until then sends fail.
	sess session.Session
)

var (
	cancelAnimationFrame  = js.Global().Get("cancelAnimationFrame")
	requestAnimationFrame = js.Global().Get("requestAnimationFrame")

	drawer canvas.CanvasDrawer = nil

	drawHandle = js.Null()
	drawFunc   = js.FuncOf(func(this js.Value, args []js.Value) any {
		if drawer != nil {
			if pendingFrame != nil {
				if err := frame.Decode(pendingFrame); err != nil {
					log.Printf("could not decode frame: %s", err)
				} else {
					cellsCache = frame.Cells
					drawer.MarkCellsDirty()
				}
				pendingFrame = nil
			}
			drawer.Draw(cellsCache)
			drawHandle = js.Null()
		}
		return js.Undefined()
	})

	onMessageFunc = js.FuncOf(func(this js.Value, args []js.Value) any {
		return onMessage(args[0])
	})
)

const (
	msgInit = iota
	msgCanvasDrag
	msgCellSizeChange
	msgCommand
	msgResize
	msgSetCells
	msgSetPattern
	msgSetSpeed
	msgSettingsChange
)

const (
	evtPlaybackStateChanged = 1
	evtSpeedChanged         = 2
	evtDisconnected         = 3
)

func main() {
	global := js.Global()
	defer func() {
		drawFunc.Release()

		global.Call("removeEventListener", "message", onMessageFunc)
		onMessageFunc.Release()
	}()

	global.Call("addEventListener", "message", onMessageFunc)
	global.Call("postMessage", map[string]any{"type": "ready"})

	postDisconnected := func(err error) {
		global.Call("postMessage", []any{
			map[string]any{
				"type":   evtDisconnected,
				"code":   int(websocket.CloseStatus(err)),
				"reason": err.Error(),
			},
		})
	}

	s, err := openSession(<-initCh)
	if err != nil {
		log.Printf("could not open session: %s", err)
		postDisconnected(err)
		return
	}
	defer s.Close()
	sess = s

	postState := func(playing bool, speed uint16) {
		global.Call(
			"postMessage",
			[]any{
				map[string]any{
					"type":    evtPlaybackStateChanged,
					"playing": playing,
				},
				map[string]any{
					"type":  evtSpeedChanged,
					"speed": speed,
				},
			},
		)
	}

	for {
		b, err := sess.Recv()
		if err != nil {
			log.Printf("session ended (code=%d): %s", websocket.CloseStatus(err), err)
			postDisconnected(err)
			return
		}
		if protocol.IsStateUpdate(b) {
			su, err := protocol.DecodeStateUpdate(b)
			if err != nil {
				log.Printf("could not decode state update: %s", err)
				return
			}
			postState(su.Playing, su.Speed)
		} else {
			playing, speed, ok := protocol.PeekHeader(b)
			if !ok {
				log.Printf("could not decode frame header")
				return
			}
			postState(playing, speed)
			pendingFrame = b
			draw()
		}
	}
}

func openSession(p sessionParams) (session.Session, error) {
	if p.enginePort.Truthy() {
		return session.NewWorker(p.enginePort), nil
	}
	s, err := session.Dial(context.Background(), p.wsPath)
	if err != nil {
		return nil, err
	}
	log.Printf("WS CONN OPEN (%s)", p.wsPath)
	return s, nil
}

func draw() {
	if !drawHandle.IsNull() {
		cancelAnimationFrame.Invoke(drawHandle)
	}
	drawHandle = requestAnimationFrame.Invoke(drawFunc)
}

func handleCanvasDrag(data js.Value) {
	drawer.IncrementOffset(
		data.Get("dx").Float(),
		data.Get("dy").Float(),
	)
}

func handleCellSizeChange(data js.Value) {
	drawer.SetCellSize(
		data.Get("cellSize").Int(),
		data.Get("mouseX").Int(),
		data.Get("mouseY").Int(),
	)
}

func handleCommand(data js.Value) js.Value {
	cmd := data.Get("cmd").Int()
	err := sendClientMessage(&protocol.Command{Cmd: protocol.CommandType(cmd)})
	if err != nil {
		return makeError(fmt.Sprintf("command write failed: %s", err)).Value
	}
	return js.Undefined()
}

func handleInit(data js.Value) js.Value {
	if initialised {
		return makeError("already initialised").Value
	}
	drawer = canvas.NewCanvasDrawer(
		data.Get("canvas"),
		data.Get("worldSize").Int(),
		int(scaleCellSize(data.Get("cellSize").Float())),
		data.Get("height").Int(),
		data.Get("width").Int(),
	)
	initialised = true

	initCh <- sessionParams{
		enginePort: data.Get("enginePort"),
		wsPath:     data.Get("wsPath").String(),
	}
	return js.Undefined()
}

func handleResize(data js.Value) {
	drawer.SetDimensions(
		data.Get("height").Int(),
		data.Get("width").Int(),
	)
}

func handleSetCells(data js.Value) js.Value {
	count := data.Get("count").Int()
	colour := uint32(data.Get("colour").Int())
	originPx := data.Get("originPx").Int()
	originPy := data.Get("originPy").Int()
	cs := make([]byte, count*4)
	cl := js.CopyBytesToGo(cs, data.Get("coordinates"))
	originCx, originCy := drawer.PixelToCellCoord(originPx, originPy)

	if cl != len(cs) {
		log.Printf("setCells: byte length (%d) does not match cells count (%d)", cl, len(cs))
		return makeError("setCells: byte length does not match cells count").Value
	}
	sc := &protocol.SetCells{
		Count: uint16(count),
		Cells: make([]protocol.Cell, count),
	}
	var sci uint
	for i := 0; i < len(cs); i += 4 {
		sc.Cells[sci] = protocol.Cell{
			X:      originCx + ((uint16(cs[i])<<8)&0xff00 | uint16(cs[i+1])&0xff),
			Y:      originCy + ((uint16(cs[i+2])<<8)&0xff00 | uint16(cs[i+3])&0xff),
			Colour: colour,
		}
		sci += 1
	}
	err := sendClientMessage(sc)
	if err != nil {
		return makeError(fmt.Sprintf("setCells write failed: %s", err)).Value
	}
	return js.Undefined()
}

func handleSetPattern(data js.Value) js.Value {
	colour := uint32(data.Get("colour").Int())
	originPx := data.Get("originPx").Int()
	originPy := data.Get("originPy").Int()
	patternType := data.Get("patternType").String()
	pattern, ok := patterns.Patterns[patternType]
	if !ok {
		return makeError(fmt.Sprintf("setPattern: pattern '%s' does not exist", patternType)).Value
	}

	originCx, originCy := drawer.PixelToCellCoord(originPx, originPy)
	count := len(pattern.Cells)

	sc := &protocol.SetCells{
		Count: uint16(count),
		Cells: make([]protocol.Cell, count),
	}
	for i, c := range pattern.Cells {
		sc.Cells[i] = protocol.Cell{
			X:      drawer.SumCoords(originCx-pattern.CenterX, c.X),
			Y:      drawer.SumCoords(originCy-pattern.CenterY, c.Y),
			Colour: colour,
		}
	}
	err := sendClientMessage(sc)
	if err != nil {
		return makeError(fmt.Sprintf("setPattern write failed: %s", err)).Value
	}
	return js.Undefined()
}

func handleSetSpeed(data js.Value) js.Value {
	sp := &protocol.SetSpeed{
		Speed: uint16(data.Get("speed").Int()),
	}
	err := sendClientMessage(sp)
	if err != nil {
		return makeError(fmt.Sprintf("setSpeed write failed: %s", err)).Value
	}
	return js.Undefined()
}

func handleSettingsChange(data js.Value) {
	drawer.SetSettings(
		data.Get("drawAge").Bool(),
		data.Get("drawGrid").Bool(),
		uint32(data.Get("backgroundColour").Int()),
		uint32(data.Get("gridColour").Int()),
	)
}

func onMessage(msgEvt js.Value) js.Value {
	data := msgEvt.Get("data")
	tpe := data.Get("type").Int()

	switch tpe {
	case msgInit:
		return handleInit(data)
	case msgCanvasDrag:
		handleCanvasDrag(data)
	case msgCellSizeChange:
		handleCellSizeChange(data)
	case msgCommand:
		return handleCommand(data)
	case msgResize:
		handleResize(data)
	case msgSetCells:
		return handleSetCells(data)
	case msgSetPattern:
		return handleSetPattern(data)
	case msgSetSpeed:
		return handleSetSpeed(data)
	case msgSettingsChange:
		handleSettingsChange(data)
	default:
		log.Printf("unknown message type: %v", data)
		return makeError(fmt.Sprintf("unknown message type: %v", data)).Value
	}

	draw()

	return js.Undefined()
}

func makeError(msg string) js.Error {
	return js.Error{Value: js.ValueOf(msg)}
}

func scaleCellSize(cellSize float64) float64 {
	return math.Round(max(cellSize, 1.0))
}

func sendClientMessage(msg protocol.ClientMessage) error {
	if sess == nil {
		return fmt.Errorf("not connected yet")
	}
	return sess.Send(msg)
}
