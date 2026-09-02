package safemap

import (
	"encoding/json"
	"fmt"
	"hash/maphash"
	"sync"
)

const ShardCount = 32

type Stringer interface {
	fmt.Stringer
	comparable
}

// ConcurrentMap is a concurrency-safe map split over ShardCount independently locked shards.
type ConcurrentMap[K comparable, V any] struct {
	shards   []*ConcurrentMapShared[K, V]
	sharding func(key K) uint32
}

// ConcurrentMapShared is one shard of a ConcurrentMap.
type ConcurrentMapShared[K comparable, V any] struct {
	items        map[K]V
	sync.RWMutex // guards items
}

func create[K comparable, V any](sharding func(key K) uint32) ConcurrentMap[K, V] {
	m := ConcurrentMap[K, V]{
		sharding: sharding,
		shards:   make([]*ConcurrentMapShared[K, V], ShardCount),
	}
	for i := 0; i < ShardCount; i++ {
		m.shards[i] = &ConcurrentMapShared[K, V]{items: make(map[K]V)}
	}
	return m
}

// NewIntC creates a concurrent map with integer keys.
func NewIntC[K int | uint | int32 | uint32 | int64 | uint64, V any]() ConcurrentMap[K, V] {
	return create[K, V](intFnv32[K])
}

// NewStringC creates a concurrent map with string keys.
func NewStringC[V any]() ConcurrentMap[string, V] {
	return create[string, V](stringShardHash)
}

// NewStringerC creates a concurrent map keyed by a Stringer.
//
//lint:ignore U1000 Ignore unused function temporarily for debugging
func NewStringerC[K Stringer, V any]() ConcurrentMap[K, V] {
	return create[K, V](stringerFnv32[K])
}

// NewCustomC creates a concurrent map using a caller-supplied shard hash.
//
//lint:ignore U1000 Ignore unused function temporarily for debugging
func NewCustomC[K comparable, V any](sharding func(key K) uint32) ConcurrentMap[K, V] {
	return create[K, V](sharding)
}

// GetShard returns the shard owning key.
func (m *ConcurrentMap[K, V]) GetShard(key K) *ConcurrentMapShared[K, V] {
	return m.shards[uint(m.sharding(key))%uint(ShardCount)]
}

// Set stores value under key, returning the previous value and whether there was one.
func (m *ConcurrentMap[K, V]) Set(key K, value V) (V, bool) {
	shard := m.GetShard(key)
	shard.Lock()
	v, b := shard.items[key]
	shard.items[key] = value
	shard.Unlock()
	return v, b
}

// SetIfNotExist stores value only if key is absent, returning the old value and whether it existed.
func (m *ConcurrentMap[K, V]) SetIfNotExist(key K, value V) (V, bool) {
	shard := m.GetShard(key)
	shard.Lock()
	v, b := shard.items[key]
	if !b {
		shard.items[key] = value
	}
	shard.Unlock()
	return v, b
}

// UpsertCb returns the element to store. It runs with the shard's write lock held, so it MUST
// NOT touch this map: sync.RWMutex is not reentrant, so a nested access deadlocks.
type UpsertCb[V any] func(exist bool, valueInMap V, newValue V) V

// Upsert stores whatever cb returns for key, inserting or updating, and returns it.
func (m *ConcurrentMap[K, V]) Upsert(key K, value V, cb UpsertCb[V]) (res V) {
	shard := m.GetShard(key)
	shard.Lock()
	v, ok := shard.items[key]
	res = cb(ok, v, value)
	shard.items[key] = res
	shard.Unlock()
	return res
}

// SetIfAbsent stores value only if key is absent, reporting whether it did.
func (m *ConcurrentMap[K, V]) SetIfAbsent(key K, value V) bool {
	shard := m.GetShard(key)
	shard.Lock()
	_, ok := shard.items[key]
	if !ok {
		shard.items[key] = value
	}
	shard.Unlock()
	return !ok
}

func (m *ConcurrentMap[K, V]) Get(key K) (V, bool) {
	shard := m.GetShard(key)
	shard.RLock()
	val, ok := shard.items[key]
	shard.RUnlock()
	return val, ok
}

func (m *ConcurrentMap[K, V]) Count() int {
	count := 0
	for i := 0; i < ShardCount; i++ {
		shard := m.shards[i]
		shard.RLock()
		count += len(shard.items)
		shard.RUnlock()
	}
	return count
}

func (m *ConcurrentMap[K, V]) Has(key K) bool {
	shard := m.GetShard(key)
	shard.RLock()
	_, ok := shard.items[key]
	shard.RUnlock()
	return ok
}

func (m *ConcurrentMap[K, V]) Remove(key K) {
	shard := m.GetShard(key)
	shard.Lock()
	delete(shard.items, key)
	shard.Unlock()
}

// RemoveCb runs under the shard's write lock (MUST NOT touch this map); returning true removes.
type RemoveCb[K any, V any] func(key K, v V, exists bool) bool

// RemoveCb calls cb with the key's current value under the shard's write lock, deleting the
// entry if cb returns true and it exists. Returns cb's result even when the key was absent.
func (m *ConcurrentMap[K, V]) RemoveCb(key K, cb RemoveCb[K, V]) bool {
	shard := m.GetShard(key)
	shard.Lock()
	v, ok := shard.items[key]
	remove := cb(key, v, ok)
	if remove && ok {
		delete(shard.items, key)
	}
	shard.Unlock()
	return remove
}

func (m *ConcurrentMap[K, V]) Pop(key K) (v V, exists bool) {
	shard := m.GetShard(key)
	shard.Lock()
	v, exists = shard.items[key]
	delete(shard.items, key)
	shard.Unlock()
	return v, exists
}

