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
	current map[uint32]V
	next    map[uint32]V
}

func newSwapSet[V any](capacity uint) *swapSet[V] {
	return &swapSet[V]{
		current: make(map[uint32]V, capacity),
		next:    make(map[uint32]V, capacity),
	}
}

func (sws *swapSet[V]) add(x, y uint16, data V) {
	sws.current[toCoord(x, y)] = data
}

func (sws *swapSet[V]) addNext(x, y uint16, data V) {
	sws.next[toCoord(x, y)] = data
}

func (sws *swapSet[V]) addNextByKey(key uint32, data V) {
	sws.next[key] = data
}

func (sws *swapSet[V]) clearAll() {
	clear(sws.current)
	clear(sws.next)
}

func (sws *swapSet[V]) clearNext() {
	clear(sws.next)
}

func (sws *swapSet[V]) get(x, y uint16) (V, bool) {
	d, ok := sws.current[toCoord(x, y)]
	return d, ok
}

func (sws *swapSet[V]) size() int {
	return len(sws.current)
}

func (sws *swapSet[V]) swap() {
	tmp := sws.current
	sws.current = sws.next
	sws.next = tmp
}

func (sws *swapSet[V]) values() map[uint32]V {
	return sws.current
}

func (sws *swapSet[V]) iter() iter.Seq2[uint16, uint16] {
	return func(yield func(uint16, uint16) bool) {
		for key := range sws.current {
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
