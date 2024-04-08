package actor

import (
	"github.com/chenxyzl/grain/al/ringbuffer"
	"runtime"
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

type IMailBox interface {
	Send(*MessageEnvelope)
	Start(*messageInvoker)
	Stop()
}

type MailBox struct {
	rb         *ringbuffer.RingBuffer[*MessageEnvelope]
	proc       messageInvoker
	procStatus int32
}

func NewMailBox(size int) *MailBox {
	return &MailBox{
		rb: ringbuffer.New[*MessageEnvelope](int64(size)),
	}
}

func (in *MailBox) Recv(msg *MessageEnvelope) {
	in.rb.Push(msg)
	in.schedule()
}

func (in *MailBox) schedule() {
	if atomic.CompareAndSwapInt32(&in.procStatus, idle, running) {
		go in.process()
	}
}

func (in *MailBox) process() {
	in.run()
	atomic.StoreInt32(&in.procStatus, idle)
}

func (in *MailBox) run() {
	i, t := 0, defaultThroughput
	for atomic.LoadInt32(&in.procStatus) != stopped {
		if i > t {
			i = 0
			runtime.Gosched()
		}
		i++
		if msg, ok := in.rb.Pop(); ok {
			in.proc.invoke(msg)
		} else {
			return
		}
	}
}

func (in *MailBox) Start(proc messageInvoker) {
	in.proc = proc
}

func (in *MailBox) Stop() {
	atomic.StoreInt32(&in.procStatus, stopped)
}
