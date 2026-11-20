package canvas

import (
	"math"
	"math/bits"
	"unsafe"

	"github.com/JackWithOneEye/conwaymore/internal/protocol"
)

// heatMaxAge is the age from which cells get the last heat colour.
const heatMaxAge = 255

// heatRGBA maps a cell's age to its little-endian RGBA pixel in age mode, on a log scale:
// newborn yellow, then orange, magenta and purple, to indigo at heatMaxAge generations and older.
var heatRGBA = func() (table [heatMaxAge + 1]uint32) {
	stops := [...]uint32{0xffd000, 0xff6a00, 0xe0218a, 0x7b3fe4, 0x3949ab}
	for age := range table {
		pos := math.Log2(1+float64(age)) / math.Log2(heatMaxAge+1) * float64(len(stops)-1)
		i := min(int(pos), len(stops)-2)
		table[age] = rgbToRGBA(mixColour(stops[i], stops[i+1], pos-float64(i)))
	}
	return
}()

// floorDiv divides a by b, rounding towards negative infinity (Go's /
// truncates towards zero, which is wrong for negative offsets here).
func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// floorMod returns the Euclidean remainder of a divided by b, in [0, b) for b > 0.
func floorMod(a, b int) int {
	r := a % b
	if r < 0 {
		r += b
	}
	return r
}

// mixColour blends two 0xRRGGBB colours, from a at t = 0 to b at t = 1.
func mixColour(a, b uint32, t float64) uint32 {
	var out uint32
	for shift := 16; shift >= 0; shift -= 8 {
		ca := float64((a >> shift) & 0xff)
		cb := float64((b >> shift) & 0xff)
		out |= uint32(math.Round(ca+(cb-ca)*t)) << shift
	}
	return out
}

// rgbToRGBA packs 0xRRGGBB into a little-endian RGBA uint32 (R in byte 0, G in byte 1,
// B in byte 2, 0xff in byte 3), matching Canvas ImageData on little-endian targets (WASM).
func rgbToRGBA(c uint32) uint32 {
	return 0xff000000 | (c&0xff)<<16 | (c & 0x00ff00) | (c>>16)&0xff
}

// fillPixels fills dst with val using doubling copy (WASM memory.copy).
func fillPixels(dst []uint32, val uint32) {
	if len(dst) == 0 {
		return
	}
	dst[0] = val
	for n := 1; n < len(dst); n *= 2 {
		copy(dst[n:], dst[:n])
	}
}

// pixelToCell maps a screen pixel coordinate to its wrapped cell coordinate along one axis.
func pixelToCell(px, offset, cellSize int, wrapMask uint16) uint16 {
	return uint16(floorDiv(px-offset, cellSize)) & wrapMask
}

// axisLayout describes how one axis of the cell-resolution buffer maps onto the canvas.
type axisLayout struct {
	full  bool // buffer holds the entire axisLength (tiles repeat with period worldSize)
	start int  // screen pixel start of the first tile (always <= 0)
	count int  // cells drawn per tile from buffer [0, count)
	tile  int  // destination pixels per tile (count * cellSize)
	shift int  // cell index offset when !full: bufIndex = (cellCoord + shift) & wrapMask
}

// computeAxisLayout computes the tiling parameters for one axis.
// When bufLen >= axisLength, the buffer is world-aligned (cell c at index c) and tiles
// repeat with period axisLength*cellSize so panning never invalidates that axis of the buffer.
// Otherwise, the buffer holds only the visible window of cells and a single tile covers canvasLen.
func computeAxisLayout(offset, canvasLen, cellSize, axisLength, bufLen int) axisLayout {
	if bufLen >= axisLength {
		worldSize := axisLength * cellSize
		start := floorMod(offset, worldSize)
		if start > 0 {
			start -= worldSize
		}
		return axisLayout{
			full:  true,
			start: start,
			count: axisLength,
			tile:  worldSize,
			shift: 0,
		}
	}

	q := floorDiv(offset, cellSize)
	rem := offset - q*cellSize
	start := 0
	shift := q
	if rem > 0 {
		start = rem - cellSize
		shift = q + 1
	}
	count := (canvasLen - start + cellSize - 1) / cellSize
	if count > bufLen {
		count = bufLen
	}
	return axisLayout{
		full:  false,
		start: start,
		count: count,
		tile:  count * cellSize,
		shift: shift,
	}
}

