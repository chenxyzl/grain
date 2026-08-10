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
	rb := New[Item](1024, 1024)
	for i := 0; i < 5000; i++ {
		if r := rb.Push(Item{i}); r != PushOK {
			t.Fatalf("push %d: got %v want PushOK", i, r)
		}
		item, ok := rb.Pop()
		if ok {
			if item.i != i {
				t.Fatal("invalid item popped")
			}
		}
	}
}

// TestPopEmptyNonBlocking verifies Pop returns immediately on an empty buffer.
func TestPopEmptyNonBlocking(t *testing.T) {
	rb := New[Item](4, 4)
	if _, ok := rb.Pop(); ok {
		t.Fatal("Pop on empty buffer must return ok=false")
	}
}

// TestGrowPreservesFIFO fills past the initial capacity so the ring doubles
// several times, then verifies every item comes back in FIFO order.
func TestGrowPreservesFIFO(t *testing.T) {
	rb := New[Item](2, 1024)
	const n = 500
	for i := 0; i < n; i++ {
		if r := rb.Push(Item{i}); r != PushOK {
			t.Fatalf("push %d unexpectedly failed: %v (cap=%d)", i, r, rb.Cap())
		}
	}
	if rb.Len() != n {
		t.Fatalf("len=%d want %d", rb.Len(), n)
	}
	if rb.Cap() < n {
		t.Fatalf("cap=%d should have grown to >= %d", rb.Cap(), n)
	}
	for i := 0; i < n; i++ {
		item, ok := rb.Pop()
		if !ok || item.i != i {
			t.Fatalf("pop %d: got (%v,%v) want (%d,true)", i, item, ok, i)
		}
	}
	if _, ok := rb.Pop(); ok {
		t.Fatal("buffer should be empty")
	}
}

// TestGrowAfterWrap forces the ring into a wrapped state (head > tail) before a
// grow, exercising the two-segment linearizing copy path.
func TestGrowAfterWrap(t *testing.T) {
	rb := New[Item](4, 1024)
	// fill 4
	for i := 0; i < 4; i++ {
		rb.Push(Item{i})
	}
	// pop 2 (head advances to 2)
	rb.Pop()
	rb.Pop()
	// push 2 more -> tail wraps to 2, so head(2) == tail(2), size==4, full & wrapped
	rb.Push(Item{4})
	rb.Push(Item{5})
	// next push must grow while wrapped; remaining logical order is 2,3,4,5
	if r := rb.Push(Item{6}); r != PushOK {
		t.Fatalf("push after wrap-grow failed: %v", r)
	}
	want := []int{2, 3, 4, 5, 6}
	for _, w := range want {
		item, ok := rb.Pop()
		if !ok || item.i != w {
			t.Fatalf("wrap-grow order: got (%v,%v) want %d", item, ok, w)
		}
	}
}

// TestOverflowDropsNewItem verifies that at max capacity Push returns
// PushOverflow and drops the NEW item, leaving queued items intact.
func TestOverflowDropsNewItem(t *testing.T) {
	rb := New[Item](2, 2) // init==max==2, never grows
	if rb.Push(Item{1}) != PushOK {
		t.Fatal("push 1 should be OK")
	}
	if rb.Push(Item{2}) != PushOK {
		t.Fatal("push 2 should be OK")
	}
	if r := rb.Push(Item{3}); r != PushOverflow {
		t.Fatalf("push 3 at max cap: got %v want PushOverflow", r)
	}
	// queued items 1,2 must be intact and in order; 3 was dropped
	if item, ok := rb.Pop(); !ok || item.i != 1 {
		t.Fatalf("got %v,%v want 1", item, ok)
	}
	if item, ok := rb.Pop(); !ok || item.i != 2 {
		t.Fatalf("got %v,%v want 2", item, ok)
	}
	if _, ok := rb.Pop(); ok {
		t.Fatal("only two items should have been enqueued")
	}
}

// TestGrowStopsAtMax verifies the ring doubles only up to maxCap, then overflows.
func TestGrowStopsAtMax(t *testing.T) {
	rb := New[Item](2, 5) // doubles 2->4->5(capped), then overflow
	for i := 0; i < 5; i++ {
		if r := rb.Push(Item{i}); r != PushOK {
			t.Fatalf("push %d: got %v want PushOK (cap=%d)", i, r, rb.Cap())
		}
	}
	if rb.Cap() != 5 {
		t.Fatalf("cap=%d want 5 (capped at max)", rb.Cap())
	}
	if r := rb.Push(Item{99}); r != PushOverflow {
		t.Fatalf("push at max: got %v want PushOverflow", r)
	}
}

// TestClosedReturnsPushClosed verifies Push after Close returns PushClosed and
// does not enqueue, while already-queued items remain poppable.
func TestClosedReturnsPushClosed(t *testing.T) {
	rb := New[Item](4, 4)
	rb.Push(Item{1})
	rb.Close()
	if r := rb.Push(Item{2}); r != PushClosed {
		t.Fatalf("push after close: got %v want PushClosed", r)
	}
	// the pre-close item is still poppable
	if item, ok := rb.Pop(); !ok || item.i != 1 {
		t.Fatalf("got %v,%v want 1", item, ok)
	}
	if _, ok := rb.Pop(); ok {
		t.Fatal("closed push must not have enqueued item 2")
	}
}

// TestMPSCNoLossWhenSized runs many concurrent producers against a single
// consumer with a max capacity large enough to absorb the load, asserting every
// message is delivered exactly once and none overflow.
func TestMPSCNoLossWhenSized(t *testing.T) {
	const producers = 16
	const perProducer = 1000
	const total = producers * perProducer

	rb := New[int](32, 1<<20) // grows freely; no overflow expected

	var overflow atomic.Int64
	var wg sync.WaitGroup
	wg.Add(producers)
	for p := 0; p < producers; p++ {
		go func(base int) {
			defer wg.Done()
			for i := 0; i < perProducer; i++ {
				if rb.Push(base*perProducer+i) == PushOverflow {
					overflow.Add(1)
				}
			}
		}(p)
	}

	var received int64
	seen := make([]bool, total)
	consumerDone := make(chan struct{})
	producersDone := make(chan struct{})
	go func() {
		for {
			if v, ok := rb.Pop(); ok {
				if seen[v] {
					t.Errorf("duplicate value %d", v)
				}
				seen[v] = true
				received++
			} else {
				select {
				case <-producersDone:
					// drain remaining, then exit
					for {
						if v, ok := rb.Pop(); ok {
							if seen[v] {
								t.Errorf("duplicate value %d", v)
							}
							seen[v] = true
							received++
						} else {
							close(consumerDone)
							return
						}
					}
				default:
				}
			}
		}
	}()

	wg.Wait()
	close(producersDone)

	select {
	case <-consumerDone:
	case <-time.After(10 * time.Second):
		t.Fatal("consumer did not finish")
	}
	if overflow.Load() != 0 {
		t.Fatalf("unexpected overflow count: %d", overflow.Load())
	}
	if received != total {
		t.Fatalf("received %d want %d", received, total)
	}
}

func BenchmarkPushPop(b *testing.B) {
	rb := New[Item](1024, 1024)
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