func (m *ConcurrentMap[K, V]) IsEmpty() bool {
	return m.Count() == 0
}

// Tuple is a key/value pair carried over the IterBuffered channel.
type Tuple[K comparable, V any] struct {
	Key K
	Val V
}

// IterBuffered returns a channel of every entry, rangeable without holding any shard lock.
func (m *ConcurrentMap[K, V]) IterBuffered() <-chan Tuple[K, V] {
	chs := snapshot(m)
	total := 0
	for _, c := range chs {
		total += cap(c)
	}
	ch := make(chan Tuple[K, V], total)
	go fanIn(chs, ch)
	return ch
}

func (m *ConcurrentMap[K, V]) Clear() {
	for item := range m.IterBuffered() {
		m.Remove(item.Key)
	}
}

// snapshot returns one buffered channel of entries per shard, returning as soon as the channels
// are sized while goroutines are still filling them: a per-shard snapshot, not a whole-map one.
func snapshot[K comparable, V any](m *ConcurrentMap[K, V]) (chs []chan Tuple[K, V]) {
	if len(m.shards) == 0 {
		panic(`ConcurrentMap is not initialized. Should run NewStringC() before usage.`)
	}
	chs = make([]chan Tuple[K, V], ShardCount)
	wg := sync.WaitGroup{}
	wg.Add(ShardCount)
	for index, shard := range m.shards {
		go func(index int, shard *ConcurrentMapShared[K, V]) {
			shard.RLock()
			chs[index] = make(chan Tuple[K, V], len(shard.items))
			wg.Done()
			for key, val := range shard.items {
				chs[index] <- Tuple[K, V]{key, val}
			}
			shard.RUnlock()
			close(chs[index])
		}(index, shard)
	}
	wg.Wait()
	return chs
}

// fanIn merges chs into out, closing out once all are drained.
func fanIn[K comparable, V any](chs []chan Tuple[K, V], out chan Tuple[K, V]) {
	wg := sync.WaitGroup{}
	wg.Add(len(chs))
	for _, ch := range chs {
		go func(ch chan Tuple[K, V]) {
			for t := range ch {
				out <- t
			}
			wg.Done()
		}(ch)
	}
	wg.Wait()
	close(out)
}

func (m *ConcurrentMap[K, V]) Items() map[K]V {
	tmp := make(map[K]V)

	for item := range m.IterBuffered() {
		tmp[item.Key] = item.Val
	}

	return tmp
}

// IterCb is called for every key/value; the view is consistent within a shard, not across them.
type IterCb[K comparable, V any] func(key K, v V)

// IterCb is the cheapest way to read every element. WARNING: fn runs under the shard's read
// lock, so it MUST NOT call back into this map, not even a read — RWMutex read locks are not
// reentrant once a writer is queued, so a nested Get on the same shard blocks behind that writer
// while holding the outer read lock: permanent deadlock, shard wedged for good. Snapshot the
// entries here and act on them afterwards instead (see registry.rangeIt).
func (m *ConcurrentMap[K, V]) IterCb(fn IterCb[K, V]) {
	for idx := range m.shards {
		shard := (m.shards)[idx]
		shard.RLock()
		for key, value := range shard.items {
			fn(key, value)
		}
		shard.RUnlock()
	}
}

func (m *ConcurrentMap[K, V]) Keys() []K {
	count := m.Count()
	ch := make(chan K, count)
	go func() {
		wg := sync.WaitGroup{}
		wg.Add(ShardCount)
		for _, shard := range m.shards {
			go func(shard *ConcurrentMapShared[K, V]) {
				shard.RLock()
				for key := range shard.items {
					ch <- key
				}
				shard.RUnlock()
				wg.Done()
			}(shard)
		}
		wg.Wait()
		close(ch)
	}()

	keys := make([]K, 0, count)
	for k := range ch {
		keys = append(keys, k)
	}
	return keys
}

func (m *ConcurrentMap[K, V]) MarshalJSON() ([]byte, error) {
	tmp := make(map[K]V)

	for item := range m.IterBuffered() {
		tmp[item.Key] = item.Val
	}
	return json.Marshal(tmp)
}

func (m *ConcurrentMap[K, V]) UnmarshalJSON(b []byte) (err error) {
	tmp := make(map[K]V)

	if err := json.Unmarshal(b, &tmp); err != nil {
		return err
	}

	for key, val := range tmp {
		m.Set(key, val)
	}
	return nil
}

func stringerFnv32[K fmt.Stringer](key K) uint32 {
	return stringFnv32(key.String())
}

// shardSeed seeds the shard hash; one process-wide seed is fine, sharding is never persisted.
var shardSeed = maphash.MakeSeed()

// stringShardHash picks a shard for a string key. maphash (runtime AES-accelerated), not the
// byte-at-a-time FNV below: 8.2ns versus 31.7ns on the framework's ~50-char actor ids, paid once
// per message since the registry lookup is on the send path, and distribution is equivalent.
func stringShardHash(key string) uint32 {
	return uint32(maphash.String(shardSeed, key))
}

func stringFnv32(key string) uint32 {
	hash := uint32(2166136261)
	const prime32 = uint32(16777619)
	keyLength := len(key)
	for i := 0; i < keyLength; i++ {
		hash *= prime32
		hash ^= uint32(key[i])
	}
	return hash
}

func intFnv32[K int | uint | int32 | uint32 | int64 | uint64](key K) uint32 {
	hash := uint32(2166136261)
	const prime32 = uint32(16777619)
	// the 8 raw bytes directly — no strconv/string allocation
	u := uint64(key)
	for i := 0; i < 8; i++ {
		hash *= prime32
		hash ^= uint32(u & 0xff)
		u >>= 8
	}
	return hash
}
