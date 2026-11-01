//go:build js
// +build js

package canvas

import (
	"math"
)

// floorDiv divides a by b, rounding towards negative infinity (Go's /
// truncates towards zero, which is wrong for negative offsets here).
func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
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
