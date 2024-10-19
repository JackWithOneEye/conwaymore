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
	Draw(cells []*protocol.Cell)
	IncrementOffset(x, y float64)
	PixelToCellCoord(px, py int) (x, y uint16)
	SetCellSize(cellSize float64)
	SetDimensions(height, width float64)
	SetSettings(age bool, grid bool)
}

const (
	gridLineWidth = 0.5
	strokeStyle   = "#ffffff"
)

type drawMode uint

const (
	drawColour drawMode = iota
	drawAge
)

type canvasDrawer struct {
	axisLength  uint16
	canvas      js.Value // OffscreenCanvas
	cellSize    float64
	cellSizeInv float64
	ctx         js.Value // OffscreenCanvasRenderingContext2D
	wrapMask    uint16

	xOffset float64
	yOffset float64

	xBoundary coordBoundary
	yBoundary coordBoundary
	worldSize float64

	cachedDim canvasDim

	drawMode drawMode
	grid     bool
}

func NewCanvasDrawer(canvas js.Value, axisLength, cellSize int, height, width float64) CanvasDrawer {
	cd := &canvasDrawer{
		axisLength:  uint16(axisLength),
		canvas:      canvas,
		cellSize:    float64(cellSize),
		cellSizeInv: 1 / float64(cellSize),
		ctx:         canvas.Call("getContext", "2d", map[string]any{"alpha": false}),
		wrapMask:    uint16(axisLength) - 1,

		xBoundary: coordBoundary{within: true},
		yBoundary: coordBoundary{within: true},
		worldSize: float64(cellSize * axisLength),

		drawMode: drawColour,
		grid:     true,
	}

	cd.SetDimensions(height, width)

	return cd
}

func (cd *canvasDrawer) Draw(cells []*protocol.Cell) {
	height, width := cd.cachedDim.values()
	cd.ctx.Call("clearRect", 0, 0, width, height)

	if cd.grid {
		cd.drawGrid()
	}

	for i := range cells {
		c := cells[i]
		if cd.coordIsVisible(c.X, c.Y) {
			cd.drawCell(c)
		}
	}
}

func (cd *canvasDrawer) IncrementOffset(x, y float64) {
	ox := cd.xOffset + x
	for math.Abs(ox) >= cd.worldSize {
		sgn := 1.0
		if math.Signbit(ox) {
			sgn = -1.0
		}
		ox = sgn * (math.Abs(ox) - cd.worldSize)
	}
	cd.xOffset = ox

	oy := cd.yOffset + y
	for math.Abs(oy) >= cd.worldSize {
		sgn := 1.0
		if math.Signbit(oy) {
			sgn = -1.0
		}
		oy = sgn * (math.Abs(oy) - cd.worldSize)
	}
	cd.yOffset = oy

	cd.calcVisibleCoordinates()
}

func (cd *canvasDrawer) PixelToCellCoord(px, py int) (x, y uint16) {
	x = uint16(math.Floor((float64(px)-cd.xOffset)*cd.cellSizeInv)) & cd.wrapMask
	y = uint16(math.Floor((float64(py)-cd.yOffset)*cd.cellSizeInv)) & cd.wrapMask
	return
}

func (cd *canvasDrawer) SetCellSize(cellSize float64) {
	cd.cellSize = cellSize
	cd.cellSizeInv = 1 / cellSize
	cd.worldSize = cellSize * float64(cd.axisLength)
	cd.IncrementOffset(0, 0)
}

func (cd *canvasDrawer) SetDimensions(height, width float64) {
	cd.canvas.Set("height", height)
	cd.canvas.Set("width", width)
	cd.cachedDim.set(height, width)
	cd.calcVisibleCoordinates()
}

func (cd *canvasDrawer) SetSettings(age bool, drawGrid bool) {
	if age {
		cd.drawMode = drawAge
	} else {
		cd.drawMode = drawColour
	}

	cd.grid = drawGrid
}

func (cd *canvasDrawer) calcVisibleCoordinates() {
	height, width := cd.cachedDim.values()

	cd.xBoundary.calc(
		width,
		cd.worldSize,
		cd.xOffset,
		cd.cellSizeInv,
		cd.axisLength,
	)
	cd.yBoundary.calc(
		height,
		cd.worldSize,
		cd.yOffset,
		cd.cellSizeInv,
		cd.axisLength,
	)
}

func (cd *canvasDrawer) coordIsVisible(cx, cy uint16) bool {
	if cd.xBoundary.within && (cx < cd.xBoundary.start || cx > cd.xBoundary.end) {
		return false
	}
	if !cd.xBoundary.within && cx < cd.xBoundary.start && cx > cd.xBoundary.end {
		return false
	}

	if cd.yBoundary.within && (cy < cd.yBoundary.start || cy > cd.yBoundary.end) {
		return false
	}
	if !cd.yBoundary.within && cy < cd.yBoundary.start && cy > cd.yBoundary.end {
		return false
	}

	return true
}

