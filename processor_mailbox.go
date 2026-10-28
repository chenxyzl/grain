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

func (p *processorMailBox) self() ActorRef { return p._self }
func (p *processorMailBox) opts() *tOpts   { return &p.tOpts }

// Called before publishing the processor. Startup is first in the mailbox, so
// even a concurrent send before the spawner starts draining cannot overtake Started.
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
