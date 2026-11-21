package conway_test

import (
	"fmt"
	"testing"

	"github.com/JackWithOneEye/conwaymore/internal/conway"
	"github.com/JackWithOneEye/conwaymore/internal/patterns"
)

type config struct {
	worldSize uint
}

func (c config) WorldSize() uint {
	return c.worldSize
}

func BenchmarkRun1024(b *testing.B) {
	p := patterns.Patterns["295P5H1V1"]

	for b.Loop() {
		b.StopTimer()
		c := conway.NewConway(config{worldSize: 1024})
		for _, pc := range p.Cells {
			c.SetCell(pc.X, pc.Y, 0, 0)
		}
		b.StartTimer()

		c.NextGen()
	}
}

func BenchmarkRun2048(b *testing.B) {
	p := patterns.Patterns["295P5H1V1"]

	for b.Loop() {
		b.StopTimer()
		c := conway.NewConway(config{worldSize: 2048})
		for _, pc := range p.Cells {
			c.SetCell(pc.X, pc.Y, 0, 0)
		}
		b.StartTimer()

		c.NextGen()
	}
}

func BenchmarkRunAfterRandom1024(b *testing.B) {
	for b.Loop() {
		b.StopTimer()
		c := conway.NewConway(config{worldSize: 1024})
		c.Randomise()
		b.StartTimer()

		c.NextGen()
	}
}

func BenchmarkRunAfterRandom2048(b *testing.B) {
	for b.Loop() {
		b.StopTimer()
		c := conway.NewConway(config{worldSize: 2048})
		c.Randomise()
		b.StartTimer()

		c.NextGen()
	}
}

// soup returns a world in a steady state: a random fill run for 300 generations,
// which settles at a density of about 5%.
func soup(size uint) conway.Conway {
	c := conway.NewConway(config{worldSize: size})
	c.Randomise()
	for range 300 {
		c.NextGen()
	}
	return c
}

func sparse(size uint) conway.Conway {
	c := conway.NewConway(config{worldSize: size})
	for _, pc := range patterns.Patterns["295P5H1V1"].Cells {
		c.SetCell(pc.X, pc.Y, 0, 0)
	}
	return c
}

// BenchmarkSteadyState times generations and the scan that builds each frame
// on worlds that have been running for a while, as on a playing server.
func BenchmarkSteadyState(b *testing.B) {
	for _, size := range []uint{1024, 2048} {
		for _, w := range []struct {
			name string
			new  func(uint) conway.Conway
		}{{"soup", soup}, {"sparse", sparse}} {
			c := w.new(size)
			b.Run(fmt.Sprintf("%s%d/NextGen", w.name, size), func(b *testing.B) {
				for b.Loop() {
					c.NextGen()
				}
			})
			b.Run(fmt.Sprintf("%s%d/Cells", w.name, size), func(b *testing.B) {
				for b.Loop() {
					for range c.Cells() {
					}
				}
			})
		}
	}
}
