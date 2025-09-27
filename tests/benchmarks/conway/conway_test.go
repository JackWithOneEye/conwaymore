package conway_test

import (
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
