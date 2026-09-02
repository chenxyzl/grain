package grain

import (
	"errors"
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

// fakeSys is a minimal ISystem for a processorMailBox without etcd; unimplemented methods panic.
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
		reg:     newRegistry(),
		cfg:     &config{askTimeout: 5 * time.Second}, // empty Kinds => register/unregister no-op
		logger:  slog.Default(),
		pending: safemap.NewIntC[uint64, chan proto.Message](),
	}
}

func (f *fakeSys) getRegistry() iRegistry { return f.reg }
func (f *fakeSys) getConfig() *config     { return f.cfg }
func (f *fakeSys) Logger() *slog.Logger   { return f.logger }
func (f *fakeSys) getProvider() iProvider { return nil }
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

// tell/tellWithSender: local-only router so real Ask and ctx.Reply work in-process.
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

// newTestProcessor mirrors spawnProcessor's constructor without the registry publish dance. The
// mailbox can grow, like production, so a self-Ask never overflows for want of a single slot.
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
	// mirror build(): enqueue initialize so Started() runs and PreStop pairs on stop
	p.rb.Push(newContext(self, self, msgInitialize, sys.nextSnId(), nil))
	sys.reg.lookup.Set(self.GetId(), p)
	return p
}

// reentryActor yields the turn on a block message and waits on a channel: a fake blocking Ask.
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

// enter/leave bracket the window in which this handler HOLDS the turn; the yield window is
// excluded, since a successor legitimately runs there. maxConc must therefore never exceed 1.
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
		// mimic BaseActor.Ask: do not hold the turn while blocked
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

// TestReentrancyGeneral: a message enqueued while a handler is blocked runs, and no two ever do.
func TestReentrancyGeneral(t *testing.T) {
	sys := newFakeSys()
	act := &reentryActor{
		block:     make(chan struct{}),
		blockedAt: make(chan string, 4),
		processed: make(chan string, 8),
	}
	p := newTestProcessor(sys, act, 64)
	p.init() // starts run loop; processes initialize -> Started

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

	close(act.block)
	got = drainProcessed(act.processed, 1, time.Second)
	if len(got) != 1 || got[0] != "blocker" {
		t.Fatalf("blocked msg did not complete, got=%v", got)
	}

	if mc := act.maxConc.Load(); mc > 1 {
		t.Fatalf("actor was not single-threaded, maxConcurrent=%d", mc)
	}
}

// TestReentrancyStopDrains: poison during a blocked handler stops only once inflight is 0.
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

// TestReentrancyNested: several handlers yield at once; all complete and the scheduler reparks.
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

// TestStartedBlockingAskHoldsBusinessMessages: a blocking Ask inside Started() must not let
// queued business messages run before Started completes (no successor handoff while starting).
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

// TestRemotePoisonWaitsInflight: a Poison arriving as a message stops only after inflight drains.
func TestRemotePoisonWaitsInflight(t *testing.T) {
	sys := newFakeSys()
	act := &reentryActor{
		block:     make(chan struct{}),
		blockedAt: make(chan string, 4),
		processed: make(chan string, 8),
	}
	p := newTestProcessor(sys, act, 64)
	p.init()

	p.send(newContext(p.self(), nil, &message.Subscribe{EventName: "blocker"}, sys.nextSnId(), nil))
	<-act.blockedAt

	// Deliver Poison as a normal message (remote path) while the handler is blocked.
	p.send(newContext(p.self(), nil, msgPoison, sys.nextSnId(), nil))

	time.Sleep(100 * time.Millisecond)
	if act.stopped.Load() {
		t.Fatal("actor stopped (PreStop ran) while a handler was suspended in Ask")
	}

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

// selfAskActor fills its own mailbox, then issues a real Ask to ITSELF. Only a successor drainer
// can serve that request, so yield-before-send is what keeps it from deadlocking.
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
			_, err := a.Ask[*message.Unsubscribe](a.Self(), &message.Subscribe{EventName: "selfask"})
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

// TestSelfAskDoesNotDeadlock: a self-Ask with a full mailbox must still complete.
func TestSelfAskDoesNotDeadlock(t *testing.T) {
	sys := newFakeSys()
	act := &selfAskActor{mailboxCap: 8, done: make(chan struct{})}
	p := newTestProcessor(sys, act, 8) // small mailbox so it fills easily
	p.init()

	p.send(newContext(p.self(), nil, &message.Subscribe{EventName: "go"}, sys.nextSnId(), nil))

	select {
	case <-act.done:
		// returned without deadlock
	case <-time.After(3 * time.Second):
		t.Fatal("self-ask deadlocked: Ask never returned")
	}
}

// TestAskTimeoutAndLateReply: Ask times out to an ErrCode; a late reply is dropped, not a panic.
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

	// late reply for a cancelled snId: Pop misses -> dropped, must not panic or block
	sys.deliverReply(snId, &message.Unsubscribe{EventName: "late"})
}

// TestWakePendingAsks: shutdown poisons a waiting Ask so it returns without waiting askTimeout.
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
		case c <- msgPoison:
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

// TestAskReplyDelivered: a reply for the snId is delivered and decoded into the typed result.
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

// startedAskActor self-asks from Started() instead of from a normal handler.
type startedAskActor struct {
	BaseActor
	out    chan *message.ErrCode
	served atomic.Bool // set if the request ever reached the target
}

