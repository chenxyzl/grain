package grain

import (
	"runtime"
	"sync/atomic"

	"github.com/chenxyzl/grain/al/ringbuffer"
	"github.com/chenxyzl/grain/ghelper"
	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

const (
	defaultThroughput = 10
)

const (
	idle int32 = iota
	running
	stopped
)

type processorMailBox struct {
	tOpts
	system     ISystem
	rb         *ringbuffer.RingBuffer[Context]
	procStatus int32
	poisoned   atomic.Bool
	started    bool // set after Started() runs; only touched on the run goroutine
	receiver   IActor
}

var _ iProcess = (*processorMailBox)(nil)

func newProcessor(system ISystem, opts tOpts) iProcess {
	return spawnProcessor(system, opts, false)
}

// newProcessorOrGet spawns the processor, or returns the existing one when a
// processor with the same id already exists. Used for internal system actors
// (write_stream / cluster kinds) that may be spawned concurrently by multiple
// senders, where a duplicate-id panic would kill the whole process.
func newProcessorOrGet(system ISystem, opts tOpts) iProcess {
	return spawnProcessor(system, opts, true)
}

func spawnProcessor(system ISystem, opts tOpts, orGet bool) iProcess {
	build := func() iProcess {
		p := &processorMailBox{
			tOpts:      opts,
			system:     system,
			rb:         ringbuffer.New[Context](int64(opts.mailboxSize)),
			procStatus: idle,
		}
		// Bind the receiver and enqueue the initialize message in the constructor,
		// before the processor is published to the registry. This guarantees that
		// a concurrent get()+send() (the write_stream / cluster spawn races) can
		// never observe a nil receiver, and that Started() (triggered by the
		// initialize message at the head of the queue) is always processed before
		// any externally-sent message. init() below only starts the run loop.
		p.receiver = p.producer()
		p.receiver._init(p.self())
		p.rb.Push(newContext(p.self(), p.self(), initialize, system.nextSnId(), system.getSender()))
		return p
	}
	if orGet {
		return system.getRegistry().getOrAdd(opts._self.GetId(), build)
	}
	return system.getRegistry().add(build)
}

func (x *processorMailBox) self() ActorRef { return x._self }
func (x *processorMailBox) opts() *tOpts   { return &x.tOpts }

// poison requests a stop without enqueuing into the (possibly full) mailbox:
// it sets a flag and wakes the run loop, which drains the remaining messages
// and then stops. Non-blocking and idempotent, so it is safe to call while
// holding registry locks (see system_life.stopActorsImpl).
func (x *processorMailBox) poison() {
	x.poisoned.Store(true)
	x.schedule()
}

func (x *processorMailBox) init() {
	// receiver binding and the initialize message are set up in build() before
	// the processor is published to the registry; here we only start the run
	// loop, which will process the queued initialize (-> Started) first.
	x.schedule()
}

func (x *processorMailBox) send(ctx Context) {
	//for re-entry
	if runningMsgId := x.receiver._getRunningMsgId(); runningMsgId != 0 && runningMsgId == ctx.GetMsgSnId() {
		x.invoke(ctx)
		return
	}
	if !x.rb.Push(ctx) {
		// mailbox already closed (actor stopped): drop rather than block forever.
		x.system.Logger().Warn("send to a stopped actor, msg dropped",
			"id", x.self(), "msgName", proto.MessageName(ctx.Message()))
		return
	}
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
		x.stop()
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
	x.run()
	// If run() exited because stop() set the status to stopped, leave it stopped.
	if !atomic.CompareAndSwapInt32(&x.procStatus, running, idle) {
		return
	}
	// Re-check the queue after going idle: a send() that pushed (or a poison()
	// that set the flag) between the last Pop and the CAS above would have had
	// its schedule() no-op (status was still running). Re-schedule so neither a
	// message nor a pending poison is lost (lost-wakeup fix).
	if x.rb.Len() > 0 || x.poisoned.Load() {
		x.schedule()
	}
}
func (x *processorMailBox) run() {
	i, t := 0, defaultThroughput
	for atomic.LoadInt32(&x.procStatus) != stopped {
		if i > t {
			i = 0
			runtime.Gosched()
		}
		i++
		if msg, ok := x.rb.Pop(); ok {
			x.receiver._setRunningMsgId(msg.GetMsgSnId())
			x.invoke(msg)
			x.receiver._cleanRunningMsgId()
		} else {
			// mailbox drained: if a poison was requested, stop now (after having
			// processed all already-enqueued messages). Otherwise just exit the
			// loop and let process() park the goroutine.
			if x.poisoned.Load() {
				x.stop()
			}
			return
		}
	}
}
func (x *processorMailBox) start() {
	defer func() {
		if err := recover(); err != nil {
			x.system.Logger().Error("spawn recover a panic on start. force to stop self",
				"id", x.self(),
				"err", err,
				"stack", ghelper.StackTrace())
			//force to stop self
			x.stop()
		}
	}()
	if x.tOpts.registerToCluster != nil {
		if err := x.tOpts.registerToCluster(x.system.GetProvider(), x.system.getConfig(), x.self()); err != nil {
			// cluster registration failed: this actor cannot serve its kind, so
			// stop it (logged, not panicked — a runtime failure, not a crash).
			// started is still false, so stop() will skip PreStop.
			x.system.Logger().Error("register to cluster failed, stop self",
				"id", x.self(), "err", err)
			x.stop()
			return
		}
	}
	x.receiver.Started()
	x.started = true
}
func (x *processorMailBox) stop() {
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
		//wake any sender blocked pushing into this now-dead mailbox (Push returns
		//false -> send() drops the message instead of blocking forever).
		x.rb.Close()
	}()
	//unregister from cluster
	defer func() {
		if x.tOpts.unRegisterFromCluster != nil {
			x.tOpts.unRegisterFromCluster(x.system.GetProvider(), x.system.getConfig(), x.self())
		}
	}()
	// PreStop pairs with a completed Started(): skip it when the actor never
	// finished starting (e.g. cluster-register failure, or Started panicked),
	// so cleanup code doesn't touch resources Started was supposed to set up.
	if x.started {
		x.receiver.PreStop()
	}
}
