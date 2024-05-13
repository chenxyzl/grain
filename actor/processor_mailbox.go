package actor

import (
	"context"
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

type processorMailBox struct {
	Opts
	system     *System
	rb         *ringbuffer.RingBuffer[IContext]
	procStatus int32
	restarts   int32
	receiver   IActor
}

var _ iProcess = (*processorMailBox)(nil)

func newProcessor(system *System, opts Opts) iProcess {
	p := &processorMailBox{
		Opts:       opts,
		system:     system,
		rb:         ringbuffer.New[IContext](int64(opts.MailboxSize)),
		procStatus: idle,
		restarts:   0,
	}
	p = system.registry.add(p).(*processorMailBox)
	return p
}

func (x *processorMailBox) self() *ActorRef {
	return x.Self
}

func (x *processorMailBox) init() {
	x.receiver = x.Producer()                        //create actor
	x.receiver._init(x.system, x.self(), x.receiver) //bind
	x.send(newContext(x.self(), nil, messageDef.initialize, x.system.getNextSnId(), context.Background()))
}

func (x *processorMailBox) send(ctx IContext) {
	//for re-entry
	if runningMsgId := x.receiver._getRunningMsgId(); runningMsgId != 0 && runningMsgId == ctx.GetMsgSnId() {
		x.invoke(ctx)
		return
	}
	x.rb.Push(ctx)
	x.schedule()
}

func (x *processorMailBox) invoke(ctx IContext) {
	defer helper.RecoverInfo(func() string {
		return fmt.Sprintf("actor receive panic, id:%v, msgType:%v, msg:%v", x.self(), ctx.Message().ProtoReflect().Descriptor().FullName(), ctx.Message())
	}, x.system.Logger())
	//todo restart ?
	//todo actor life?
	switch ctx.Message().(type) {
	case *internal.Initialize:
		x.start()
	case *internal.Poison:
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
			x.system.Logger().Info("spawn recover a panic on start. will poison", "actor", x.self(), "err", err, "stack", debug.Stack())
			x.send(newContext(x.self(), nil, messageDef.poison, x.system.getNextSnId(), context.Background()))
		}
	}()
	x.receiver.Started()
}
func (x *processorMailBox) stop() {
	defer func() {
		if err := recover(); err != nil {
			x.system.Logger().Error("recover a panic on stop", "self", x.self(), "panic", err, "stack", debug.Stack())
		}
	}()
	//send stop to actor
	defer func() {
		//stop run
		atomic.StoreInt32(&x.procStatus, stopped)
		//remove from registry
		x.system.registry.remove(x.self())
	}()
	x.receiver.PreStop()
}
