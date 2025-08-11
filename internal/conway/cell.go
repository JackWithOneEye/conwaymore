package conway

import "iter"

type Cell interface {
	Values() (x, y uint16, colour uint32, age uint16)
}

type aliveCell struct {
	x      uint16
	y      uint16
	colour uint32
	age    uint16
}

func (ac *aliveCell) Values() (x, y uint16, colour uint32, age uint16) {
	return ac.x, ac.y, ac.colour, ac.age
}

type swapSet[V any] struct {
	current uint8
	sets    [2]map[uint32]V
}

func newSwapSet[V any](capacity uint) *swapSet[V] {
	return &swapSet[V]{
		current: 0,
		sets:    [2]map[uint32]V{make(map[uint32]V, capacity), make(map[uint32]V, capacity)},
	}
}

func (sws *swapSet[V]) add(x, y uint16, data V) {
	sws.sets[sws.current][toCoord(x, y)] = data
}

func (sws *swapSet[V]) addNext(x, y uint16, data V) {
	sws.sets[sws.current^1][toCoord(x, y)] = data
}

func (sws *swapSet[V]) addNextByKey(key uint32, data V) {
	sws.sets[sws.current^1][key] = data
}

func (sws *swapSet[V]) clearAll() {
	clear(sws.sets[0])
	clear(sws.sets[1])
}

func (sws *swapSet[V]) clearNext() {
	clear(sws.sets[sws.current^1])
}

func (sws *swapSet[V]) get(x, y uint16) (V, bool) {
	d, ok := sws.sets[sws.current][toCoord(x, y)]
	return d, ok
}

func (sws *swapSet[V]) size() int {
	return len(sws.sets[sws.current])
}

func (sws *swapSet[V]) swap() {
	sws.current ^= 1
}

func (sws *swapSet[V]) values() map[uint32]V {
	return sws.sets[sws.current]
}

func (sws *swapSet[V]) iter() iter.Seq2[uint16, uint16] {
	return func(yield func(uint16, uint16) bool) {
		for key := range sws.sets[sws.current] {
			x := uint16(key >> 16)
			y := uint16(key)
			if !yield(x, y) {
				return
			}
		}
	}
}

func toCoord(x, y uint16) uint32 {
	return (uint32(x) << 16) | uint32(y)
}
