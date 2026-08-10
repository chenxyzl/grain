package grain

import (
	"sync"
	"testing"
	"time"

	"github.com/chenxyzl/grain/al/ringbuffer"
	"github.com/chenxyzl/grain/message"
)

// blockingActor never returns from its first Receive until released, so its
// mailbox fills up and subsequent sends overflow once at max capacity.
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

// TestDeadLetterOnOverflow: with init==max==2, filling the mailbox past capacity
// while the actor is blocked in its handler drives PushOverflow, which must be
// routed to the configured DeadLetterHandler with the right fields.
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
	// init==max==2 -> fixed capacity 2, no growth, easy overflow.
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

	// first real message: the handler blocks inside Receive holding the turn.
	p.send(newContext(p._self, nil, &message.Subscribe{EventName: "block"}, sys.nextSnId(), nil))
	select {
	case <-act.enter:
	case <-time.After(2 * time.Second):
		t.Fatal("actor never entered its handler")
	}

	// Now the drainer is parked in the blocked handler. Fill the mailbox (cap 2)
	// and then overflow it. Some of these enqueue, later ones overflow.
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

// TestDeadLetterOnStopped: sending to a closed (stopped) mailbox routes to the
// dead letter with the "actor stopped" reason.
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

// TestDeadLetterHandlerPanicRecovered: a panicking handler must not crash the
// sender goroutine — the panic is recovered and logged.
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