func (cd *canvasDrawer) drawCell(cell *protocol.Cell) {
	pxStart := float64(cell.X)*cd.cellSize + cd.xOffset
	pxEnd := pxStart + cd.cellSize

	drawXCount := 1

	drawX1 := 0.0
	drawWidth1 := 0.0

	drawX2 := 0.0
	drawWidth2 := 0.0

	if pxStart < 0 && pxEnd > 0 {
		drawX1 = 0
		drawWidth1 = pxEnd + gridLineWidth
		drawX2 = cd.worldSize + pxStart + gridLineWidth
		drawWidth2 = -pxStart
		drawXCount = 2
	} else if pxStart < 0 && pxEnd <= 0 {
		pxStart += cd.worldSize
	} else if pxStart >= cd.worldSize {
		pxStart -= cd.worldSize
	} else if pxEnd >= cd.worldSize {
		leftWidth := cd.worldSize - pxStart
		drawX1 = pxStart + gridLineWidth
		drawWidth1 = leftWidth - gridLineWidth
		drawX2 = 0
		drawWidth2 = cd.cellSize - leftWidth
		drawXCount = 2
	}

	if drawXCount == 1 {
		drawX1 = pxStart + gridLineWidth
		drawWidth1 = cd.cellSize // - gridLineWidth
	}

	pyStart := float64(cell.Y)*cd.cellSize + cd.yOffset
	pyEnd := pyStart + cd.cellSize

	drawYCount := 1

	drawY1 := 0.0
	drawHeight1 := 0.0

	drawY2 := 0.0
	drawHeight2 := 0.0

	if pyStart < 0 && pyEnd > 0 {
		drawY1 = 0
		drawHeight1 = pyEnd + gridLineWidth
		drawY2 = cd.worldSize + pyStart + gridLineWidth
		drawHeight2 = -pyStart
		drawYCount = 2
	} else if pyStart < 0 && pyEnd <= 0 {
		pyStart += cd.worldSize
	} else if pyStart >= cd.worldSize {
		pyStart -= cd.worldSize
	} else if pyEnd >= cd.worldSize {
		topHeight := cd.worldSize - pyStart
		drawY1 = pyStart + gridLineWidth
		drawHeight1 = topHeight
		drawY2 = 0
		drawHeight2 = cd.cellSize - topHeight
		drawYCount = 2
	}

	if drawYCount == 1 {
		drawY1 = pyStart + gridLineWidth
		drawHeight1 = cd.cellSize // - gridLineWidth
	}

	if cd.drawMode == drawAge {
		// TODO
	} else {
		cd.ctx.Set("fillStyle", fmt.Sprintf("#%.6x", cell.Colour))
	}

	// x1 y1
	cd.ctx.Call("fillRect", drawX1, drawY1, drawWidth1, drawHeight1)

	if drawXCount == 2 {
		// x2 y1
		cd.ctx.Call("fillRect", drawX2, drawY1, drawWidth2, drawHeight1)
	}

	if drawYCount == 2 {
		// x1 y2
		cd.ctx.Call("fillRect", drawX1, drawY2, drawWidth1, drawHeight2)

		if drawXCount == 2 {
			// x2 y2
			cd.ctx.Call("fillRect", drawX2, drawY2, drawWidth2, drawHeight2)
		}
	}
}

func (cd *canvasDrawer) drawGrid() {
	cd.ctx.Call("beginPath")
	height, width := cd.cachedDim.values()
	height = min(height, cd.worldSize)
	width = min(width, cd.worldSize)

	xRem := math.Remainder(cd.xOffset, float64(cd.cellSize))

	x := xRem
	for x <= width {
		cd.ctx.Call("moveTo", x+gridLineWidth, 0)
		cd.ctx.Call("lineTo", x+gridLineWidth, height)
		x += cd.cellSize
	}

	y := math.Remainder(cd.yOffset, float64(cd.cellSize))
	for y <= height {
		cd.ctx.Call("moveTo", 0, y+gridLineWidth)
		cd.ctx.Call("lineTo", width, y+gridLineWidth)
		// xx := xRem
		// for xx < width {
		// 	xx += cd.cellSize
		// }
		y += cd.cellSize
	}

	cd.ctx.Set("strokeStyle", strokeStyle)
	cd.ctx.Set("lineWidth", gridLineWidth)
	cd.ctx.Call("stroke")
}
