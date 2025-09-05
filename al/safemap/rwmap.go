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
func (rm *RWMap[K, V]) Range(f func(key K, value V) bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	for k, v := range rm.m {
		if !f(k, v) {
			break
		}
	}
}
