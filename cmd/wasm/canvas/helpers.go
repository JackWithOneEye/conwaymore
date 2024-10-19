//go:build js
// +build js

package canvas

import (
	"math"
)

type canvasDim struct {
	height float64
	width  float64
}

func (cd *canvasDim) set(height, width float64) {
	cd.height = height
	cd.width = width
}

func (cd *canvasDim) values() (height, width float64) {
	return cd.height, cd.width
}

type coordBoundary struct {
	start  uint16
	end    uint16
	within bool
}

func (b *coordBoundary) calc(visiblePx, actualPx, offsetPx, cellSizeInv float64, axisLen uint16) {
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
	b.end = b.start + uint16(math.Ceil(visiblePx*cellSizeInv))
	if b.end >= axisLen {
		b.end -= axisLen
	}
}