func (a *startedAskActor) Started() {
	_, err := a.Ask[*message.Unsubscribe](a.Self(), &message.Subscribe{EventName: "req"})
	a.out <- err
}
func (a *startedAskActor) PreStop() {}
func (a *startedAskActor) Receive(ctx Context) {
	if m, ok := ctx.Message().(*message.Subscribe); ok && m.EventName == "req" {
		a.served.Store(true)
		ctx.Reply(&message.Unsubscribe{EventName: "reply"})
	}
}

// TestAskFromStartedIsRejected (docs/reentrancy.md §九): an Ask from Started() is refused at the
// call site with CodeAskNotRunning and nothing is sent. Reentrancy is off during Started(), so
// the actor cannot answer requests then and such an Ask would just wait out askTimeout.
func TestAskFromStartedIsRejected(t *testing.T) {
	sys := newFakeSys()
	// generous: a correct rejection never waits, so ~askTimeout here means the Ask went out
	sys.cfg.askTimeout = 30 * time.Second
	act := &startedAskActor{out: make(chan *message.ErrCode, 1)}
	p := newTestProcessor(sys, act, 8)
	p.init()

	select {
	case err := <-act.out:
		if err == nil {
			t.Fatal("an Ask from Started() must be rejected, but it succeeded")
		}
		if !errors.Is(err, message.CodeAskNotRunning) {
			t.Errorf("want CodeAskNotRunning (%d), got code %d: %q",
				message.CodeAskNotRunning, err.Code, err.Des)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Ask from Started() neither returned nor was rejected — it blocked, " +
			"so the rejection is not in effect")
	}

	// the request must never have been sent, before or after Started() returned
	time.Sleep(50 * time.Millisecond)
	if act.served.Load() {
		t.Error("the rejected Ask still delivered its request to the target")
	}
}

// preStopAskActor attempts a real (turn-yielding) Ask from PreStop() and counts PreStop runs.
type preStopAskActor struct {
	BaseActor
	sys      *fakeSys
	out      chan *message.ErrCode
	preStops atomic.Int32
}

func (a *preStopAskActor) Started()            {}
func (a *preStopAskActor) Receive(ctx Context) {}
func (a *preStopAskActor) PreStop() {
	if n := a.preStops.Add(1); n > 1 {
		return // never recurse; the count is what the test asserts
	}
	// fakeSys drops a ghost target, so an unrefused Ask would yield the turn and block askTimeout
	ghost := newDirectActorRef("local", "ghost", "test", a.sys)
	_, err := a.Ask[*message.Unsubscribe](ghost, &message.Subscribe{EventName: "x"})
	a.out <- err
}

// TestAskFromPreStopIsRejected pins two coupled rules: PreStop() is outside the Ask-allowed phase
// (stop() sets lifeStopping first, so isStarted() refuses with CodeAskNotRunning), and PreStop
// runs exactly once — a yielding call there lets a successor re-enter stop() while it runs.
func TestAskFromPreStopIsRejected(t *testing.T) {
	sys := newFakeSys()
	sys.cfg.askTimeout = 200 * time.Millisecond // short, so a regression shows as a delay not a hang
	act := &preStopAskActor{sys: sys, out: make(chan *message.ErrCode, 1)}
	p := newTestProcessor(sys, act, 8)
	p.init()
	time.Sleep(50 * time.Millisecond) // let Started() complete
	p.poison()

	select {
	case err := <-act.out:
		if err == nil {
			t.Fatal("an Ask from PreStop() must be rejected, but it succeeded")
		}
		if !errors.Is(err, message.CodeAskNotRunning) {
			t.Errorf("want CodeAskNotRunning (%d), got code %d: %q",
				message.CodeAskNotRunning, err.Code, err.Des)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("PreStop() never ran")
	}

	// Give any wrongly-spawned successor drainer time to re-enter stop().
	time.Sleep(300 * time.Millisecond)
	if n := act.preStops.Load(); n != 1 {
		t.Errorf("PreStop ran %d times, want exactly 1 (lifeStopping guard is not holding)", n)
	}
}

// preStopYieldActor releases the turn from PreStop() directly, bypassing Ask — the remaining
// path that can re-enter stop(), so it is what guards lifeStopping.
type preStopYieldActor struct {
	BaseActor
	turnCtl  reentryTurn
	release  chan struct{}
	preStops atomic.Int32
}

func (a *preStopYieldActor) _bindTurn(t reentryTurn) { a.BaseActor._bindTurn(t); a.turnCtl = t }
func (a *preStopYieldActor) Started()                {}
func (a *preStopYieldActor) Receive(ctx Context)     {}
func (a *preStopYieldActor) PreStop() {
	if n := a.preStops.Add(1); n > 1 {
		return
	}
	// hand the turn to a successor drainer
	ds := a.turnCtl.yieldTurn()
	<-a.release
	a.turnCtl.resumeTurn(ds)
}

// TestPreStopRunsOnceWhenItYieldsTheTurn: a successor re-entering stop() must not re-run PreStop.
func TestPreStopRunsOnceWhenItYieldsTheTurn(t *testing.T) {
	sys := newFakeSys()
	act := &preStopYieldActor{release: make(chan struct{})}
	p := newTestProcessor(sys, act, 8)
	p.init()
	time.Sleep(50 * time.Millisecond)
	p.poison()

	// Wait for PreStop to have yielded, let a successor try to re-enter stop().
	time.Sleep(300 * time.Millisecond)
	if n := act.preStops.Load(); n != 1 {
		t.Errorf("PreStop ran %d times while the turn was yielded, want exactly 1", n)
	}
	close(act.release)
	time.Sleep(200 * time.Millisecond)
	if n := act.preStops.Load(); n != 1 {
		t.Errorf("PreStop ran %d times after resume, want exactly 1", n)
	}
}
