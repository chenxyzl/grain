package grain

import (
	"github.com/chenxyzl/grain/ghelper"
	"github.com/chenxyzl/grain/message"
)

type callKind uint8

const (
	callStart callKind = iota
	callReceive
	callStop
)

// execution is private to one drainer goroutine. It reuses its invocation scope
// while draining normally; after the first Ask it finishes only its current call.
// The execution gate publishes this scope only while this drainer holds permission.
type execution struct {
	scheduler                 *mailboxScheduler
	callKind                  callKind
	drainOwnershipTransferred bool
	goroutineID               uint64 // obtained once per drainer, reused when publishing ownership
}

func (call *execution) invoke(ctx Context) {
	defer call.recoverPanic()
	p := call.scheduler.processor
	if ctx.Target() == p.self() {
		switch ctx.Message().(type) {
		case *message.Initialize:
			call.callKind = callStart
			p.lifecycle.start(call)
			return
		case *message.Poison:
			p.poison()
			return
		}
	}
	call.callKind = callReceive
	p.receiver.Receive(ctx)
}
func (call *execution) stop() {
	defer call.recoverPanic()
	call.scheduler.processor.lifecycle.stop(call)
}
func (call *execution) recoverPanic() {
	if failure := recover(); failure != nil {
		call.reportPanic(failure)
	}
}
func (call *execution) reportPanic(failure any) {
	defer func() { _ = recover() }()
	p := call.scheduler.processor
	p.system.Logger().Error("actor call panic", "id", p.self(), "callKind", call.callKind, "err", failure, "stack", ghelper.StackTrace())
}
func (call *execution) suspend() {
	if !call.drainOwnershipTransferred {
		call.drainOwnershipTransferred = true
		// The successor inherits the existing drainRunning role; it does not CAS it.
		go call.scheduler.drain()
	}
	call.scheduler.executionGate.leave()
}
func (call *execution) resume() {
	call.scheduler.executionGate.enter(call)
}
