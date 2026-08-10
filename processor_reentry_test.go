package grain

import (
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/chenxyzl/grain/al/ringbuffer"
	"github.com/chenxyzl/grain/al/safemap"
	"github.com/chenxyzl/grain/message"
	"github.com/chenxyzl/grain/uuid"
)

// fakeSys is a minimal ISystem for driving a processorMailBox without etcd.
// Only the methods the scheduler actually calls are implemented; the rest are
// inherited from the embedded nil ISystem and will panic if unexpectedly used.
type fakeSys struct {
	ISystem
	reg     *registry
	cfg     *config
	snId    atomic.Uint64
	logger  *slog.Logger
	pending safemap.ConcurrentMap[uint64, chan proto.Message]
}

func newFakeSys() *fakeSys {
	_ = uuid.Init(1) // reply-processor ids use the global uuid generator
	return &fakeSys{
		reg:     newRegistry(slog.Default()),
		cfg:     &config{askTimeout: 5 * time.Second}, // empty Kinds => register/unregister no-op
		logger:  slog.Default(),
		pending: safemap.NewIntC[uint64, chan proto.Message](),
	}
}

func (f *fakeSys) getRegistry() iRegistry { return f.reg }
func (f *fakeSys) getConfig() *config     { return f.cfg }
func (f *fakeSys) Logger() *slog.Logger   { return f.logger }
func (f *fakeSys) GetProvider() iProvider { return nil }
func (f *fakeSys) nextSnId() uint64       { return f.snId.Add(1) }
func (f *fakeSys) getSender() iSender     { return f }
func (f *fakeSys) getAddr() string        { return "test" }

func (f *fakeSys) registerAsk(snId uint64) chan proto.Message {
	ch := make(chan proto.Message, 1)
	f.pending.Set(snId, ch)
	return ch
}
func (f *fakeSys) cancelAsk(snId uint64) { f.pending.Remove(snId) }
func (f *fakeSys) deliverReply(snId uint64, msg proto.Message) {
	if ch, ok := f.pending.Pop(snId); ok {
		ch <- msg
	}
}

// tell / tellWithSender: minimal local-only router so real BaseActor.Ask (and
// ctx.Reply) work in-process without etcd/grpc. Looks the target up in the
// registry and delivers via proc.send; unknown targets are dropped.
func (f *fakeSys) tell(target ActorRef, msg proto.Message) {
	f.tellWithSender(target, msg, nil, f.nextSnId())
}
func (f *fakeSys) tellWithSender(target ActorRef, msg proto.Message, sender ActorRef, msgSnId uint64) {
	// mirror system.sendToLocal: reply targets go to the pending table by snId.
	if target.isAsk() {
		f.deliverReply(target.askSnId(), msg)
		return
	}
	if proc := f.reg.get(target); proc != nil {
		proc.send(newContext(proc.self(), sender, msg, msgSnId, f))
	}
}

// newTestProcessor builds a processorMailBox wired to a fakeSys, mirroring
// spawnProcessor's constructor but without the registry publish dance. The
// mailbox starts at `mailbox` and can grow (like production), so a self-Ask
// never overflows for want of a single slot.
func newTestProcessor(sys *fakeSys, r IActor, mailbox int) *processorMailBox {
	self := newDirectActorRef("local", "t", "test", sys)
	p := &processorMailBox{
		system:     sys,
		rb:         ringbuffer.New[Context](int64(mailbox), int64(mailbox)*1024),
		procStatus: idle,
		turn:       make(chan struct{}, 1),
		receiver:   r,
	}
	p.tOpts = newOpts(func() IActor { return r })
	p._self = self
	p.turn <- struct{}{}
	p.receiver._init(self)
	p.receiver._bindTurn(p)
	// mirror build(): enqueue initialize so Started() runs (started=true) and
	// PreStop pairs correctly on stop.
	p.rb.Push(newContext(self, self, initialize, sys.nextSnId(), nil))
	sys.reg.lookup.Set(self.GetId(), p)
	return p
}

// reentryActor: on msgBlock it yields the turn and waits on a caller-controlled
// channel (simulating a blocking Ask); every Receive bumps a concurrency guard
// to assert strict single-threading.
type reentryActor struct {
	BaseActor
	turnCtl     reentryTurn
	block       chan struct{} // test releases this to unblock the "Ask"
	blockedAt   chan string   // signals which msg is currently blocked
	processed   chan string   // signals each completed (non-blocking) msg
	concurrent  atomic.Int32
	maxConc     atomic.Int32
	stopped     atomic.Bool
	blockStart  bool        // if true, Started() yields+blocks like an Ask
	startedDone atomic.Bool // set true after Started() returns
}

