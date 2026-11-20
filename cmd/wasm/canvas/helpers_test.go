package canvas

import (
	"testing"

	"github.com/JackWithOneEye/conwaymore/internal/protocol"
	"github.com/stretchr/testify/require"
)

func TestAxisLayoutCoversScreenExactlyAndMatchesPixelToCell(t *testing.T) {
	const axisLength = 1024
	const wrapMask = uint16(axisLength - 1)

	cases := []struct {
		name      string
		canvasLen int
		cellSize  int
		offsets   []int
	}{
		{"wide 2560 at 1x", 2560, 1, []int{0, 1, -1, 500, -500, 1023, -1023}},
		{"exact boundary 1024 at 1x", 1024, 1, []int{0, 1, -1, 512, -512}},
		{"just under world 1023 at 1x", 1023, 1, []int{0, 1, -1, 500, -500}},
		{"ultrawide 3440 at 2x", 3440, 2, []int{0, 1, -1, 2, -2, 1001, -1001}},
		{"ultrawide 3440 at 3x", 3440, 3, []int{0, 1, 2, -1, -2, 7, -7, 3071, -3071}},
		{"exact boundary 2048 at 2x", 2048, 2, []int{0, 1, -1, 2047, -2047}},
		{"zoomed in 1280 at 20x", 1280, 20, []int{0, 1, 19, 20, 25, -1, -19, -20, -25, 20479, -20479}},
		{"max zoom 1280 at 100x", 1280, 100, []int{0, 1, 50, 99, -1, -50, -99}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bufLen := min(tc.canvasLen/tc.cellSize+2, axisLength)
			for _, offset := range tc.offsets {
				lay := computeAxisLayout(offset, tc.canvasLen, tc.cellSize, axisLength, bufLen)

				require.LessOrEqual(t, lay.start, 0, "first tile must start at or before 0")
				require.Greater(t, lay.tile, 0, "tile width must be positive")
				require.Equal(t, lay.count*tc.cellSize, lay.tile)
				require.LessOrEqual(t, lay.count, bufLen)

				if !lay.full {
					require.GreaterOrEqual(t, lay.start+lay.tile, tc.canvasLen,
						"non-full axis must cover canvas in a single tile")
				}

				// Simulate the exact tiling loop from worker.js and check every screen pixel.
				covered := make([]int, tc.canvasLen)
				cellAt := make([]uint16, tc.canvasLen)
				for tileStart := lay.start; tileStart < tc.canvasLen; tileStart += lay.tile {
					for j := 0; j < lay.count; j++ {
						var cell uint16
						if lay.full {
							cell = uint16(j)
						} else {
							cell = uint16(j-lay.shift) & wrapMask
						}
						px0 := tileStart + j*tc.cellSize
						for s := 0; s < tc.cellSize; s++ {
							px := px0 + s
							if px >= 0 && px < tc.canvasLen {
								covered[px]++
								cellAt[px] = cell
							}
						}
					}
				}

				for px := 0; px < tc.canvasLen; px++ {
					require.Equal(t, 1, covered[px], "offset=%d px=%d must be covered exactly once", offset, px)
					want := pixelToCell(px, offset, tc.cellSize, wrapMask)
					require.Equal(t, want, cellAt[px], "offset=%d px=%d rendered cell must match pixelToCell", offset, px)
				}
			}
		})
	}
}

