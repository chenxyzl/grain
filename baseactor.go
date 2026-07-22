package grain

import (
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

//var _ IActor = (*BaseActor)(nil)

type BaseActor struct {
	ActorRef
	logger *slog.Logger
	// runningMsgId is written by the actor's own goroutine (run loop) and read
	// by arbitrary sender goroutines in processorMailBox.send (re-entry check),
	// so it must be accessed atomically.
	runningMsgId atomic.Uint64
}

//func (x *BaseActor) Started()             {}
//func (x *BaseActor) PreStop()             {}
//func (x *BaseActor) Receive(ctx IContext) {}

func (x *BaseActor) _init(self ActorRef) {
	x.ActorRef = self
	x.logger = slog.With("actor", x.Self()) //warning: slog.With performance too slow
}
func (x *BaseActor) _getRunningMsgId() uint64             { return x.runningMsgId.Load() }
func (x *BaseActor) _setRunningMsgId(runningMsgId uint64) { x.runningMsgId.Store(runningMsgId) }
func (x *BaseActor) _cleanRunningMsgId()                  { x.runningMsgId.Store(0) }

func (x *BaseActor) Self() ActorRef       { return x.ActorRef }
func (x *BaseActor) Logger() *slog.Logger { return x.logger }

func (x *BaseActor) Send(target ActorRef, msg proto.Message) {
	if target == nil {
		x.Logger().Error("send target is nil", "id", x.Self(), "msgName", proto.MessageName(msg), "msg", msg)
		return
	}
	x.GetSystem().getSender().tell(target, msg)
}

// Ask allowed re-entry
// wanted BaseActor.Ask[T proto.Message](target ActorRef, req proto.Message) T
// but golang not support
//
// Returns the reply, or an *message.ErrCode on failure (nil target, timeout,
// remote error, type mismatch). Runtime failures are returned to the caller,
// not panicked.
func (x *BaseActor) Ask(target ActorRef, msg proto.Message) (proto.Message, *message.ErrCode) {
	if target == nil {
		return nil, message.WithErr(fmt.Sprintf("ask target is nil, sender:%v", x.Self()))
	}
	//
	sys := target.GetSystem()
	reqTimeout := sys.getConfig().askTimeout
	//
	reply := newProcessorReplay[proto.Message](sys, reqTimeout)
	//
	sys.getSender().tellWithSender(target, msg, reply.self(), x._getRunningMsgId())
	//
	return reply.Result()
}

func (x *BaseActor) ScheduleSelfOnce(delay time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.GetSystem().GetScheduler().ScheduleOnce(x.Self(), delay, msg)
}

func (x *BaseActor) ScheduleSelfRepeated(delay time.Duration, interval time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.GetSystem().GetScheduler().ScheduleRepeated(x.Self(), delay, interval, msg)
}
