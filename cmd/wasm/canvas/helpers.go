//go:build js
// +build js

package canvas

import (
	"math"
)

type coordBoundary struct {
	start  uint16
	end    uint16
	within bool
}

func (b *coordBoundary) calc(visiblePx, actualPx int, offsetPx, cellSizeInv float64, axisLen uint16) {
	defer func() { b.within = b.start < b.end }()

	if visiblePx >= actualPx {
		b.start = 0
		b.end = math.MaxUint16
		return
	}
	if offsetPx <= 0 {
		b.start = uint16(math.Floor(-offsetPx * cellSizeInv))
	} else {
		b.start = uint16(math.Floor(float64(axisLen) - offsetPx*cellSizeInv))
	}
	b.end = b.start + uint16(math.Ceil(float64(visiblePx)*cellSizeInv))
	if b.end >= axisLen {
		b.end -= axisLen
	}
}

func absInt(v int) (int, int) {
	if v < 0 {
		return -1, -v
	}
	return 1, v
}