func TestPixelBufferDirtyTrackingMatchesReference(t *testing.T) {
	const axisLength = 64
	const wrapMask = axisLength - 1

	// Naive reference renderer for the exact same pixel buffer.
	renderRef := func(cols, rows int, bg uint32, cells []protocol.Cell, xLay, yLay axisLayout, ageMode bool) []uint32 {
		out := make([]uint32, cols*rows)
		fillPixels(out, bg)
		for _, c := range cells {
			j := (int(c.X) + xLay.shift) & wrapMask
			k := (int(c.Y) + yLay.shift) & wrapMask
			if j >= cols || k >= rows {
				continue
			}
			var rgba uint32
			if ageMode {
				rgba = heatRGBA[min(c.Age, heatMaxAge)]
			} else {
				rgba = rgbToRGBA(c.Colour)
			}
			out[k*cols+j] = rgba
		}
		return out
	}

	lightBG := rgbToRGBA(0xf2f2f2)
	darkBG := rgbToRGBA(0x141414)

	var pb pixelBuffer
	pb.resize(axisLength, axisLength, lightBG)
	fullLay := computeAxisLayout(0, 200, 1, axisLength, axisLength)

	prev := make([]uint32, axisLength*axisLength)
	checkFrame := func(label string, cells []protocol.Cell, xLay, yLay axisLayout, ageMode bool) {
		t.Helper()
		dirtyY, dirtyH := pb.render(cells, axisLength, wrapMask, xLay, yLay, ageMode)
		want := renderRef(pb.cols, pb.rows, pb.bgRGBA, cells, xLay, yLay, ageMode)
		require.Equal(t, want, pb.pixels, label)

		// Every pixel that changed since the previous uploaded state must be inside [dirtyY, dirtyY+dirtyH).
		for row := 0; row < pb.rows; row++ {
			for col := 0; col < pb.cols; col++ {
				idx := row*pb.cols + col
				if pb.pixels[idx] != prev[idx] {
					require.Greater(t, dirtyH, 0, "%s: row %d changed but dirtyH=0", label, row)
					require.GreaterOrEqual(t, row, dirtyY, "%s: changed row %d < dirtyY %d", label, row, dirtyY)
					require.Less(t, row, dirtyY+dirtyH, "%s: changed row %d >= dirtyY+dirtyH %d", label, row, dirtyY+dirtyH)
				}
			}
		}
		copy(prev, pb.pixels)
	}

	// 1. Initial sparse cells
	sparse1 := []protocol.Cell{
		{X: 3, Y: 10, Colour: 0xff6600, Age: 0},
		{X: 40, Y: 12, Colour: 0x00a3c4, Age: 5},
	}
	checkFrame("initial sparse", sparse1, fullLay, fullLay, false)

	// 2. Sparse cells move (only rows 10..20 should be dirty, not all 64 rows!)
	sparse2 := []protocol.Cell{
		{X: 4, Y: 18, Colour: 0xff6600, Age: 1},
		{X: 41, Y: 20, Colour: 0x00a3c4, Age: 6},
	}
	dirtyY, dirtyH := pb.render(sparse2, axisLength, wrapMask, fullLay, fullLay, false)
	require.Equal(t, 10, dirtyY)
	require.Equal(t, 11, dirtyH) // rows 10..20 inclusive
	require.Equal(t, renderRef(axisLength, axisLength, lightBG, sparse2, fullLay, fullLay, false), pb.pixels)
	copy(prev, pb.pixels)

	// 3. Age mode on same cells
	checkFrame("age mode", sparse2, fullLay, fullLay, true)

	// 4. Dense world (> 12.5% fill switches to bulk clear)
	dense := make([]protocol.Cell, 0, 2000)
	for y := 0; y < axisLength; y++ {
		for x := 0; x < axisLength; x += 2 {
			dense = append(dense, protocol.Cell{X: uint16(x), Y: uint16(y), Colour: 0x18a957, Age: uint16(x + y)})
		}
	}
	checkFrame("dense world", dense, fullLay, fullLay, false)

	// 5. Clear after dense world
	checkFrame("clear after dense", nil, fullLay, fullLay, false)

	// 6. Second empty frame should report zero dirty rows
	dirtyY, dirtyH = pb.render(nil, axisLength, wrapMask, fullLay, fullLay, false)
	require.Equal(t, 0, dirtyH)

	// 7. Theme background colour switch
	pb.setBackground(darkBG)
	checkFrame("dark background", sparse1, fullLay, fullLay, false)

	// 8. Windowed (non-full) buffer + pan shift
	pb.resize(20, 15, darkBG)
	prev = make([]uint32, 20*15)
	winX1 := computeAxisLayout(0, 180, 10, axisLength, 20)
	winY1 := computeAxisLayout(0, 130, 10, axisLength, 15)
	checkFrame("windowed initial", dense, winX1, winY1, false)

	winX2 := computeAxisLayout(35, 180, 10, axisLength, 20)
	winY2 := computeAxisLayout(-25, 130, 10, axisLength, 15)
	checkFrame("windowed panned", dense, winX2, winY2, true)
}
