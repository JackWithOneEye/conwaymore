//go:build js
// +build js

package canvas

import (
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
	SetSettings(age bool, grid bool)
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

	xBoundary coordBoundary
	yBoundary coordBoundary
	worldSize int

	drawMode drawMode
	grid     bool

	// ImageData batch rendering
	imageData    js.Value
	pixelCount   int
	byteBuffer   []byte // Reusable buffer for uint32->byte conversion
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

		xBoundary: coordBoundary{within: true},
		yBoundary: coordBoundary{within: true},
		worldSize: cellSize * axisLength,

		drawMode: drawColour,
		grid:     true,

		colorCache: lrucache.NewLruCache[uint32, string](256),
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

	// Clear canvas with white background
	cd.ctx.Set("fillStyle", "#ffffff")
	cd.ctx.Call("fillRect", 0, 0, cd.canvasWidth, cd.canvasHeight)

	// Clear byte buffer to prevent ghost cells
	for i := range cd.byteBuffer {
		cd.byteBuffer[i] = 0
	}

	// Batch draw cells to pixel buffer
	for i := range cells {
		c := cells[i]
		if cd.coordIsVisible(c.X, c.Y) {
			cd.drawCellToBuffer(&c)
		}
	}

	// Copy pixel buffer to ImageData and draw to canvas
	data := cd.imageData.Get("data")
	js.CopyBytesToJS(data, cd.byteBuffer)
	cd.ctx.Call("putImageData", cd.imageData, 0, 0)

	// Draw grid after ImageData (so it appears on top)
	showGrid := cd.grid && cd.cellSize >= gridMinPx
	if showGrid {
		cd.ctx.Call("beginPath")
		cd.ctx.Set("strokeStyle", "#cccccc") // Light gray grid
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

	cd.calcVisibleCoordinates()
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

	// Use IncrementOffset to adjust the offsets
	cd.IncrementOffset(newXOffset, newYOffset)
}

func (cd *canvasDrawer) SetDimensions(height, width int) {
	global.Call("setDimensions", cd.canvas, width, height)
	cd.calcVisibleCoordinates()

	// Update ImageData for batch rendering
	if cd.canvasWidth != width || cd.canvasHeight != height {
		cd.canvasWidth = width
		cd.canvasHeight = height
		cd.imageData = cd.ctx.Call("createImageData", width, height)
		pixelCount := width * height
		cd.pixelCount = pixelCount
		cd.byteBuffer = make([]byte, pixelCount*4)
	}
}

func (cd *canvasDrawer) SetSettings(age bool, drawGrid bool) {
	if age {
		cd.drawMode = drawAge
	} else {
		cd.drawMode = drawColour
	}

	cd.grid = drawGrid
}

func (cd *canvasDrawer) SumCoords(coords ...uint16) uint16 {
	res := coords[0] & cd.wrapMask
	for i := 1; i < len(coords); i++ {
		res += coords[i]
		res &= cd.wrapMask
	}

	return res
}

func (cd *canvasDrawer) calcVisibleCoordinates() {
	cd.xBoundary.calc(
		cd.canvasWidth,
		cd.worldSize,
		cd.xOffset,
		cd.cellSizeInv,
		cd.axisLength,
	)
	cd.yBoundary.calc(
		cd.canvasHeight,
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

// drawCellToBuffer renders a cell directly to the pixel buffer with proper wrapping
func (cd *canvasDrawer) drawCellToBuffer(cell *protocol.Cell) {
	// Convert cell coordinates to pixel coordinates
	pxStartX := int(cell.X)*cd.cellSize + int(cd.xOffset)
	pxStartY := int(cell.Y)*cd.cellSize + int(cd.yOffset)

	// Handle X wrapping - may need to draw in 1 or 2 locations
	type xPosition struct {
		start, width int
	}
	xPositions := []xPosition{}

	if pxStartX < 0 && pxStartX+cd.cellSize > 0 {
		// Cell straddles left edge
		xPositions = append(xPositions, xPosition{0, pxStartX + cd.cellSize})
		xPositions = append(xPositions, xPosition{cd.worldSize + pxStartX, -pxStartX})
	} else if pxStartX < 0 {
		// Cell is completely off left edge, wrap to right
		xPositions = append(xPositions, xPosition{cd.worldSize + pxStartX, cd.cellSize})
	} else if pxStartX >= cd.worldSize {
		// Cell is completely off right edge, wrap to left
		xPositions = append(xPositions, xPosition{pxStartX - cd.worldSize, cd.cellSize})
	} else if pxStartX+cd.cellSize > cd.worldSize {
		// Cell straddles right edge
		rightWidth := cd.worldSize - pxStartX
		xPositions = append(xPositions, xPosition{pxStartX, rightWidth})
		xPositions = append(xPositions, xPosition{0, cd.cellSize - rightWidth})
	} else {
		// Cell is completely visible, no wrapping needed
		xPositions = append(xPositions, xPosition{pxStartX, cd.cellSize})
	}

	type yPosition struct {
		start, height int
	}
	// Handle Y wrapping - may need to draw in 1 or 2 locations
	yPositions := []yPosition{}

	if pxStartY < 0 && pxStartY+cd.cellSize > 0 {
		// Cell straddles top edge
		yPositions = append(yPositions, yPosition{0, pxStartY + cd.cellSize})
		yPositions = append(yPositions, yPosition{cd.worldSize + pxStartY, -pxStartY})
	} else if pxStartY < 0 {
		// Cell is completely off top edge, wrap to bottom
		yPositions = append(yPositions, yPosition{cd.worldSize + pxStartY, cd.cellSize})
	} else if pxStartY >= cd.worldSize {
		// Cell is completely off bottom edge, wrap to top
		yPositions = append(yPositions, yPosition{pxStartY - cd.worldSize, cd.cellSize})
	} else if pxStartY+cd.cellSize > cd.worldSize {
		// Cell straddles bottom edge
		bottomHeight := cd.worldSize - pxStartY
		yPositions = append(yPositions, yPosition{pxStartY, bottomHeight})
		yPositions = append(yPositions, yPosition{0, cd.cellSize - bottomHeight})
	} else {
		// Cell is completely visible, no wrapping needed
		yPositions = append(yPositions, yPosition{pxStartY, cd.cellSize})
	}

	r := (cell.Colour >> 16) & 0xff
	g := (cell.Colour >> 8) & 0xff
	b := cell.Colour & 0xff

	// Draw all combinations of X and Y positions
	for _, xPos := range xPositions {
		for _, yPos := range yPositions {
			cd.fillRect(xPos.start, yPos.start, xPos.width, yPos.height, r, g, b)
		}
	}
}

// fillRect fills a rectangle in the pixel buffer
func (cd *canvasDrawer) fillRect(startX, startY, width, height int, r, g, b uint32) {
	for y := startY; y < startY+height; y += 1 {
		if y >= cd.canvasHeight {
			continue
		}
		for x := startX; x < startX+width; x += 1 {
			if x >= cd.canvasWidth {
				continue
			}
			idx := y*cd.canvasWidth + x
			if idx < cd.pixelCount {
				i := idx * 4
				cd.byteBuffer[i] = byte(r)   // R
				cd.byteBuffer[i+1] = byte(g) // G
				cd.byteBuffer[i+2] = byte(b) // B
				cd.byteBuffer[i+3] = 0xff    // A
			}
		}
	}
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
