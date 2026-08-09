package ringbuffer

import (
	"sync"
)

// RingBuffer is a fixed-capacity FIFO queue.
//   - Push blocks when the buffer is full (back-pressure on the sender), and
//     returns false once the buffer is closed (so a blocked sender to a dead
//     mailbox is woken rather than stuck forever).
//   - Pop never blocks: it returns (zero, false) when empty, which keeps the
//     actor scheduler's drain-and-exit model intact.
//
// warning: pushing to your own already-full mailbox blocks the actor goroutine
// and can self-deadlock. Size the mailbox for the expected in-flight depth.
type RingBuffer[T any] struct {
	mu      sync.Mutex
	notFull *sync.Cond
	items   []T
	head    int64 // index of next Pop
	tail    int64 // index of next Push
	size    int64 // current element count
	cap     int64
	closed  bool
}

func New[T any](size int64) *RingBuffer[T] {
	if size <= 0 {
		size = 1
	}
	rb := &RingBuffer[T]{
		items: make([]T, size),
		cap:   size,
	}
	rb.notFull = sync.NewCond(&rb.mu)
	return rb
}

// Push appends item, blocking while the buffer is full. It returns false if the
// buffer has been closed (item is not enqueued), true otherwise.
func (rb *RingBuffer[T]) Push(item T) bool {
	rb.mu.Lock()
	for rb.size == rb.cap && !rb.closed {
		rb.notFull.Wait()
	}
	if rb.closed {
		rb.mu.Unlock()
		return false
	}
	rb.items[rb.tail] = item
	rb.tail = (rb.tail + 1) % rb.cap
	rb.size++
	rb.mu.Unlock()
	return true
}

// Close marks the buffer closed and wakes every blocked Push. Already-enqueued
// items remain poppable; further Push calls return false.
func (rb *RingBuffer[T]) Close() {
	rb.mu.Lock()
	rb.closed = true
	rb.notFull.Broadcast()
	rb.mu.Unlock()
}

func (rb *RingBuffer[T]) Len() int64 {
	rb.mu.Lock()
	n := rb.size
	rb.mu.Unlock()
	return n
}

func (rb *RingBuffer[T]) Pop() (T, bool) {
	rb.mu.Lock()
	if rb.size == 0 {
		rb.mu.Unlock()
		var t T
		return t, false
	}
	wasFull := rb.size == rb.cap
	item := rb.items[rb.head]
	var zero T
	rb.items[rb.head] = zero
	rb.head = (rb.head + 1) % rb.cap
	rb.size--
	// only wake a blocked Push if the buffer was actually full; on the common
	// not-full path there is no waiter, so skip the Cond notify.
	if wasFull {
		rb.notFull.Signal()
	}
	rb.mu.Unlock()
	return item, true
}
