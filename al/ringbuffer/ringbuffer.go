package ringbuffer

import (
	"sync"
)

// PushResult is the outcome of a Push.
type PushResult int8

const (
	// PushOK: enqueued; the ring grew first if it was full and below maxCap.
	PushOK PushResult = iota
	// PushOverflow: full at maxCap — the NEW item is dropped (queued ones kept, Akka-style).
	PushOverflow
	// PushClosed: closed (actor stopped); not enqueued.
	PushClosed
)

// RingBuffer is a FIFO queue backed by a ring that doubles on demand up to maxCap, never
// shrinking. Neither end ever blocks: full at maxCap returns PushOverflow instead of parking
// the sender, empty Pops (zero, false), which keeps the scheduler's drain-and-exit model.
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

// New creates a ring buffer from initCap, doubling up to maxCap; initCap is clamped to >= 1
// and maxCap to >= initCap.
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
	rb.tail = rb.next(rb.tail)
	rb.size++
	rb.mu.Unlock()
	return PushOK
}

// next advances a ring index, wrapping at cap. A branch, not `(i+1) % rb.cap`: cap is a runtime
// variable, so modulo compiles to a hardware DIV, ~10ns twice per message inside the mutex.
// Masking power-of-two caps is faster still but would inflate a configured maxMailbox to 1024.
func (rb *RingBuffer[T]) next(i int64) int64 {
	i++
	if i == rb.cap {
		return 0
	}
	return i
}

// grow doubles the capacity (capped at maxCap, so copying stays O(1) amortized) and unwraps the
// ring into the new slice in FIFO order so head starts at 0. Hold rb.mu; only valid at size==cap.
func (rb *RingBuffer[T]) grow() {
	newCap := min(rb.cap*2, rb.maxCap)
	items := make([]T, newCap)
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

// Close marks the buffer closed: queued items stay poppable, further Push returns PushClosed.
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
	rb.head = rb.next(rb.head)
	rb.size--
	rb.mu.Unlock()
	return item, true
}
