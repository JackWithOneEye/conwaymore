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
