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
	restarts   int32
	receiver   IActor
}

var _ iProcess = (*processorMailBox)(nil)

func newProcessor(system ISystem, opts tOpts) iProcess {
	p := &processorMailBox{
		tOpts:      opts,
		system:     system,
		rb:         ringbuffer.New[Context](int64(opts.mailboxSize)),
		procStatus: idle,
		restarts:   0,
	}
	p = system.getRegistry().add(p).(*processorMailBox)
	return p
}

func (x *processorMailBox) self() ActorRef {
	return x._self
}

func (x *processorMailBox) init() {
	x.receiver = x.producer()  //create actor
	x.receiver._init(x.self()) //bind
	x.send(newContext(x.self(), x.self(), initialize, x.system.getGenRequestId().genRequestId(), x.system.getSender()))
}

func (x *processorMailBox) send(ctx Context) {
	//for re-entry
	if runningMsgId := x.receiver._getRunningMsgId(); runningMsgId != 0 && runningMsgId == ctx.GetMsgSnId() {
		x.invoke(ctx)
		return
	}
	x.rb.Push(ctx)
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
	atomic.StoreInt32(&x.procStatus, idle)
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
			//x.send(newContext(x.self(), nil, messageDef.poison, x.system.getNextSnId(), context.Background()))
		}
	}()
	if x.tOpts.registerToCluster != nil {
		x.tOpts.registerToCluster(x.system.getProvider(), x.system.getConfig(), x.self())
	}
	x.receiver.Started()
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
	}()
	x.receiver.PreStop()
	if x.tOpts.unRegisterFromCluster != nil {
		x.tOpts.unRegisterFromCluster(x.system.getProvider(), x.system.getConfig(), x.self())
	}
}
