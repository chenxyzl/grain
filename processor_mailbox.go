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

// lifecycle is the actor's Started/PreStop phase. Its values are mutually
// exclusive and it is only ever read/written while holding the turn, so it needs
// no atomic. It is orthogonal to procStatus (scheduler park state) and poisoned
// (stop-requested latch), which live in different concurrency domains.
type lifecycle int8

const (
	lifeInit     lifecycle = iota // not started yet
	lifeStarting                  // inside start()/Started()
	lifeStarted                   // Started() completed; the only phase Ask is allowed in
	// lifeStopping means we are inside PreStop(). It exists so stop() cannot run
	// PreStop twice: if PreStop releases the turn (any blocking, turn-yielding
	// call), a successor drainer can re-enter doStop -> stop() while procStatus is
	// still `running` (it only becomes `stopped` in stop()'s defer, after PreStop
	// returns). The lifeStarted check would pass again and re-run PreStop.
	lifeStopping
)

type processorMailBox struct {
	tOpts
	system     ISystem
	rb         *ringbuffer.RingBuffer[Context]
	procStatus int32
	poisoned   atomic.Bool
	life       lifecycle // Started/PreStop phase; only touched while holding turn
	// registered records that THIS processor's registerToCluster succeeded, so stop()
	// only ever deletes a lock it actually holds. Same concurrency domain as life: written
	// in start() and read in stop(), both while holding the turn.
	registered bool

	// turn is a capacity-1 semaphore (one token = free). Holding a token grants
	// the exclusive right to execute actor code (Started/Receive/PreStop), so the
	// actor is strictly single-threaded even across reentrant Ask.
	turn chan struct{}
	// inflight counts handler executions in progress, INCLUDING handlers
	// suspended inside a blocking Ask. stop() may only run once inflight==0.
	inflight atomic.Int32
	// activeDS is the drain state of the goroutine currently holding the turn;
	// only accessed while holding the turn, so it needs no extra sync.
	activeDS *drainState

	receiver IActor
}

var _ iProcess = (*processorMailBox)(nil)
var _ reentryTurn = (*processorMailBox)(nil)

func newProcessor(system ISystem, opts tOpts) (iProcess, error) {
	return spawnProcessor(system, opts, false)
}

