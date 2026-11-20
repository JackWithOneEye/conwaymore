//go:build js
// +build js

package canvas

import (
	"fmt"
	"math"
	"syscall/js"

	"github.com/JackWithOneEye/conwaymore/internal/protocol"
)

type CanvasDrawer interface {
	Draw(cells []protocol.Cell)
	IncrementOffset(x, y float64)
	MarkCellsDirty()
	PixelToCellCoord(px, py int) (x, y uint16)
	SetCellSize(cellSize, mouseX, mouseY int)
	SetDimensions(height, width int)
	SetSettings(age, grid bool, backgroundColour, gridColour uint32)
	SumCoords(coords ...uint16) uint16
}

const (
	gridLineWidth = 0.5
	gridMinPx     = 3 // hide grid automatically below this size
)

type drawMode uint

const (
	drawColour drawMode = iota
	drawAge
)

var global = js.Global()

type canvasDrawer struct {
	axisLength  uint16
	canvas      js.Value // OffscreenCanvas
	cellSize    int
	cellSizeInv float64
	ctx         js.Value // OffscreenCanvasRenderingContext2D
	wrapMask    uint16

	xOffset float64
	yOffset float64

	worldSize int

	drawMode drawMode
	grid     bool

	backgroundColour uint32
	gridColour       string

	// Cell-resolution rendering: one pixel per visible cell (capped at axisLength)
	// goes into a small OffscreenCanvas, which the browser scales up and tiles
	// with drawImage and imageSmoothingEnabled = false.
	bufCanvas    js.Value // small OffscreenCanvas, bufCols x bufRows
	bufCtx       js.Value
	imageData    js.Value
	imageDataBuf js.Value // cached imageData.data Uint8ClampedArray
	buf          pixelBuffer
	bufCols      int
	bufRows      int

	// Cache invalidation for bufCanvas: when cells and window shifts are unchanged
	// (e.g. panning while zoomed out, or sub-cell panning), skip buffer rebuild and
	// GPU upload and only re-blit the cached OffscreenCanvas at the new tile offsets.
	cellsDirty bool
	bufDirty   bool
	lastXShift int
	lastYShift int

	canvasWidth  int
	canvasHeight int
}

func NewCanvasDrawer(canvas js.Value, axisLength, cellSize, height, width int) CanvasDrawer {
	ctx := canvas.Call("getContext", "2d", map[string]any{"alpha": false})

	cd := &canvasDrawer{
		axisLength:  uint16(axisLength),
		canvas:      canvas,
		cellSize:    cellSize,
		cellSizeInv: 1 / float64(cellSize),
		ctx:         ctx,
		wrapMask:    uint16(axisLength) - 1,

		worldSize: cellSize * axisLength,

		drawMode: drawColour,
		grid:     true,

		backgroundColour: 0xf2f2f2,
		gridColour:       "#d6d6d6",

		cellsDirty: true,
		bufDirty:   true,
	}

	cd.SetDimensions(height, width)

	return cd
}

func (cd *canvasDrawer) MarkCellsDirty() {
	cd.cellsDirty = true
}

func (cd *canvasDrawer) Draw(cells []protocol.Cell) {
	if cd.canvasWidth <= 0 || cd.canvasHeight <= 0 || cd.bufCols <= 0 || cd.bufRows <= 0 {
		return
	}

	xOff := int(cd.xOffset)
	yOff := int(cd.yOffset)

	xLay := computeAxisLayout(xOff, cd.canvasWidth, cd.cellSize, int(cd.axisLength), cd.bufCols)
	yLay := computeAxisLayout(yOff, cd.canvasHeight, cd.cellSize, int(cd.axisLength), cd.bufRows)

	needBuf := cd.cellsDirty || cd.bufDirty ||
		(!xLay.full && xLay.shift != cd.lastXShift) ||
		(!yLay.full && yLay.shift != cd.lastYShift)

	uploadBuf := false
	dirtyY, dirtyH := 0, 0
	if needBuf {
		dirtyY, dirtyH = cd.buf.render(cells, int(cd.axisLength), int(cd.wrapMask), xLay, yLay, cd.drawMode == drawAge)
		cd.cellsDirty = false
		cd.bufDirty = false
		cd.lastXShift = xLay.shift
		cd.lastYShift = yLay.shift

		if dirtyH > 0 {
			rowBytes := cd.bufCols * 4
			start := dirtyY * rowBytes
			end := (dirtyY + dirtyH) * rowBytes
			if dirtyY == 0 && dirtyH == cd.bufRows {
				js.CopyBytesToJS(cd.imageDataBuf, cd.buf.bytes)
			} else {
				js.CopyBytesToJS(cd.imageDataBuf.Call("subarray", start, end), cd.buf.bytes[start:end])
			}
			uploadBuf = true
		}
	}

	showGrid := cd.grid && cd.cellSize >= gridMinPx
	gridX, gridY := 0, 0
	if showGrid {
		gridX = floorMod(xOff, cd.cellSize)
		gridY = floorMod(yOff, cd.cellSize)
	}

	global.Call(
		"renderFrame",
		cd.ctx,
		cd.bufCanvas,
		cd.bufCtx,
		cd.imageData,
		uploadBuf,
		dirtyY,
		cd.bufCols,
		dirtyH,
		xLay.count,
		yLay.count,
		xLay.start,
		yLay.start,
		xLay.tile,
		yLay.tile,
		cd.canvasWidth,
		cd.canvasHeight,
		showGrid,
		gridX,
		gridY,
		cd.cellSize,
		cd.gridColour,
		gridLineWidth,
	)
}

