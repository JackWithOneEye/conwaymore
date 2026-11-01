//go:build js
// +build js

package canvas

import (
	"fmt"
	"math"
	"syscall/js"

	"github.com/JackWithOneEye/conwaymore/internal/lrucache"
	"github.com/JackWithOneEye/conwaymore/internal/protocol"
)

type CanvasDrawer interface {
	Draw(cells []protocol.Cell)
	IncrementOffset(x, y float64)
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

// heatMaxAge is the age from which cells get the last heat colour.
const heatMaxAge = 255

// heatColours maps a cell's age to its colour in age mode, on a log scale: newborn yellow,
// then orange, magenta and purple, to indigo at heatMaxAge generations and older.
var heatColours = func() (table [heatMaxAge + 1]uint32) {
	stops := [...]uint32{0xffd000, 0xff6a00, 0xe0218a, 0x7b3fe4, 0x3949ab}
	for age := range table {
		pos := math.Log2(1+float64(age)) / math.Log2(heatMaxAge+1) * float64(len(stops)-1)
		i := min(int(pos), len(stops)-2)
		table[age] = mixColour(stops[i], stops[i+1], pos-float64(i))
	}
	return
}()

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
	backgroundStyle  string // backgroundColour as a CSS colour
	gridColour       string

	// Cell-resolution rendering: one pixel per visible cell goes into a small
	// ImageData, which the browser then scales up with drawImage and
	// imageSmoothingEnabled = false. At 30x zoom the buffer is ~900x smaller
	// than a screen-sized one, and the 8 MB CopyBytesToJS + putImageData per
	// frame shrink to a few KB.
	bufCanvas js.Value // small OffscreenCanvas, bufCols x bufRows
	bufCtx    js.Value
	imageData js.Value
	byteBuffer []byte
	bufCols    int
	bufRows    int

	canvasWidth  int
	canvasHeight int

	// Color cache to avoid repeated fmt.Sprintf
	colorCache lrucache.LruCache[uint32, string]
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
		backgroundStyle:  "#f2f2f2",
		gridColour:       "#d6d6d6",

		colorCache: lrucache.NewLruCache[uint32, string](256),
	}

	cd.SetDimensions(height, width)

	return cd
}

func (cd *canvasDrawer) Draw(cells []protocol.Cell) {
	// Reset the cell-resolution buffer to the background colour to prevent ghost cells
	r := byte(cd.backgroundColour >> 16)
	g := byte(cd.backgroundColour >> 8)
	b := byte(cd.backgroundColour)
	for i := 0; i < len(cd.byteBuffer); i += 4 {
		cd.byteBuffer[i] = r
		cd.byteBuffer[i+1] = g
		cd.byteBuffer[i+2] = b
		cd.byteBuffer[i+3] = 0xff
	}

	// Truncate the offsets to whole pixels like the previous per-pixel renderer
	// did, so the scaled-up result is identical to it. Column j of the buffer
	// holds the cell at (j - 1 - qx) mod axisLength, drawn at screen x
	// j*cellSize + destX; the -1 column covers the partially visible cell at
	// the left/top edge (destX, destY <= 0).
	xOff := int(cd.xOffset)
	yOff := int(cd.yOffset)
	qx := floorDiv(xOff, cd.cellSize)
	qy := floorDiv(yOff, cd.cellSize)
	destX := xOff - (qx+1)*cd.cellSize
	destY := yOff - (qy+1)*cd.cellSize

	cols := min((cd.canvasWidth-destX+cd.cellSize-1)/cd.cellSize, cd.bufCols, int(cd.axisLength))
	rows := min((cd.canvasHeight-destY+cd.cellSize-1)/cd.cellSize, cd.bufRows, int(cd.axisLength))

	// Plot one pixel per visible cell; the modular index handles the toroidal
	// wrap-around and only viewport cells land inside [0, cols) x [0, rows).
	for i := range cells {
		c := &cells[i]
		j := (int(c.X) + 1 + qx) & int(cd.wrapMask)
		k := (int(c.Y) + 1 + qy) & int(cd.wrapMask)
		if j >= cols || k >= rows {
			continue
		}
		colour := c.Colour
		if cd.drawMode == drawAge {
			colour = heatColours[min(c.Age, heatMaxAge)]
		}
		idx := (k*cd.bufCols + j) * 4
		cd.byteBuffer[idx] = byte(colour >> 16)
		cd.byteBuffer[idx+1] = byte(colour >> 8)
		cd.byteBuffer[idx+2] = byte(colour)
	}

	js.CopyBytesToJS(cd.imageData.Get("data"), cd.byteBuffer)
	cd.bufCtx.Call("putImageData", cd.imageData, 0, 0)

	// Clear the canvas, then let the browser scale the cell pixels up.
	cd.ctx.Set("fillStyle", cd.backgroundStyle)
	cd.ctx.Call("fillRect", 0, 0, cd.canvasWidth, cd.canvasHeight)
	cd.ctx.Set("imageSmoothingEnabled", false)
	cd.ctx.Call("drawImage", cd.bufCanvas, 0, 0, cols, rows, destX, destY, cols*cd.cellSize, rows*cd.cellSize)

	// Draw grid after the cells (so it appears on top)
	showGrid := cd.grid && cd.cellSize >= gridMinPx
	if showGrid {
		cd.ctx.Call("beginPath")
		cd.ctx.Set("strokeStyle", cd.gridColour)
		cd.ctx.Set("lineWidth", gridLineWidth)
		cd.drawGrid()
	}
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
	x = uint16(math.Floor((float64(px)-cd.xOffset)*cd.cellSizeInv)) & cd.wrapMask
	y = uint16(math.Floor((float64(py)-cd.yOffset)*cd.cellSizeInv)) & cd.wrapMask
	return
}

func (cd *canvasDrawer) SetCellSize(cellSize, mouseX, mouseY int) {
	cellSize = max(cellSize, 1.0) // Prevent division by zero

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
	cd.byteBuffer = make([]byte, cols*rows*4)
}

func (cd *canvasDrawer) SetSettings(age, drawGrid bool, backgroundColour, gridColour uint32) {
	if age {
		cd.drawMode = drawAge
	} else {
		cd.drawMode = drawColour
	}

	cd.grid = drawGrid
	cd.backgroundColour = backgroundColour
	cd.backgroundStyle = fmt.Sprintf("#%06x", backgroundColour&0xffffff)
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

func (cd *canvasDrawer) drawGrid() {
	if cd.cellSize < gridMinPx {
		return
	}

	height := float64(min(cd.canvasHeight, cd.worldSize))
	width := float64(min(cd.canvasWidth, cd.worldSize))

	// Fix math.Remainder potentially returning negative values
	xRem := math.Mod(cd.xOffset, float64(cd.cellSize))
	if xRem < 0 {
		xRem += float64(cd.cellSize)
	}

	x := xRem
	for x <= width {
		global.Call("vertPath", cd.ctx, x+gridLineWidth/2, height)
		x += float64(cd.cellSize)
	}

	yRem := math.Mod(cd.yOffset, float64(cd.cellSize))
	if yRem < 0 {
		yRem += float64(cd.cellSize)
	}

	y := yRem
	for y <= height {
		global.Call("horizPath", cd.ctx, y+gridLineWidth/2, width)
		y += float64(cd.cellSize)
	}

	cd.ctx.Call("stroke")
}