func (a *reentryActor) _bindTurn(t reentryTurn) { a.BaseActor._bindTurn(t); a.turnCtl = t }
func (a *reentryActor) Started() {
	if a.blockStart {
		// simulate a blocking Ask issued from Started()
		a.blockedAt <- "started"
		ds := a.turnCtl.yieldTurn()
		<-a.block
		a.turnCtl.resumeTurn(ds)
	}
	a.startedDone.Store(true)
}
func (a *reentryActor) PreStop() { a.stopped.Store(true) }

// enter/leave bracket the window in which this handler HOLDS the turn. The
// yield window (between yieldTurn and resumeTurn) is explicitly excluded, since
// the handler does not hold the turn there — that is exactly when a successor
// legitimately runs. maxConc must therefore never exceed 1.
func (a *reentryActor) enter() {
	n := a.concurrent.Add(1)
	for {
		m := a.maxConc.Load()
		if n <= m || a.maxConc.CompareAndSwap(m, n) {
			break
		}
	}
}
func (a *reentryActor) leave() { a.concurrent.Add(-1) }

func (a *reentryActor) Receive(ctx Context) {
	a.enter()
	defer a.leave()

	switch m := ctx.Message().(type) {
	case *message.Subscribe: // "block" message: simulate a blocking Ask
		a.blockedAt <- m.EventName
		// mimic BaseActor.Ask's yield/resume around a blocking wait: not holding
		// the turn while blocked.
		a.leave()
		ds := a.turnCtl.yieldTurn()
		<-a.block
		a.turnCtl.resumeTurn(ds)
		a.enter()
		a.processed <- m.EventName
	case *message.Unsubscribe: // normal message
		a.processed <- m.EventName
	}
}

func drainProcessed(ch chan string, n int, d time.Duration) []string {
	var got []string
	to := time.After(d)
	for len(got) < n {
		select {
		case s := <-ch:
			got = append(got, s)
		case <-to:
			return got
		}
	}
	return got
}

// TestReentrancyGeneral verifies that while a handler is blocked (yielded turn),
// a later-enqueued unrelated message is processed, and that no two handlers ever
// run concurrently (strict single-threading).
func TestReentrancyGeneral(t *testing.T) {
	sys := newFakeSys()
	act := &reentryActor{
		block:     make(chan struct{}),
		blockedAt: make(chan string, 4),
		processed: make(chan string, 8),
	}
	p := newTestProcessor(sys, act, 64)
	p.init() // starts run loop; processes initialize -> Started

	// msg1 blocks (yields turn)
	p.send(newContext(p.self(), nil, &message.Subscribe{EventName: "blocker"}, sys.nextSnId(), nil))
	select {
	case got := <-act.blockedAt:
		if got != "blocker" {
			t.Fatalf("unexpected blocked msg: %s", got)
		}
	case <-time.After(time.Second):
		t.Fatal("msg1 never started")
	}

	// msg2 (unrelated) enqueued while msg1 is blocked; must be processed now.
	p.send(newContext(p.self(), nil, &message.Unsubscribe{EventName: "other"}, sys.nextSnId(), nil))
	got := drainProcessed(act.processed, 1, time.Second)
	if len(got) != 1 || got[0] != "other" {
		t.Fatalf("reentrant msg not processed during block, got=%v", got)
	}

	// unblock msg1
	close(act.block)
	got = drainProcessed(act.processed, 1, time.Second)
	if len(got) != 1 || got[0] != "blocker" {
		t.Fatalf("blocked msg did not complete, got=%v", got)
	}

	if mc := act.maxConc.Load(); mc > 1 {
		t.Fatalf("actor was not single-threaded, maxConcurrent=%d", mc)
	}
}

