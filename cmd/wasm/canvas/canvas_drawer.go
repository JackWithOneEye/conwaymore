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
	PixelToCellCoord(px, py int) (x, y uint16)
	SetCellSize(cellSize float64, mouseX, mouseY float64)
	SetDimensions(height, width float64)
	SetSettings(age bool, grid bool)
}

const (
	gridLineWidth = 0.5
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
	ctx := canvas.Call("getContext", "2d", map[string]any{"alpha": false})

	cd := &canvasDrawer{
		axisLength:  uint16(axisLength),
		canvas:      canvas,
		cellSize:    float64(cellSize),
		cellSizeInv: 1 / float64(cellSize),
		ctx:         ctx,
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

func (cd *canvasDrawer) Draw(cells []protocol.Cell) {
	// start := time.Now()
	// defer func() {
	// 	dur := time.Since(start).Microseconds()
	// 	log.Printf("??? %d", dur)
	// }()

	global.Call("prepareCtx", cd.ctx, 0xffffff, gridLineWidth)

	if cd.grid {
		cd.drawGrid()
	}

	for i := range cells {
		c := cells[i]
		if cd.coordIsVisible(c.X, c.Y) {
			cd.drawCell(&c)
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

func (cd *canvasDrawer) SetCellSize(cellSize float64, mouseX, mouseY float64) {
	// Calculate the cell coordinates to zoom around
	var cellX, cellY float64
	if mouseX < 0 || mouseY < 0 {
		// If no mouse position provided, zoom around center
		height, width := cd.cachedDim.values()
		cellX = (width/2 - cd.xOffset) * cd.cellSizeInv
		cellY = (height/2 - cd.yOffset) * cd.cellSizeInv
	} else {
		// Zoom around mouse position
		cellX = (mouseX - cd.xOffset) * cd.cellSizeInv
		cellY = (mouseY - cd.yOffset) * cd.cellSizeInv
	}

	// Update cell size related properties
	cd.cellSize = cellSize
	cd.cellSizeInv = 1 / cellSize
	cd.worldSize = cellSize * float64(cd.axisLength)

	// Calculate new offsets
	var newXOffset, newYOffset float64
	if mouseX < 0 || mouseY < 0 {
		height, width := cd.cachedDim.values()
		newXOffset = width/2 - cellX*cellSize
		newYOffset = height/2 - cellY*cellSize
	} else {
		newXOffset = mouseX - cellX*cellSize
		newYOffset = mouseY - cellY*cellSize
	}

	// Use IncrementOffset to adjust the offsets
	cd.IncrementOffset(newXOffset-cd.xOffset, newYOffset-cd.yOffset)
}

func (cd *canvasDrawer) SetDimensions(height, width float64) {
	global.Call("setDimensions", cd.canvas, width, height)
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
		drawWidth1 = cd.cellSize - gridLineWidth
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
		drawHeight1 = cd.cellSize - gridLineWidth
	}

	if cd.drawMode == drawAge {
		// TODO
	} else {
		cd.ctx.Set("fillStyle", fmt.Sprintf("#%.6x", cell.Colour))
	}

	// x1 y1
	global.Call("strokeAndFillRect", cd.ctx, drawX1, drawY1, drawWidth1, drawHeight1)

	if drawXCount == 2 {
		// x2 y1
		global.Call("strokeAndFillRect", cd.ctx, drawX2, drawY1, drawWidth2, drawHeight1)
	}

	if drawYCount == 2 {
		// x1 y2
		global.Call("strokeAndFillRect", cd.ctx, drawX1, drawY2, drawWidth1, drawHeight2)

		if drawXCount == 2 {
			// x2 y2
			global.Call("strokeAndFillRect", cd.ctx, drawX2, drawY2, drawWidth2, drawHeight2)
		}
	}
}

func (cd *canvasDrawer) drawGrid() {
	height, width := cd.cachedDim.values()
	height = min(height, cd.worldSize)
	width = min(width, cd.worldSize)

	xRem := math.Remainder(cd.xOffset, float64(cd.cellSize))

	x := xRem
	for x <= width {
		global.Call("vertPath", cd.ctx, x+gridLineWidth, height)
		x += cd.cellSize
	}

	y := math.Remainder(cd.yOffset, float64(cd.cellSize))
	for y <= height {
		global.Call("horizPath", cd.ctx, y+gridLineWidth, width)
		y += cd.cellSize
	}

	cd.ctx.Call("stroke")
}
