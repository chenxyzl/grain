package grain

import (
	"sync"
	"testing"
	"time"

	"github.com/chenxyzl/grain/al/ringbuffer"
	"github.com/chenxyzl/grain/message"
)

// blockingActor parks in its first Receive until released, so its mailbox fills and overflows.
type blockingActor struct {
	BaseActor
	enter   chan struct{}
	release chan struct{}
	once    sync.Once
}

func (a *blockingActor) Started() {}
func (a *blockingActor) PreStop() {}
func (a *blockingActor) Receive(ctx Context) {
	a.once.Do(func() {
		close(a.enter)
		<-a.release
	})
}

// A PushOverflow must reach the configured DeadLetterHandler with its fields populated.
func TestDeadLetterOnOverflow(t *testing.T) {
	sys := newFakeSys()

	var mu sync.Mutex
	var got []DeadLetter
	sys.cfg = &config{
		askTimeout: 5 * time.Second,
		deadLetterHandler: func(dl DeadLetter) {
			mu.Lock()
			got = append(got, dl)
			mu.Unlock()
		},
	}

	act := &blockingActor{enter: make(chan struct{}), release: make(chan struct{})}
	p := &processorMailBox{
		system:     sys,
		rb:         ringbuffer.New[Context](2, 2), // fixed cap 2: no growth, easy overflow
		procStatus: idle,
		turn:       make(chan struct{}, 1),
		receiver:   act,
	}
	p.tOpts = newOpts(func() IActor { return act })
	p._self = newDirectActorRef("local", "dl", "test", sys)
	p.turn <- struct{}{}
	p.receiver._init(p._self)
	p.receiver._bindTurn(p)
	p.init() // runs the initialize -> Started

	// first real message: the handler blocks inside Receive holding the turn
	p.send(newContext(p._self, nil, &message.Subscribe{EventName: "block"}, sys.nextSnId(), nil))
	select {
	case <-act.enter:
	case <-time.After(2 * time.Second):
		t.Fatal("actor never entered its handler")
	}

	// drainer is parked in the blocked handler: the first sends enqueue, the rest overflow
	const extra = 10
	for i := 0; i < extra; i++ {
		p.send(newContext(p._self, nil, &message.Unsubscribe{EventName: "flood"}, sys.nextSnId(), nil))
	}

	mu.Lock()
	n := len(got)
	var sample DeadLetter
	if n > 0 {
		sample = got[0]
	}
	mu.Unlock()

	if n == 0 {
		t.Fatal("expected at least one dead letter on overflow, got none")
	}
	if sample.Reason != DeadLetterReasonOverflow {
		t.Fatalf("reason=%q want %q", sample.Reason, DeadLetterReasonOverflow)
	}
	if sample.Message == nil || sample.Target == nil {
		t.Fatalf("dead letter missing fields: %+v", sample)
	}

	close(act.release) // let the actor finish
}

// A send to a closed mailbox dead-letters with reason "actor stopped".
func TestDeadLetterOnStopped(t *testing.T) {
	sys := newFakeSys()
	var got []DeadLetter
	sys.cfg = &config{
		askTimeout:        5 * time.Second,
		deadLetterHandler: func(dl DeadLetter) { got = append(got, dl) },
	}
	p := &processorMailBox{
		system:     sys,
		rb:         ringbuffer.New[Context](4, 4),
		procStatus: idle,
		turn:       make(chan struct{}, 1),
		receiver:   &blockingActor{enter: make(chan struct{}), release: make(chan struct{})},
	}
	p.tOpts = newOpts(func() IActor { return p.receiver })
	p._self = newDirectActorRef("local", "dl2", "test", sys)
	p.turn <- struct{}{}
	p.rb.Close() // simulate a stopped mailbox

	p.send(newContext(p._self, nil, &message.Subscribe{EventName: "x"}, sys.nextSnId(), nil))
	if len(got) != 1 || got[0].Reason != DeadLetterReasonStopped {
		t.Fatalf("want one dead letter with reason %q, got %+v", DeadLetterReasonStopped, got)
	}
}

// A panicking handler must not take down the sender goroutine.
func TestDeadLetterHandlerPanicRecovered(t *testing.T) {
	sys := newFakeSys()
	sys.cfg = &config{
		askTimeout:        5 * time.Second,
		deadLetterHandler: func(dl DeadLetter) { panic("boom") },
	}
	p := &processorMailBox{
		system:     sys,
		rb:         ringbuffer.New[Context](4, 4),
		procStatus: idle,
		turn:       make(chan struct{}, 1),
		receiver:   &blockingActor{enter: make(chan struct{}), release: make(chan struct{})},
	}
	p.tOpts = newOpts(func() IActor { return p.receiver })
	p._self = newDirectActorRef("local", "dl3", "test", sys)
	p.turn <- struct{}{}
	p.rb.Close()

	// must not panic
	p.send(newContext(p._self, nil, &message.Subscribe{EventName: "x"}, sys.nextSnId(), nil))
}

// slowPreStopActor blocks inside PreStop until released, holding stop() open.
type slowPreStopActor struct {
	BaseActor
	inPreStop chan struct{}
	release   chan struct{}
	once      sync.Once
}

func (a *slowPreStopActor) Started()            {}
func (a *slowPreStopActor) Receive(ctx Context) {}
func (a *slowPreStopActor) PreStop() {
	a.once.Do(func() {
		close(a.inPreStop)
		<-a.release
	})
}

// Messages sent while PreStop is running must dead-letter, not vanish: procStatus is still
// `running` so the sender gets PushOK, but schedule()'s CAS never fires again, so nothing
// will ever drain them.
func TestDeadLetterOnStopWindow(t *testing.T) {
	sys := newFakeSys()
	var mu sync.Mutex
	var got []DeadLetter
	sys.cfg = &config{
		askTimeout: 5 * time.Second,
		deadLetterHandler: func(dl DeadLetter) {
			mu.Lock()
			got = append(got, dl)
			mu.Unlock()
		},
	}

	act := &slowPreStopActor{inPreStop: make(chan struct{}), release: make(chan struct{})}
	p := newTestProcessor(sys, act, 8)
	p.init()
	time.Sleep(50 * time.Millisecond) // let Started() complete
	p.poison()

	// wait until PreStop is executing: stop() is open and procStatus is still `running`
	select {
	case <-act.inPreStop:
	case <-time.After(3 * time.Second):
		t.Fatal("PreStop never ran")
	}

	const n = 3
	for range n {
		p.send(newContext(p.self(), nil, &message.Subscribe{EventName: "sent-during-stop"}, sys.nextSnId(), sys))
	}
	close(act.release) // let PreStop finish so stop()'s defers run

	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		c := len(got)
		mu.Unlock()
		if c >= n {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("only %d of %d messages sent during the stop window surfaced as "+
				"dead letters — the rest were silently dropped", c, n)
		case <-time.After(10 * time.Millisecond):
		}
	}

	mu.Lock()
	defer mu.Unlock()
	for i, dl := range got {
		if dl.Reason != DeadLetterReasonStopped {
			t.Errorf("dead letter %d: reason = %q, want %q", i, dl.Reason, DeadLetterReasonStopped)
		}
		if dl.Message == nil {
			t.Errorf("dead letter %d: nil Message", i)
		}
	}
}