// newProcessorOrGet spawns the processor, or returns the existing one when a
// processor with the same id already exists. Used for internal system actors
// (write_stream / cluster kinds) that may be spawned concurrently by multiple
// senders, where a duplicate-id panic would kill the whole process.
func newProcessorOrGet(system ISystem, opts tOpts) iProcess {
	// getOrAdd is idempotent, so this variant cannot report a duplicate id.
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
		// Bind the receiver and enqueue the initialize message in the constructor,
		// before the processor is published to the registry. This guarantees that
		// a concurrent get()+send() (the write_stream / cluster spawn races) can
		// never observe a nil receiver, and that Started() (triggered by the
		// initialize message at the head of the queue) is always processed before
		// any externally-sent message. init() below only starts the run loop.
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

// isStarted reports whether the actor is in its running phase — Started() has
// completed and PreStop() has not begun. Callers hold the turn (every handler
// does), and life is only written while holding it, so this needs no atomic.
//
// Deliberately a positive test against lifeStarted rather than `!= lifeStarting`:
// askImpl uses it as an allow-list, so a lifecycle phase added later is refused by
// default instead of silently permitting a blocking Ask. lifeStopping is exactly
// such a phase — under the old `!= lifeStarting` form an Ask in PreStop would slip
// through and re-enter stop().
func (x *processorMailBox) isStarted() bool { return x.life == lifeStarted }

// yieldTurn is called on behalf of BaseActor.Ask (from askImpl, while holding the
// turn) right before it blocks on a reply. It hands off the drainer role to a
// fresh successor (so the mailbox keeps draining while this handler is suspended)
// and releases the turn. Returns the caller's drain state for resumeTurn to
// restore.
func (x *processorMailBox) yieldTurn() *drainState {
	ds := x.activeDS
	// During start()/Started(), do NOT hand off to a successor: business messages
	// must not be processed until Started completes, or a handler would run against
	// half-initialized actor state.
	//
	// Because nothing drains the mailbox in that window, the actor cannot answer any
	// incoming request either — which is why askImpl refuses an Ask unless
	// isStarted() (errAskNotRunning), instead of letting it block here and possibly
	// wait out askTimeout. So in practice this branch is only reached via the
	// lower-level yieldTurn path, not through Ask.
	//
	// Once Started returns, the same drainer resumes and drains normally.
	if ds != nil && !ds.handedOff && x.life != lifeStarting {
		// first yield of this drainer: spawn a successor to take over draining.
		// procStatus stays `running` — the running-owner role passes to the
		// successor; this goroutine exits (handedOff) once its handler completes.
		ds.handedOff = true
		go x.process()
	}
	x.releaseTurn()
	return ds
}

// resumeTurn is called on behalf of BaseActor.Ask (from askImpl) after the reply
// arrives; it reacquires the turn so the suspended handler can finish
// single-threaded.
func (x *processorMailBox) resumeTurn(ds *drainState) {
	x.acquireTurn()
	x.activeDS = ds
}

func (x *processorMailBox) init() {
	// receiver binding and the initialize message are set up in build() before
	// the processor is published to the registry; here we only start the run
	// loop, which will process the queued initialize (-> Started) first.
	x.schedule()
}

func (x *processorMailBox) send(ctx Context) {
	switch x.rb.Push(ctx) {
	case ringbuffer.PushOK:
		x.schedule()
	case ringbuffer.PushOverflow:
		// mailbox full at max capacity: drop rather than block the sender (which
		// could deadlock a→b→a). Route to the dead letter.
		x.toDeadLetter(ctx, DeadLetterReasonOverflow)
	case ringbuffer.PushClosed:
		// mailbox closed (actor stopped): route to the dead letter.
		x.toDeadLetter(ctx, DeadLetterReasonStopped)
	}
}

// toDeadLetter surfaces an undeliverable message to the configured
// DeadLetterHandler, or logs it at WARN when none is set. Runs on the sender's
// goroutine, so a panicking handler is recovered here rather than crashing an
// arbitrary sender (consistent with invoke/start/stop).
func (x *processorMailBox) toDeadLetter(ctx Context, reason string) {
	// Fail a waiting Ask immediately. sendToLocal already replies errActorNotFound
	// when the target does not exist, so an Ask into a saturated or stopped mailbox
	// must be equally prompt — it used to get no reply at all and block for the full
	// askTimeout, which made "actor missing" fast and "actor overloaded" slow.
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

// poison requests a stop without enqueuing into the (possibly full) mailbox:
// it sets a flag and wakes the run loop, which drains the remaining messages
// and then stops. Non-blocking and idempotent, so it is safe to call while
// holding registry locks (see system_life.stopActorsImpl).
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
		// A Poison delivered as a message (e.g. from a remote node) must not stop
		// synchronously here: another handler may be suspended in a blocking Ask
		// (inflight>0). Route it through the flag path so stop only runs once the
		// mailbox is drained and inflight==0, exactly like a local poison().
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
		// a successor took over the running-owner role; just exit.
		return
	}
	// If run() exited because stop() set the status to stopped, leave it stopped.
	if !atomic.CompareAndSwapInt32(&x.procStatus, running, idle) {
		return
	}
	// Re-check after going idle: a send() that pushed re-arms draining. For the
	// poison path we only need to wake up to run doStop, whose precondition is
	// inflight==0 — otherwise we'd busy-spin (schedule -> Pop empty -> can't stop
	// -> reschedule) until a suspended Ask returns. When inflight>0, the handler
	// that finishes last re-schedules from run() (see the remaining==0 branch).
	if x.rb.Len() > 0 || (x.poisoned.Load() && x.inflight.Load() == 0) {
		x.schedule()
	}
}

