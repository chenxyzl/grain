package actor

import (
	"github.com/chenxyzl/grain/utils/al/ringbuffer"
	"runtime"
	"sync/atomic"
)

const (
	defaultThroughput = 100
)

const (
	idle int32 = iota
	running
	stopped
)

type IMailBox interface {
	Send(IContext)
	Start(IContext)
	Stop()
}

type MailBox struct {
	rb         *ringbuffer.RingBuffer[IContext]
	proc       messageInvoker
	procStatus int32
}

func NewMailBox(size int, proc messageInvoker) *MailBox {
	return &MailBox{
		rb:         ringbuffer.New[IContext](int64(size)),
		proc:       proc,
		procStatus: idle,
	}
}

func (in *MailBox) send(msg IContext) {
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

func (in *MailBox) stop() {
	atomic.StoreInt32(&in.procStatus, stopped)
}