// TestReentrancyStopDrains verifies poison during a blocked handler eventually
// stops the actor after the handler resumes (inflight returns to 0).
func TestReentrancyStopDrains(t *testing.T) {
	sys := newFakeSys()
	act := &reentryActor{
		block:     make(chan struct{}),
		blockedAt: make(chan string, 4),
		processed: make(chan string, 8),
	}
	p := newTestProcessor(sys, act, 64)
	p.init()

	p.send(newContext(p.self(), nil, &message.Subscribe{EventName: "blocker"}, sys.nextSnId(), nil))
	<-act.blockedAt // handler is blocked, turn yielded, inflight=1

	p.poison() // request stop while a handler is in flight
	// stop must NOT happen yet (handler still in flight)
	time.Sleep(50 * time.Millisecond)
	if act.stopped.Load() {
		t.Fatal("actor stopped while a handler was still in flight")
	}

	close(act.block) // handler resumes and returns -> inflight 0 -> stop
	deadline := time.After(2 * time.Second)
	for !act.stopped.Load() {
		select {
		case <-deadline:
			t.Fatal("actor did not stop after in-flight handler finished")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// TestReentrancyNested drives multiple messages that each block (yield) at once,
// then releases them, verifying: all complete, strict single-turn holds, and the
// scheduler parks cleanly afterward (no stuck/duplicate drainer) by processing a
// final normal message.
func TestReentrancyNested(t *testing.T) {
	sys := newFakeSys()
	act := &reentryActor{
		block:     make(chan struct{}),
		blockedAt: make(chan string, 16),
		processed: make(chan string, 16),
	}
	p := newTestProcessor(sys, act, 64)
	p.init()

	const n = 5
	for i := 0; i < n; i++ {
		p.send(newContext(p.self(), nil, &message.Subscribe{EventName: "b"}, sys.nextSnId(), nil))
	}
	// all n handlers should reach the blocked (yielded) state
	for i := 0; i < n; i++ {
		select {
		case <-act.blockedAt:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d/%d handlers blocked", i, n)
		}
	}
	close(act.block) // release all
	if got := drainProcessed(act.processed, n, 2*time.Second); len(got) != n {
		t.Fatalf("expected %d completions, got %d", n, len(got))
	}
	if mc := act.maxConc.Load(); mc > 1 {
		t.Fatalf("not single-threaded, maxConcurrent=%d", mc)
	}

	// scheduler must still be alive and parked: a fresh normal message processes.
	p.send(newContext(p.self(), nil, &message.Unsubscribe{EventName: "final"}, sys.nextSnId(), nil))
	if got := drainProcessed(act.processed, 1, 2*time.Second); len(got) != 1 || got[0] != "final" {
		t.Fatalf("scheduler not alive after nested reentry, got=%v", got)
	}
}

// TestStartedBlockingAskHoldsBusinessMessages verifies that a blocking Ask
// inside Started() does NOT let queued business messages run before Started
// completes (#5): starting=true suppresses the successor handoff.
func TestStartedBlockingAskHoldsBusinessMessages(t *testing.T) {
	sys := newFakeSys()
	act := &reentryActor{
		block:      make(chan struct{}),
		blockedAt:  make(chan string, 4),
		processed:  make(chan string, 8),
		blockStart: true,
	}
	p := newTestProcessor(sys, act, 64)
	p.init()

	// Started() blocks (yields turn). A business message is enqueued now.
	<-act.blockedAt // "started"
	p.send(newContext(p.self(), nil, &message.Unsubscribe{EventName: "early"}, sys.nextSnId(), nil))

	// The business message must NOT be processed while Started is still blocked.
	select {
	case got := <-act.processed:
		t.Fatalf("business message %q processed before Started() completed", got)
	case <-time.After(150 * time.Millisecond):
	}
	if act.startedDone.Load() {
		t.Fatal("Started() reported done while still blocked")
	}

	// Let Started() finish; only then may the business message run.
	close(act.block)
	if got := drainProcessed(act.processed, 1, 2*time.Second); len(got) != 1 || got[0] != "early" {
		t.Fatalf("business message not processed after Started(), got=%v", got)
	}
	if !act.startedDone.Load() {
		t.Fatal("Started() not marked done")
	}
	if mc := act.maxConc.Load(); mc > 1 {
		t.Fatalf("not single-threaded, maxConcurrent=%d", mc)
	}
}

// TestRemotePoisonWaitsInflight verifies that a Poison delivered as a message
// (remote path, via invoke) does not stop the actor while a handler is
// suspended in a blocking Ask (#3); PreStop runs only after inflight drains.
func TestRemotePoisonWaitsInflight(t *testing.T) {
	sys := newFakeSys()
	act := &reentryActor{
		block:     make(chan struct{}),
		blockedAt: make(chan string, 4),
		processed: make(chan string, 8),
	}
	p := newTestProcessor(sys, act, 64)
	p.init()

	// A handler blocks in Ask (inflight=1, turn yielded).
	p.send(newContext(p.self(), nil, &message.Subscribe{EventName: "blocker"}, sys.nextSnId(), nil))
	<-act.blockedAt

	// Deliver Poison as a normal message (remote path) while the handler is blocked.
	p.send(newContext(p.self(), nil, poison, sys.nextSnId(), nil))

	time.Sleep(100 * time.Millisecond)
	if act.stopped.Load() {
		t.Fatal("actor stopped (PreStop ran) while a handler was suspended in Ask")
	}

	// Release the handler; now stop may proceed.
	close(act.block)
	deadline := time.After(2 * time.Second)
	for !act.stopped.Load() {
		select {
		case <-deadline:
			t.Fatal("actor did not stop after in-flight handler finished")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}


// selfAskActor: on the trigger message it enqueues some self-messages, then
// issues a real BaseActor.Ask to ITSELF. The self-ask request can only be
// processed by a drainer; with the send-before-yield ordering this goroutine
// would block in awaitReply while still holding the turn, and no successor would
// exist to drain the request -> deadlock. With yield-before-send, a successor
// drainer runs the request and replies, so the self-ask completes.
type selfAskActor struct {
	BaseActor
	mailboxCap int
	done       chan struct{} // closed once, when the self-ask returns
	doneOnce   atomic.Bool
}

func (a *selfAskActor) Started() {}
func (a *selfAskActor) PreStop() {}
func (a *selfAskActor) Receive(ctx Context) {
	switch m := ctx.Message().(type) {
	case *message.Subscribe:
		switch m.EventName {
		case "go": // trigger: enqueue self-messages, then self-ask
			for i := 0; i < a.mailboxCap; i++ {
				a.Self().Tell(&message.Unsubscribe{EventName: "filler"})
			}
			// real self-ask: only a successor drainer can process this request and
			// reply; yield-before-send guarantees that successor exists.
			_, err := a.Ask(a.Self(), &message.Subscribe{EventName: "selfask"})
			_ = err
			if a.doneOnce.CompareAndSwap(false, true) {
				close(a.done)
			}
		case "selfask": // the self-ask request: reply so Ask returns a value
			ctx.Reply(&message.Unsubscribe{EventName: "reply"})
		}
	case *message.Unsubscribe:
		// filler / reply: no-op (draining a slot is the point)
	}
}

// TestSelfAskDoesNotDeadlock reproduces the self-ask + full-mailbox deadlock and
// verifies the yield-before-send ordering resolves it.
func TestSelfAskDoesNotDeadlock(t *testing.T) {
	sys := newFakeSys()
	act := &selfAskActor{mailboxCap: 8, done: make(chan struct{})}
	p := newTestProcessor(sys, act, 8) // small mailbox so it fills easily
	p.init()

	p.send(newContext(p.self(), nil, &message.Subscribe{EventName: "go"}, sys.nextSnId(), nil))

	select {
	case <-act.done:
		// self-ask returned without deadlock — success.
	case <-time.After(3 * time.Second):
		t.Fatal("self-ask deadlocked: Ask never returned")
	}
}

// TestAskTimeoutAndLateReply verifies: an Ask times out to an ErrCode when no
// reply arrives, and a reply that arrives AFTER the timeout (pending entry
// already cancelled) is dropped without panicking or blocking.
func TestAskTimeoutAndLateReply(t *testing.T) {
	sys := newFakeSys()
	sys.cfg = &config{askTimeout: 100 * time.Millisecond}

	snId := sys.nextSnId()
	ch := sys.registerAsk(snId)
	// no reply sent -> should time out
	_, err := awaitReply[*message.Unsubscribe](ch, sys.cfg.askTimeout)
	if err == nil {
		t.Fatal("expected timeout ErrCode, got nil")
	}
	sys.cancelAsk(snId) // caller cleans up (as Ask's defer does)

	// a late reply for the same snId: pending entry is gone -> Pop misses -> drop.
	// must not panic or block.
	sys.deliverReply(snId, &message.Unsubscribe{EventName: "late"})
}

// TestWakePendingAsks verifies shutdown delivers poison to a waiting Ask so it
// returns immediately instead of waiting out askTimeout.
func TestWakePendingAsks(t *testing.T) {
	sys := newFakeSys()
	snId := sys.nextSnId()
	ch := sys.registerAsk(snId)

	got := make(chan *message.ErrCode, 1)
	go func() {
		_, err := awaitReply[*message.Unsubscribe](ch, 10*time.Second)
		got <- err
	}()
	time.Sleep(20 * time.Millisecond) // ensure the goroutine is blocked in select

	// simulate shutdown wakeup
	sys.pending.IterCb(func(_ uint64, c chan proto.Message) {
		select {
		case c <- poison:
		default:
		}
	})

	select {
	case err := <-got:
		if err == nil {
			t.Fatal("expected poisoned ErrCode on shutdown wakeup")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ask not woken by shutdown (would have waited askTimeout)")
	}
}

// TestAskReplyDelivered verifies the happy path: a reply for the snId is
// delivered and decoded into the typed result.
func TestAskReplyDelivered(t *testing.T) {
	sys := newFakeSys()
	snId := sys.nextSnId()
	ch := sys.registerAsk(snId)

	go func() {
		time.Sleep(10 * time.Millisecond)
		sys.deliverReply(snId, &message.Unsubscribe{EventName: "ok"})
	}()

	v, err := awaitReply[*message.Unsubscribe](ch, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v == nil || v.EventName != "ok" {
		t.Fatalf("wrong reply: %+v", v)
	}
	sys.cancelAsk(snId)
}
