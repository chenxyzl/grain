package safemap

import "sync"

// RWMap read write lock with map
type RWMap[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

// NewRWMap ...
func NewRWMap[K comparable, V any]() *RWMap[K, V] {
	return &RWMap[K, V]{
		m: make(map[K]V),
	}
}

// Get ...
func (rm *RWMap[K, V]) Get(key K) (V, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	val, exists := rm.m[key]
	return val, exists
}

// GetOrCreate returns the value stored under key, calling create() to build and
// store one when the key is absent. The check and the insert happen under a
// single write lock, so concurrent callers all receive the *same* value.
//
// This is not equivalent to Get + nil-check + Set: in that sequence two callers
// can both miss, both build, and the second Set clobbers the value the first one
// already handed out, silently discarding whatever was written into it.
//
// create runs while the write lock is held, so it must not touch this map.
func (rm *RWMap[K, V]) GetOrCreate(key K, create func() V) V {
	rm.mu.Lock()         // w lock
	defer rm.mu.Unlock() // w unlock
	if val, exists := rm.m[key]; exists {
		return val
	}
	val := create()
	rm.m[key] = val
	return val
}

// Set ...
func (rm *RWMap[K, V]) Set(key K, value V) {
	rm.mu.Lock()         // w lock
	defer rm.mu.Unlock() // w unlock
	rm.m[key] = value
}

// Delete ...
func (rm *RWMap[K, V]) Delete(key K) {
	rm.mu.Lock()         // w lock
	defer rm.mu.Unlock() // w unlock
	delete(rm.m, key)
}

// Len ...
func (rm *RWMap[K, V]) Len() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.m)
}

// Range ...
// return@true break for range
//
// WARNING: f runs while the read lock is held, so it MUST NOT call back into this
// map (a nested write deadlocks outright; a nested read deadlocks as soon as a
// writer is queued, because sync.RWMutex read locks are not reentrant). Snapshot
// and act afterwards if you need that.
func (rm *RWMap[K, V]) Range(f func(key K, value V) bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	for k, v := range rm.m {
		if !f(k, v) {
			break
		}
	}
}