// pixelBuffer manages the cell-resolution RGBA backing store with sparse dirty-pixel
// clearing and dirty-row tracking so only changed rows are uploaded to the OffscreenCanvas.
type pixelBuffer struct {
	cols            int
	rows            int
	pixels          []uint32
	bgPixels        []uint32
	bytes           []byte // aliases pixels
	bgRGBA          uint32
	dirtyIdx        []int
	fullClearNeeded bool
}

func (pb *pixelBuffer) resize(cols, rows int, bgRGBA uint32) {
	pb.cols = cols
	pb.rows = rows
	pb.bgRGBA = bgRGBA
	n := cols * rows
	pb.pixels = make([]uint32, n)
	pb.bgPixels = make([]uint32, n)
	fillPixels(pb.bgPixels, bgRGBA)
	if n > 0 {
		pb.bytes = unsafe.Slice((*byte)(unsafe.Pointer(&pb.pixels[0])), n*4)
	} else {
		pb.bytes = nil
	}
	pb.dirtyIdx = pb.dirtyIdx[:0]
	pb.fullClearNeeded = true
}

func (pb *pixelBuffer) setBackground(bgRGBA uint32) {
	if pb.bgRGBA == bgRGBA {
		return
	}
	pb.bgRGBA = bgRGBA
	fillPixels(pb.bgPixels, bgRGBA)
	pb.dirtyIdx = pb.dirtyIdx[:0]
	pb.fullClearNeeded = true
}

