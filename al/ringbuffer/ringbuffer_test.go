package ringbuffer

import (
	"testing"
	"time"
)

type Item struct {
	i int
}

func TestPushPop(t *testing.T) {
	rb := New[Item](1024)
	for i := 0; i < 5000; i++ {
		rb.Push(Item{i})
		item, ok := rb.Pop()
		if ok {
			if item.i != i {
				t.Fatal("invalid item popped")
			}
		}
	}
}

// TestPushBlocksWhenFull verifies Push back-pressures (blocks) when the buffer
// is full and unblocks once a Pop frees a slot.
func TestPushBlocksWhenFull(t *testing.T) {
	rb := New[Item](2)
	rb.Push(Item{1})
	rb.Push(Item{2})

	pushed := make(chan struct{})
	go func() {
		rb.Push(Item{3}) // must block: buffer full
		close(pushed)
	}()

	select {
	case <-pushed:
		t.Fatal("Push should block while buffer is full")
	case <-time.After(50 * time.Millisecond):
	}

	if _, ok := rb.Pop(); !ok {
		t.Fatal("expected to pop an item")
	}

	select {
	case <-pushed:
	case <-time.After(time.Second):
		t.Fatal("Push should unblock after a Pop frees a slot")
	}
}

// TestPopEmptyNonBlocking verifies Pop returns immediately on an empty buffer.
func TestPopEmptyNonBlocking(t *testing.T) {
	rb := New[Item](4)
	if _, ok := rb.Pop(); ok {
		t.Fatal("Pop on empty buffer must return ok=false")
	}
}

// TestCloseWakesBlockedPush verifies Close unblocks a Push that is waiting on a
// full buffer, and that the woken Push returns false (item dropped).
func TestCloseWakesBlockedPush(t *testing.T) {
	rb := New[Item](1)
	if ok := rb.Push(Item{1}); !ok {
		t.Fatal("first push should succeed")
	}

	result := make(chan bool, 1)
	go func() {
		result <- rb.Push(Item{2}) // blocks: buffer full
	}()

	select {
	case <-result:
		t.Fatal("Push should block while buffer is full")
	case <-time.After(50 * time.Millisecond):
	}

	rb.Close()

	select {
	case ok := <-result:
		if ok {
			t.Fatal("Push after Close must return false")
		}
	case <-time.After(time.Second):
		t.Fatal("Close should wake the blocked Push")
	}

	// Push on a closed buffer returns false immediately.
	if ok := rb.Push(Item{3}); ok {
		t.Fatal("Push on closed buffer must return false")
	}
}

func BenchmarkPushPop(b *testing.B) {
	rb := New[Item](1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Push(Item{i})
		item, ok := rb.Pop()
		if ok {
			if item.i != i {
				b.Error("invalid item popped")
			}
		}
	}
}
