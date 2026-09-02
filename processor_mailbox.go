package grain

import (
	"sync/atomic"

	"github.com/chenxyzl/grain/al/ringbuffer"
	"github.com/chenxyzl/grain/ghelper"
	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

const (
	idle int32 = iota
	running
	stopped
)

// lifecycle is the actor's Started/PreStop phase. Only read/written while holding the turn, so
// no atomic. Orthogonal to procStatus (scheduler park state) and poisoned (stop-request latch).
type lifecycle int8

const (
	lifeInit     lifecycle = iota // not started yet
	lifeStarting                  // inside start()/Started()
	lifeStarted                   // Started() completed; the only phase Ask is allowed in
	// inside PreStop(); exists so stop() cannot run PreStop twice, which it would if PreStop
	// yields the turn and a successor re-enters doStop while procStatus is still `running`
	lifeStopping
)

type processorMailBox struct {
	tOpts
	system     ISystem
	rb         *ringbuffer.RingBuffer[Context]
	procStatus int32
	poisoned   atomic.Bool
	life       lifecycle // Started/PreStop phase; only touched while holding turn
	// registered records that THIS processor's registerToCluster succeeded, so stop() only ever
	// deletes a lock it holds. Same domain as life: written in start(), read in stop(), on turn.
	registered bool

	// turn is a capacity-1 semaphore (one token = free). Its holder has the exclusive right to run
	// actor code (Started/Receive/PreStop), so the actor is single-threaded even across Ask.
	turn chan struct{}
	// inflight counts handlers in progress, INCLUDING ones suspended in an Ask (stop needs 0).
	inflight atomic.Int32
	// activeDS is the drain state of the turn holder; only accessed while holding the turn.
	activeDS *drainState

	receiver IActor
}

var _ iProcess = (*processorMailBox)(nil)
var _ reentryTurn = (*processorMailBox)(nil)

func newProcessor(system ISystem, opts tOpts) (iProcess, error) {
	return spawnProcessor(system, opts, false)
}

// newProcessorOrGet spawns the processor, or returns the existing one with the same id: internal
// system actors may be spawned concurrently, where a duplicate-id panic would kill the process.
func newProcessorOrGet(system ISystem, opts tOpts) iProcess {
	proc, _ := spawnProcessor(system, opts, true)
	return proc
}

func spawnProcessor(system ISystem, opts tOpts, orGet bool) (iProcess, error) {
	build := func() iProcess {
		p := &processorMailBox{
			tOpts:      opts,
			system:     system,
			rb:         ringbuffer.New[Context](int64(opts.mailboxInitSize), int64(opts.mailboxMaxSize)),
			procStatus: idle,
			turn:       make(chan struct{}, 1),
		}
		p.turn <- struct{}{} // turn starts free
		// Bind the receiver and enqueue initialize BEFORE publishing to the registry: a concurrent
		// get()+send() cannot then see a nil receiver, and Started() runs before any external message.
		p.receiver = p.producer()
		p.receiver._init(p.self())
		p.receiver._bindTurn(p)
		p.rb.Push(newContext(p.self(), p.self(), msgInitialize, system.nextSnId(), system.getSender()))
		return p
	}
	if orGet {
		return system.getRegistry().getOrAdd(opts._self.GetId(), build), nil
	}
	return system.getRegistry().add(build)
}

func (x *processorMailBox) self() ActorRef { return x._self }
func (x *processorMailBox) opts() *tOpts   { return &x.tOpts }

func (x *processorMailBox) acquireTurn() { <-x.turn }
func (x *processorMailBox) releaseTurn() { x.turn <- struct{}{} }

// isStarted reports whether Started() has completed and PreStop() has not begun. Callers hold the
// turn, and life is only written under it, so no atomic. A positive test against lifeStarted, not
// `!= lifeStarting`: askImpl uses it as an allow-list, so a phase added later (lifeStopping) is
// refused by default instead of silently permitting a blocking Ask.
func (x *processorMailBox) isStarted() bool { return x.life == lifeStarted }

// yieldTurn (askImpl, holding the turn, just before blocking on a reply) hands the drainer role to
// a successor so the mailbox keeps draining, releases the turn, and returns ds for resumeTurn.
func (x *processorMailBox) yieldTurn() *drainState {
	ds := x.activeDS
	// No successor during start()/Started(): business messages must not run against half-initialized
	// state. Nothing drains the mailbox then, so the actor cannot answer requests either — hence
	// askImpl refuses an Ask unless isStarted(). The same drainer drains on once Started returns.
	if ds != nil && !ds.handedOff && x.life != lifeStarting {
		// first yield of this drainer: spawn a successor. procStatus stays `running` — the
		// running-owner role passes to it; this goroutine exits once its handler completes.
		ds.handedOff = true
		go x.process()
	}
	x.releaseTurn()
	return ds
}

// resumeTurn reacquires the turn after the reply arrives, so the handler finishes single-threaded.
func (x *processorMailBox) resumeTurn(ds *drainState) {
	x.acquireTurn()
	x.activeDS = ds
}

func (x *processorMailBox) init() {
	// receiver and initialize are set up in build(); this only starts the run loop
	x.schedule()
}

func (x *processorMailBox) send(ctx Context) {
	switch x.rb.Push(ctx) {
	case ringbuffer.PushOK:
		x.schedule()
	case ringbuffer.PushOverflow:
		// full at max capacity: drop rather than block the sender (a→b→a could deadlock)
		x.toDeadLetter(ctx, DeadLetterReasonOverflow)
	case ringbuffer.PushClosed:
		// mailbox closed (actor stopped)
		x.toDeadLetter(ctx, DeadLetterReasonStopped)
	}
}

// toDeadLetter surfaces an undeliverable message to the DeadLetterHandler, or logs WARN when none
// is set. Runs on the sender's goroutine, so a panicking handler is recovered here.
func (x *processorMailBox) toDeadLetter(ctx Context, reason string) {
	// Fail a waiting Ask now: a saturated or stopped mailbox must be as prompt as errActorNotFound.
	if s := ctx.Sender(); s != nil && s.isAsk() {
		s.Tell(message.WithErr("ask target mailbox unavailable: " + reason))
	}
	if h := x.system.getConfig().deadLetterHandler; h != nil {
		defer func() {
			if err := recover(); err != nil {
				x.system.Logger().Error("dead letter handler panic",
					"id", x.self(), "reason", reason, "err", err, "stack", ghelper.StackTrace())
			}
		}()
		h(DeadLetter{
			Target:  ctx.Target(),
			Owner:   x.self(),
			Sender:  ctx.Sender(),
			Message: ctx.Message(),
			MsgSnId: ctx.GetMsgSnId(),
			Reason:  reason,
		})
		return
	}
	x.system.Logger().Warn("dead letter",
		"owner", x.self(), "target", ctx.Target(),
		"msgName", proto.MessageName(ctx.Message()), "reason", reason)
}

// poison requests a stop without enqueuing into the (possibly full) mailbox: it sets a flag and
// wakes the run loop, which drains then stops. Non-blocking, so it is safe to call under locks.
func (x *processorMailBox) poison() {
	x.poisoned.Store(true)
	x.schedule()
}

func (x *processorMailBox) invoke(ctx Context) {
	defer func() {
		if err := recover(); err != nil {
			x.system.Logger().Error("actor receive panic",
				"id", x.self(),
				"msgName", proto.MessageName(ctx.Message()),
				"msg", ctx.Message(),
				"err", err,
				"stack", ghelper.StackTrace())
		}
	}()
	switch ctx.Message().(type) {
	case *message.Initialize:
		x.start()
	case *message.Poison:
		// A Poison arriving as a message must not stop synchronously: another handler may be
		// suspended in a blocking Ask. Via the flag path, stop runs only once inflight==0.
		x.poison()
	default:
		x.receiver.Receive(ctx)
	}
}

func (x *processorMailBox) schedule() {
	if atomic.CompareAndSwapInt32(&x.procStatus, idle, running) {
		go x.process()
	}
}

func (x *processorMailBox) process() {
	ds := &drainState{}
	x.run(ds)
	if ds.handedOff {
		// a successor took over the running-owner role
		return
	}
	// if run() exited because stop() set stopped, leave it stopped
	if !atomic.CompareAndSwapInt32(&x.procStatus, running, idle) {
		return
	}
	// Re-check after parking: a send() re-arms draining. The poison path only needs a wake-up to run
	// doStop (precondition inflight==0); gating on it avoids busy-spinning until a suspended Ask ends.
	if x.rb.Len() > 0 || (x.poisoned.Load() && x.inflight.Load() == 0) {
		x.schedule()
	}
}

func (x *processorMailBox) run(ds *drainState) {
	for atomic.LoadInt32(&x.procStatus) != stopped {
		msg, ok := x.rb.Pop()
		if !ok {
			// drained: stop now if poisoned and nothing is in flight (incl. suspended in Ask)
			if x.poisoned.Load() && x.inflight.Load() == 0 {
				x.doStop()
			}
			return
		}
		x.inflight.Add(1)
		x.acquireTurn()
		x.activeDS = ds
		x.invoke(msg) // may yield/resume the turn internally on a blocking Ask
		x.releaseTurn()
		remaining := x.inflight.Add(-1)
		if ds.handedOff {
			// I yielded, so the successor owns draining. If I was the last in-flight handler and
			// we're poisoned, retrigger so the stop check runs (the successor may have parked).
			if remaining == 0 && x.poisoned.Load() {
				x.schedule()
			}
			return
		}
	}
}

// doStop stops the actor while holding the turn, after re-confirming the mailbox is drained and
// no handler is in flight. Idempotent via the stopped-state check.
func (x *processorMailBox) doStop() {
	x.acquireTurn()
	defer x.releaseTurn()
	if atomic.LoadInt32(&x.procStatus) == stopped {
		return
	}
	if x.rb.Len() != 0 || x.inflight.Load() != 0 {
		// work reappeared before we got the turn; a drainer will handle it and retry stop
		return
	}
	x.stop()
}

func (x *processorMailBox) start() {
	defer func() {
		if err := recover(); err != nil {
			x.system.Logger().Error("spawn recover a panic on start. force to stop self",
				"id", x.self(),
				"err", err,
				"stack", ghelper.StackTrace())
			//force to stop self (life stays lifeStarting, so stop skips PreStop)
			x.stop()
		}
	}()
	if x.tOpts.registerToCluster != nil {
		if err := x.tOpts.registerToCluster(x.system.getProvider(), x.system.getConfig(), x.self()); err != nil {
			// cannot serve its kind: stop it (logged, not panicked). life is still lifeInit so
			// stop() skips PreStop, and registered is false so it won't touch the winner's lock.
			x.system.Logger().Error("register to cluster failed, stop self",
				"id", x.self(), "err", err)
			x.stop()
			return
		}
		x.registered = true
	}
	x.life = lifeStarting
	x.receiver.Started()
	x.life = lifeStarted
}

// stop is always called while holding the turn (from invoke of a Poison message, from doStop,
// or from start()'s failure/recover paths), so it runs single-threaded. Idempotent.
func (x *processorMailBox) stop() {
	if atomic.LoadInt32(&x.procStatus) == stopped {
		return
	}
	defer func() {
		if err := recover(); err != nil {
			x.system.Logger().Error("recover a panic on stop",
				"id", x.self(),
				"err", err,
				"stack", ghelper.StackTrace())
		}
	}()
	defer func() {
		atomic.StoreInt32(&x.procStatus, stopped)
		x.system.getRegistry().remove(x.self())
		// Close the ring so later Push returns PushClosed, then dead-letter what is still queued.
		// The drain is not optional: procStatus stays `running` for all of PreStop, so a sender in
		// that window gets PushOK while schedule()'s CAS(idle→running) fails and nothing drains it.
		// Close() keeps queued items poppable, so they would otherwise vanish with no dead letter.
		x.rb.Close()
		for {
			ctx, ok := x.rb.Pop()
			if !ok {
				break
			}
			x.toDeadLetter(ctx, DeadLetterReasonStopped)
		}
	}()
	defer func() {
		// Gated on `registered`, as PreStop is on lifeStarted: an actor that FAILED to register
		// (single-activation loser, transient etcd error) must not delete the owner's key.
		if x.registered && x.tOpts.unRegisterFromCluster != nil {
			if err := x.tOpts.unRegisterFromCluster(x.system.getProvider(), x.system.getConfig(), x.self()); err != nil {
				// peers keep routing to a stale entry until the lease expires, so make it visible
				x.system.Logger().Warn("unregister from cluster failed, stale routing entry may remain",
					"id", x.self(), "err", err)
			}
		}
	}()
	// PreStop pairs with a completed Started(): skip it when the actor never finished starting, so
	// cleanup doesn't touch resources Started never set up. Advance to lifeStopping BEFORE the call
	// so a re-entrant stop() (PreStop may release the turn) cannot run PreStop twice.
	if x.life == lifeStarted {
		x.life = lifeStopping
		x.receiver.PreStop()
	}
}