func (cd *canvasDrawer) IncrementOffset(x, y float64) {
	ox := cd.xOffset + x
	for math.Abs(ox) >= float64(cd.worldSize) {
		sgn := 1.0
		if math.Signbit(ox) {
			sgn = -1.0
		}
		ox = sgn * (math.Abs(ox) - float64(cd.worldSize))
	}
	cd.xOffset = ox

	oy := cd.yOffset + y
	for math.Abs(oy) >= float64(cd.worldSize) {
		sgn := 1.0
		if math.Signbit(oy) {
			sgn = -1.0
		}
		oy = sgn * (math.Abs(oy) - float64(cd.worldSize))
	}
	cd.yOffset = oy
}

func (cd *canvasDrawer) PixelToCellCoord(px, py int) (x, y uint16) {
	x = pixelToCell(px, int(cd.xOffset), cd.cellSize, cd.wrapMask)
	y = pixelToCell(py, int(cd.yOffset), cd.cellSize, cd.wrapMask)
	return
}

func (cd *canvasDrawer) SetCellSize(cellSize, mouseX, mouseY int) {
	cellSize = max(cellSize, 1) // Prevent division by zero

	scaling := 1.0 - float64(cellSize)*cd.cellSizeInv

	var newXOffset, newYOffset float64
	if mouseX < 0 || mouseY < 0 {
		// If no mouse position provided, zoom around center
		halfWidth := float64(cd.canvasWidth) * 0.5
		halfHeight := float64(cd.canvasHeight) * 0.5
		newXOffset = (halfWidth - cd.xOffset) * scaling
		newYOffset = (halfHeight - cd.yOffset) * scaling
	} else {
		// Zoom around mouse position
		newXOffset = (float64(mouseX) - cd.xOffset) * scaling
		newYOffset = (float64(mouseY) - cd.yOffset) * scaling
	}

	// Update cell size related properties
	cd.cellSize = cellSize
	cd.cellSizeInv = 1.0 / float64(cellSize)
	cd.worldSize = cellSize * int(cd.axisLength)
	cd.resizeBuffer()

	// Use IncrementOffset to adjust the offsets
	cd.IncrementOffset(newXOffset, newYOffset)
}

func (cd *canvasDrawer) SetDimensions(height, width int) {
	global.Call("setDimensions", cd.canvas, width, height)
	if cd.canvasWidth != width || cd.canvasHeight != height {
		cd.canvasWidth = width
		cd.canvasHeight = height
		cd.resizeBuffer()
	}
}

// resizeBuffer (re)allocates the cell-resolution buffer when the number of
// visible cells changes. It is sized for the worst-case sub-cell offset so
// panning never triggers a reallocation.
func (cd *canvasDrawer) resizeBuffer() {
	if cd.canvasWidth <= 0 || cd.canvasHeight <= 0 {
		return
	}
	cols := min(cd.canvasWidth/cd.cellSize+2, int(cd.axisLength))
	rows := min(cd.canvasHeight/cd.cellSize+2, int(cd.axisLength))
	if cols == cd.bufCols && rows == cd.bufRows {
		return
	}
	cd.bufCols = cols
	cd.bufRows = rows
	cd.bufCanvas = global.Get("OffscreenCanvas").New(cols, rows)
	cd.bufCtx = cd.bufCanvas.Call("getContext", "2d", map[string]any{"alpha": false})
	cd.imageData = cd.bufCtx.Call("createImageData", cols, rows)
	cd.imageDataBuf = cd.imageData.Get("data")
	cd.buf.resize(cols, rows, rgbToRGBA(cd.backgroundColour))
	cd.bufDirty = true
}

func (cd *canvasDrawer) SetSettings(age, drawGrid bool, backgroundColour, gridColour uint32) {
	newMode := drawColour
	if age {
		newMode = drawAge
	}
	if cd.drawMode != newMode {
		cd.drawMode = newMode
		cd.bufDirty = true
	}

	cd.grid = drawGrid
	if cd.backgroundColour != backgroundColour {
		cd.backgroundColour = backgroundColour
		cd.buf.setBackground(rgbToRGBA(backgroundColour))
		cd.bufDirty = true
	}
	cd.gridColour = fmt.Sprintf("#%06x", gridColour&0xffffff)
}

func (cd *canvasDrawer) SumCoords(coords ...uint16) uint16 {
	res := coords[0] & cd.wrapMask
	for i := 1; i < len(coords); i++ {
		res += coords[i]
		res &= cd.wrapMask
	}

	return res
}
