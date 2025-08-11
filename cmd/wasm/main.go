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
	"github.com/JackWithOneEye/conwaymore/internal/patterns"
	"github.com/JackWithOneEye/conwaymore/internal/protocol"
	"github.com/coder/websocket"
)

var (
	initialised = false

	cellsCache []protocol.Cell = nil

	ctx  = context.Background()
	conn *websocket.Conn
)

var (
	cancelAnimationFrame  = js.Global().Get("cancelAnimationFrame")
	requestAnimationFrame = js.Global().Get("requestAnimationFrame")

	drawer canvas.CanvasDrawer = nil

	drawHandle = js.Null()
	drawFunc   = js.FuncOf(func(this js.Value, args []js.Value) any {
		if drawer != nil {
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

func main() {
	global := js.Global()
	defer func() {
		drawFunc.Release()

		global.Call("removeEventListener", "message", onMessageFunc)
		onMessageFunc.Release()
	}()

	var err error

	conn, _, err = websocket.Dial(ctx, "/play", &websocket.DialOptions{})
	if err != nil {
		log.Fatalf("websocket dial failed: %s", err)
	}
	conn.SetReadLimit(33554432) // 2^25
	log.Println("WS CONN OPEN")

	global.Call("addEventListener", "message", onMessageFunc)
	global.Call("postMessage", map[string]any{"type": "ready"})

	for {
		var o protocol.Output
		_, b, err := conn.Read(ctx)
		if err != nil {
			log.Fatalf("could not read from websocket: %s", err)
		}
		err = o.Decode(b)
		if err != nil {
			log.Fatalf("could not decode data: %s", err)
		}

		global.Call(
			"postMessage",
			[]any{
				map[string]any{
					"type":    1,
					"playing": o.Playing,
				},
				map[string]any{
					"type":  2,
					"speed": o.Speed,
				},
			},
		)
		cellsCache = o.Cells
		draw()
	}
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
	drawer.SetSettings(data.Get("drawAge").Bool(), data.Get("drawGrid").Bool())
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
	return conn.Write(ctx, websocket.MessageBinary, msg.Encode())
}
