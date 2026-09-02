package safemap

import "sync"

// RWMap is a map guarded by a single RWMutex.
type RWMap[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

func NewRWMap[K comparable, V any]() *RWMap[K, V] {
	return &RWMap[K, V]{
		m: make(map[K]V),
	}
}

func (rm *RWMap[K, V]) Get(key K) (V, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	val, exists := rm.m[key]
	return val, exists
}

// GetOrCreate returns the value under key, calling create() to build and store one when absent.
// Check and insert happen under one write lock, so concurrent callers all receive the SAME
// value — unlike Get + Set, where both can miss, both build, and the second Set clobbers the
// value the first already handed out. create runs under the write lock: it must not touch rm.
func (rm *RWMap[K, V]) GetOrCreate(key K, create func() V) V {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if val, exists := rm.m[key]; exists {
		return val
	}
	val := create()
	rm.m[key] = val
	return val
}

func (rm *RWMap[K, V]) Set(key K, value V) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.m[key] = value
}

func (rm *RWMap[K, V]) Delete(key K) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	delete(rm.m, key)
}

func (rm *RWMap[K, V]) Len() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.m)
}

// Range calls f for every entry; f returning false breaks the loop.
//
// WARNING: f runs while the read lock is held, so it MUST NOT call back into this map — a
// nested write deadlocks outright, a nested read deadlocks as soon as a writer is queued
// (sync.RWMutex read locks are not reentrant). Snapshot here and act afterwards instead.
func (rm *RWMap[K, V]) Range(f func(key K, value V) bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	for k, v := range rm.m {
		if !f(k, v) {
			break
		}
	}
}