// render clears previous cells, plots live cells into pixels, and returns the vertical
// row band [dirtyY, dirtyY+dirtyH) that changed (dirtyH == 0 when nothing changed).
func (pb *pixelBuffer) render(cells []protocol.Cell, axisLength, wrapMask int, xLay, yLay axisLayout, ageMode bool) (dirtyY, dirtyH int) {
	cols, rows := pb.cols, pb.rows
	if cols <= 0 || rows <= 0 {
		return 0, 0
	}

	pixels := pb.pixels
	minY, maxY := rows, -1

	if pb.fullClearNeeded {
		copy(pixels, pb.bgPixels)
		pb.dirtyIdx = pb.dirtyIdx[:0]
		pb.fullClearNeeded = false
		minY, maxY = 0, rows-1
	} else if len(pb.dirtyIdx) > 0 {
		bg := pb.bgRGBA
		if xLay.full && yLay.full {
			axisShift := bits.TrailingZeros(uint(axisLength))
			for _, idx := range pb.dirtyIdx {
				pixels[idx] = bg
				row := idx >> axisShift
				if row < minY {
					minY = row
				}
				if row > maxY {
					maxY = row
				}
			}
		} else {
			for _, idx := range pb.dirtyIdx {
				pixels[idx] = bg
				row := idx / cols
				if row < minY {
					minY = row
				}
				if row > maxY {
					maxY = row
				}
			}
		}
		pb.dirtyIdx = pb.dirtyIdx[:0]
	}

	dirtyLimit := len(pixels) >> 3 // switch to bulk copy above 12.5% fill

	if xLay.full && yLay.full {
		axisShift := bits.TrailingZeros(uint(axisLength))
		if len(cells) > dirtyLimit {
			pb.fullClearNeeded = true
			minY, maxY = 0, rows-1
			if ageMode {
				for i := range cells {
					c := &cells[i]
					idx := (int(c.Y)&wrapMask)<<axisShift | (int(c.X) & wrapMask)
					age := c.Age
					if age > heatMaxAge {
						age = heatMaxAge
					}
					pixels[idx] = heatRGBA[age]
				}
			} else {
				for i := range cells {
					c := &cells[i]
					idx := (int(c.Y)&wrapMask)<<axisShift | (int(c.X) & wrapMask)
					col := c.Colour
					pixels[idx] = 0xff000000 | (col&0xff)<<16 | (col & 0x00ff00) | (col>>16)&0xff
				}
			}
		} else {
			dirty := pb.dirtyIdx
			if ageMode {
				for i := range cells {
					c := &cells[i]
					k := int(c.Y) & wrapMask
					idx := k<<axisShift | (int(c.X) & wrapMask)
					age := c.Age
					if age > heatMaxAge {
						age = heatMaxAge
					}
					pixels[idx] = heatRGBA[age]
					dirty = append(dirty, idx)
					if k < minY {
						minY = k
					}
					if k > maxY {
						maxY = k
					}
				}
			} else {
				for i := range cells {
					c := &cells[i]
					k := int(c.Y) & wrapMask
					idx := k<<axisShift | (int(c.X) & wrapMask)
					col := c.Colour
					pixels[idx] = 0xff000000 | (col&0xff)<<16 | (col & 0x00ff00) | (col>>16)&0xff
					dirty = append(dirty, idx)
					if k < minY {
						minY = k
					}
					if k > maxY {
						maxY = k
					}
				}
			}
			pb.dirtyIdx = dirty
		}
	} else {
		xShift := xLay.shift
		yShift := yLay.shift
		// For small zoomed-in buffers (<= 16K pixels = 64 KB), bulk copy + full upload
		// takes microseconds and avoids per-cell dirty tracking overhead on dense worlds.
		trackDirty := len(pixels) > 16384 && len(cells) <= dirtyLimit
		if !trackDirty {
			pb.fullClearNeeded = true
			minY, maxY = 0, rows-1
			if ageMode {
				for i := range cells {
					c := &cells[i]
					j := (int(c.X) + xShift) & wrapMask
					if j >= cols {
						continue
					}
					k := (int(c.Y) + yShift) & wrapMask
					if k >= rows {
						continue
					}
					age := c.Age
					if age > heatMaxAge {
						age = heatMaxAge
					}
					pixels[k*cols+j] = heatRGBA[age]
				}
			} else {
				for i := range cells {
					c := &cells[i]
					j := (int(c.X) + xShift) & wrapMask
					if j >= cols {
						continue
					}
					k := (int(c.Y) + yShift) & wrapMask
					if k >= rows {
						continue
					}
					col := c.Colour
					pixels[k*cols+j] = 0xff000000 | (col&0xff)<<16 | (col & 0x00ff00) | (col>>16)&0xff
				}
			}
		} else {
			dirty := pb.dirtyIdx
			if ageMode {
				for i := range cells {
					c := &cells[i]
					j := (int(c.X) + xShift) & wrapMask
					if j >= cols {
						continue
					}
					k := (int(c.Y) + yShift) & wrapMask
					if k >= rows {
						continue
					}
					idx := k*cols + j
					age := c.Age
					if age > heatMaxAge {
						age = heatMaxAge
					}
					pixels[idx] = heatRGBA[age]
					dirty = append(dirty, idx)
					if k < minY {
						minY = k
					}
					if k > maxY {
						maxY = k
					}
				}
			} else {
				for i := range cells {
					c := &cells[i]
					j := (int(c.X) + xShift) & wrapMask
					if j >= cols {
						continue
					}
					k := (int(c.Y) + yShift) & wrapMask
					if k >= rows {
						continue
					}
					idx := k*cols + j
					col := c.Colour
					pixels[idx] = 0xff000000 | (col&0xff)<<16 | (col & 0x00ff00) | (col>>16)&0xff
					dirty = append(dirty, idx)
					if k < minY {
						minY = k
					}
					if k > maxY {
						maxY = k
					}
				}
			}
			pb.dirtyIdx = dirty
		}
	}

	if minY > maxY {
		return 0, 0
	}
	return minY, maxY - minY + 1
}
