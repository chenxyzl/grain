package grain

import (
	"github.com/chenxyzl/grain/al/ringbuffer"
	"github.com/chenxyzl/grain/message"
)

// processorMailBox connects message storage, lifecycle and scheduling. Each
// component owns its state; ordinary messages are handled by the drainer itself.
type processorMailBox struct {
	tOpts
	system    ISystem
	mailbox   *ringbuffer.RingBuffer[Context]
	receiver  IActor
	scheduler mailboxScheduler
	lifecycle actorLifecycleState
}

var _ iProcess = (*processorMailBox)(nil)

func newProcessor(system ISystem, opts tOpts) (iProcess, error) {
	return spawnProcessor(system, opts, false)
}
func newProcessorOrGet(system ISystem, opts tOpts) iProcess {
	processor, _ := spawnProcessor(system, opts, true)
	return processor
}
func spawnProcessor(system ISystem, opts tOpts, orGet bool) (iProcess, error) {
	build := func() iProcess {
		processor := &processorMailBox{tOpts: opts, system: system,
			mailbox: ringbuffer.New[Context](int64(opts.mailboxInitSize), int64(opts.mailboxMaxSize))}
		// Bind and enqueue startup before publishing, so external messages cannot overtake it.
		processor.receiver = processor.producer()
		processor.receiver._init(processor.self())
		processor.initializeScheduling()
		return processor
	}
	if orGet {
		return system.getRegistry().getOrAdd(opts._self.GetId(), build), nil
	}
	return system.getRegistry().add(build)
}
func (p *processorMailBox) self() ActorRef { return p._self }
func (p *processorMailBox) opts() *tOpts   { return &p.tOpts }

// Called before publishing the processor. Startup is first in the mailbox, so
// even a concurrent send before registry.init cannot overtake Started.
func (p *processorMailBox) initializeScheduling() {
	p.scheduler.initialize(p)
	p.receiver._bindExecutionGate(&p.scheduler.executionGate)
	p.mailbox.Push(newContext(p.self(), p.self(), msgInitialize, p.system.nextSnId(), p.system.getSender()))
}
func (p *processorMailBox) init() { p.scheduler.schedule() }
func (p *processorMailBox) send(ctx Context) {
	switch p.mailbox.Push(ctx) {
	case ringbuffer.PushOK:
		p.scheduler.schedule()
	case ringbuffer.PushOverflow:
		p.toDeadLetter(ctx, DeadLetterReasonOverflow)
	case ringbuffer.PushClosed:
		p.toDeadLetter(ctx, DeadLetterReasonStopped)
	}
}
func (p *processorMailBox) poison() {
	p.lifecycle.stopRequested.Store(true)
	p.scheduler.schedule()
}
func (p *processorMailBox) toDeadLetter(ctx Context, reason string) {
	if sender := ctx.Sender(); sender != nil && sender.isAsk() {
		sender.Tell(message.WithErr("ask target mailbox unavailable: " + reason))
	}
	p.system.emitDeadLetter(DeadLetter{Target: ctx.Target(), Owner: p.self(), Sender: ctx.Sender(),
		Message: ctx.Message(), MsgSnId: ctx.GetMsgSnId(), Reason: reason})
}