func (x *processorMailBox) run(ds *drainState) {
	for atomic.LoadInt32(&x.procStatus) != stopped {
		msg, ok := x.rb.Pop()
		if !ok {
			// mailbox drained: if a poison was requested and no handler is still
			// in flight (including ones suspended in Ask), stop now.
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
			// I yielded during this handler; the successor owns draining now. If I
			// was the last in-flight handler and we're poisoned, retrigger so the
			// stop check runs (the successor may already have parked).
			if remaining == 0 && x.poisoned.Load() {
				x.schedule()
			}
			return
		}
	}
}

// doStop stops the actor while holding the turn, after re-confirming the mailbox
// is drained and no handler is in flight. Called from the run loop on the poison
// path. Idempotent via the stopped-state check.
func (x *processorMailBox) doStop() {
	x.acquireTurn()
	defer x.releaseTurn()
	if atomic.LoadInt32(&x.procStatus) == stopped {
		return
	}
	if x.rb.Len() != 0 || x.inflight.Load() != 0 {
		// work reappeared between the run-loop check and acquiring the turn; a
		// drainer (spawned by the send/retrigger) will handle it and retry stop.
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
			// cluster registration failed: this actor cannot serve its kind, so
			// stop it (logged, not panicked — a runtime failure, not a crash).
			// life is still lifeInit, so stop() will skip PreStop, and registered is
			// still false, so stop() will not touch the lock the winner holds.
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

// stop is always called while holding the turn (from the run loop's invoke of a
// Poison message, from doStop, or from start()'s failure/recover paths), so it
// runs single-threaded against the actor. It is idempotent.
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
	//send stop to actor
	defer func() {
		//stop run
		atomic.StoreInt32(&x.procStatus, stopped)
		//remove from registry
		x.system.getRegistry().remove(x.self())
		// Close the ring so any later Push returns PushClosed and send() dead-letters
		// it, then dead-letter whatever is still queued.
		//
		// The drain is not optional. doStop only verifies the mailbox is empty BEFORE
		// PreStop runs, and procStatus stays `running` for the whole of PreStop (it
		// only becomes `stopped` here). In that window — which lasts as long as user
		// PreStop code does — a sender gets PushOK but schedule()'s CAS(idle→running)
		// fails, so nothing ever drains those messages. Close() keeps queued items
		// poppable rather than rejecting them, so without this loop they were dropped
		// with no dead letter and no log at all.
		x.rb.Close()
		for {
			ctx, ok := x.rb.Pop()
			if !ok {
				break
			}
			x.toDeadLetter(ctx, DeadLetterReasonStopped)
		}
	}()
	//unregister from cluster
	defer func() {
		// Gated on `registered`, exactly as PreStop is gated on lifeStarted: this used to
		// run unconditionally, so an actor that FAILED to register — the loser of the
		// single-activation race, or any actor hit by a transient etcd error — still went
		// on to delete the registration key. Paired with the old empty-string lock value
		// that deleted the actual owner's lock and let the same grain activate twice. Both
		// halves are fixed; this one also stops the loser from emitting a bogus
		// "unregister failed" WARN for a lock it never held.
		if x.registered && x.tOpts.unRegisterFromCluster != nil {
			if err := x.tOpts.unRegisterFromCluster(x.system.getProvider(), x.system.getConfig(), x.self()); err != nil {
				// Not fatal for this actor, but peers will keep routing to a stale
				// cluster entry until the lease expires, so it must be visible.
				x.system.Logger().Warn("unregister from cluster failed, stale routing entry may remain",
					"id", x.self(), "err", err)
			}
		}
	}()
	// PreStop pairs with a completed Started(): skip it when the actor never
	// finished starting (e.g. cluster-register failure, or Started panicked),
	// so cleanup code doesn't touch resources Started was supposed to set up.
	//
	// Advance to lifeStopping BEFORE the call, so a re-entrant stop() (possible when
	// PreStop releases the turn — see lifeStopping) finds lifeStarted false and does
	// not run PreStop a second time. It also takes the actor out of the Ask-allowed
	// phase for the duration of PreStop.
	if x.life == lifeStarted {
		x.life = lifeStopping
		x.receiver.PreStop()
	}
}
