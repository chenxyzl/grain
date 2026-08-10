package ringbuffer

import (
	"sync"
)

// PushResult is the outcome of a Push.
type PushResult int8

const (
	// PushOK: the item was enqueued (the buffer grew first if it was full and
	// below its max capacity).
	PushOK PushResult = iota
	// PushOverflow: the buffer is full AND already at its max capacity, so the
	// item was dropped (the caller should route it to a dead letter). Existing
	// queued items are kept — the NEW item is the one dropped (Akka-style).
	PushOverflow
	// PushClosed: the buffer has been closed (actor stopped); the item was not
	// enqueued.
	PushClosed
)

// RingBuffer is a FIFO queue backed by a ring that grows on demand.
//   - Push never blocks. It grows the ring (doubling, up to maxCap) when full;
//     once at maxCap a further Push returns PushOverflow (the new item is
//     dropped) so senders are never blocked and cannot deadlock.
//   - Pop never blocks: it returns (zero, false) when empty, which keeps the
//     actor scheduler's drain-and-exit model intact.
type RingBuffer[T any] struct {
	mu     sync.Mutex
	items  []T
	head   int64 // index of next Pop
	tail   int64 // index of next Push
	size   int64 // current element count
	cap    int64 // current capacity (len(items))
	maxCap int64 // hard ceiling; the ring never grows past this
	closed bool
}

// New creates a ring buffer starting at initCap and growing (doubling) up to
// maxCap. initCap is clamped to >= 1 and maxCap to >= initCap.
func New[T any](initCap int64, maxCap int64) *RingBuffer[T] {
	if initCap <= 0 {
		initCap = 1
	}
	if maxCap < initCap {
		maxCap = initCap
	}
	return &RingBuffer[T]{
		items:  make([]T, initCap),
		cap:    initCap,
		maxCap: maxCap,
	}
}

// Push enqueues item and never blocks. See PushResult for the outcomes.
func (rb *RingBuffer[T]) Push(item T) PushResult {
	rb.mu.Lock()
	if rb.closed {
		rb.mu.Unlock()
		return PushClosed
	}
	if rb.size == rb.cap {
		if rb.cap >= rb.maxCap {
			rb.mu.Unlock()
			return PushOverflow
		}
		rb.grow()
	}
	rb.items[rb.tail] = item
	rb.tail = (rb.tail + 1) % rb.cap
	rb.size++
	rb.mu.Unlock()
	return PushOK
}

// grow doubles the capacity (capped at maxCap) and linearizes the ring into the
// new backing slice so head starts at 0. Must be called with rb.mu held and
// only when size == cap. Doubling keeps the amortized copy cost O(1).
func (rb *RingBuffer[T]) grow() {
	newCap := min(rb.cap*2, rb.maxCap)
	items := make([]T, newCap)
	// copy in FIFO order starting at head, unwrapping the ring
	if rb.head < rb.tail {
		copy(items, rb.items[rb.head:rb.tail])
	} else {
		n := copy(items, rb.items[rb.head:])
		copy(items[n:], rb.items[:rb.tail])
	}
	rb.items = items
	rb.head = 0
	rb.tail = rb.size // size < newCap, so no wrap
	rb.cap = newCap
}

// Close marks the buffer closed. Already-enqueued items remain poppable; further
// Push calls return PushClosed.
func (rb *RingBuffer[T]) Close() {
	rb.mu.Lock()
	rb.closed = true
	rb.mu.Unlock()
}

func (rb *RingBuffer[T]) Len() int64 {
	rb.mu.Lock()
	n := rb.size
	rb.mu.Unlock()
	return n
}

// Cap returns the current capacity (grows over time; never shrinks).
func (rb *RingBuffer[T]) Cap() int64 {
	rb.mu.Lock()
	c := rb.cap
	rb.mu.Unlock()
	return c
}

func (rb *RingBuffer[T]) Pop() (T, bool) {
	rb.mu.Lock()
	if rb.size == 0 {
		rb.mu.Unlock()
		var t T
		return t, false
	}
	item := rb.items[rb.head]
	var zero T
	rb.items[rb.head] = zero
	rb.head = (rb.head + 1) % rb.cap
	rb.size--
	rb.mu.Unlock()
	return item, true
}
