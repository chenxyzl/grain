package actor

import (
	"fmt"
	"github.com/chenxyzl/grain/actor/internal"
	"github.com/chenxyzl/grain/utils/al/ringbuffer"
	"github.com/chenxyzl/grain/utils/helper"
	"runtime"
	"runtime/debug"
	"sync/atomic"
)

const (
	defaultThroughput = 10
)

const (
	idle int32 = iota
	running
	stopped
)

type processor struct {
	Opts
	system     *System
	rb         *ringbuffer.RingBuffer[IContext]
	procStatus int32
	restarts   int32
	receiver   IActor
}

var _ iProcess = (*processor)(nil)

func newProcessor(system *System, opts Opts) *processor {
	p := &processor{
		Opts:       opts,
		system:     system,
		rb:         ringbuffer.New[IContext](int64(opts.MailboxSize)),
		procStatus: idle,
		restarts:   0,
	}
	return p
}

func (p *processor) self() *ActorRef {
	return p.Self
}

func (p *processor) start() {
	p.system.registry.add(p)
	defer func() {
		if err := recover(); err != nil {
			p.system.registry.remove(p.self())
			p.system.Logger().Info("spawn actor error.", "actor", p.self(), "err", err)
		}
	}()
	//create actor
	p.receiver = p.Producer()
	p.receiver._init(p.system, p.self(), p.receiver)
	p.receiver._setRunningMsgId(p.system.getNextSnIdIfNot0(p.receiver._getRunningMsgId()))
	p.receiver._cleanRunningMsgId()
	p.receiver.Started()
}

func (p *processor) stop(ignoreRegistry bool) {
	defer func() {
		if err := recover(); err != nil {
			p.system.Logger().Error("recover a panic on stop", "self", p.self(), "panic", err, "stack", debug.Stack())
		}
	}()
	//send stop to actor
	p.receiver._setRunningMsgId(p.system.getNextSnIdIfNot0(p.receiver._getRunningMsgId()))
	defer func() {
		p.receiver._cleanRunningMsgId()
		//stop run
		atomic.StoreInt32(&p.procStatus, stopped)
		//remove from registry
		if !ignoreRegistry {
			p.system.registry.remove(p.self())
		}
	}()
	p.receiver.PreStop()
}

func (p *processor) send(ctx IContext) {
	//for re-entry
	if runningMsgId := p.receiver._getRunningMsgId(); runningMsgId != 0 && runningMsgId == ctx.GetMsgSnId() {
		p.invoke(ctx)
		return
	}
	p.rb.Push(ctx)
	p.schedule()
}

func (p *processor) invoke(ctx IContext) {
	defer helper.RecoverInfo(func() string {
		return fmt.Sprintf("actor receive panic, id:%v, msgType:%v, msg:%v", p.self(), ctx.Message().ProtoReflect().Descriptor().FullName(), ctx.Message())
	}, p.system.Logger())
	//todo restart ?
	//todo actor life?
	switch msg := ctx.Message().(type) {
	case *internal.Poison:
		p.stop(msg.GetIgnoreRegistry())
	default:
		p.receiver.Receive(ctx)
	}
}
func (p *processor) schedule() {
	if atomic.CompareAndSwapInt32(&p.procStatus, idle, running) {
		go p.process()
	}
}
func (p *processor) process() {
	p.run()
	atomic.StoreInt32(&p.procStatus, idle)
}
func (p *processor) run() {
	i, t := 0, defaultThroughput
	for atomic.LoadInt32(&p.procStatus) != stopped {
		if i > t {
			i = 0
			runtime.Gosched()
		}
		i++
		if msg, ok := p.rb.Pop(); ok {
			p.receiver._setRunningMsgId(msg.GetMsgSnId())
			p.invoke(msg)
			p.receiver._cleanRunningMsgId()
		} else {
			return
		}
	}
}
