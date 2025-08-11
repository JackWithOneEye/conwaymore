package lrucache

import (
	"container/list"
	"sync"
)

type LruCache[K, V comparable] interface {
	Get(key K) (V, bool)
	Add(key K, val V)
}

type lruCache[K, V comparable] struct {
	mu  sync.Mutex
	cap int
	ll  *list.List
	m   map[K]*list.Element
}

type lruCacheEntry[K, V comparable] struct {
	key K
	val V
}

func NewLruCache[K, V comparable](capacity int) LruCache[K, V] {
	return &lruCache[K, V]{cap: max(1, capacity), ll: list.New(), m: make(map[K]*list.Element)}
}

func (c *lruCache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ele, ok := c.m[key]; ok {
		c.ll.MoveToFront(ele)
		return ele.Value.(lruCacheEntry[K, V]).val, true
	}
	return *new(V), false
}

func (c *lruCache[K, V]) Add(key K, val V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ele, ok := c.m[key]; ok {
		ele.Value = lruCacheEntry[K, V]{key: key, val: val}
		c.ll.MoveToFront(ele)
		return
	}
	ele := c.ll.PushFront(lruCacheEntry[K, V]{key: key, val: val})
	c.m[key] = ele
	if c.ll.Len() > c.cap {
		tail := c.ll.Back()
		if tail != nil {
			c.ll.Remove(tail)
			ent := tail.Value.(lruCacheEntry[K, V])
			delete(c.m, ent.key)
		}
	}
}
