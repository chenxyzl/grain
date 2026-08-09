package ringbuffer

import (
	"sync"
	"sync/atomic"
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

// TestSchedulerDrainNoLostWakeup reproduces the production deadlock faithfully.
// It mirrors the actor scheduler: a single drainer pops until empty then parks,
// and is only (re)started by schedule(), which a producer calls right AFTER its
// Push returns (processorMailBox.send). So a producer whose Push never unblocks
// never restarts the drainer.
//
// With a Pop that signals only when it observed the buffer full, one drain pass
// wakes just a single blocked producer; the drainer then pops that one item,
// finds the buffer empty on a non-full pop (no signal), and parks — stranding
// every other blocked producer forever. Every freed slot must wake a waiter.
func TestSchedulerDrainNoLostWakeup(t *testing.T) {
	const capacity = 4
	const producers = 64
	const perProducer = 50
	const total = producers * perProducer

	rb := New[Item](capacity)

	var running int32
	var received int64
	var schedule func()
	drain := func() {
		for {
			if _, ok := rb.Pop(); !ok {
				atomic.StoreInt32(&running, 0)
				// re-arm if work reappeared after we decided to park
				if rb.Len() > 0 {
					schedule()
				}
				return
			}
			atomic.AddInt64(&received, 1)
		}
	}
	schedule = func() {
		if atomic.CompareAndSwapInt32(&running, 0, 1) {
			go drain()
		}
	}

	var wg sync.WaitGroup
	wg.Add(producers)
	for p := 0; p < producers; p++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perProducer; i++ {
				rb.Push(Item{i}) // may block on a full buffer (back-pressure)
				schedule()       // restart the drainer AFTER Push returns
			}
		}()
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("lost wakeup deadlock: only %d/%d items sent", atomic.LoadInt64(&received), total)
	}

	// drain any tail left in the buffer after the last producer finished
	for {
		if _, ok := rb.Pop(); !ok {
			break
		}
		atomic.AddInt64(&received, 1)
	}
	if got := atomic.LoadInt64(&received); got != total {
		t.Fatalf("received %d items, want %d", got, total)
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
