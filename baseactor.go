package grain

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/protobuf/proto"
)

//var _ IActor = (*BaseActor)(nil)

const defaultRegisterTimes = 3

type BaseActor struct {
	ActorRef
	logger       *slog.Logger
	runningMsgId uint64
}

//func (x *BaseActor) Started()             {}
//func (x *BaseActor) PreStop()             {}
//func (x *BaseActor) Receive(ctx IContext) {}

func (x *BaseActor) _init(self ActorRef) {
	x.ActorRef = self
	x.logger = slog.With("actor", x.Self()) //warning: slog.With performance too slow
}
func (x *BaseActor) _getRunningMsgId() uint64             { return x.runningMsgId }
func (x *BaseActor) _setRunningMsgId(runningMsgId uint64) { x.runningMsgId = runningMsgId }
func (x *BaseActor) _cleanRunningMsgId()                  { x.runningMsgId = 0 }

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
func (x *BaseActor) Ask(target ActorRef, msg proto.Message) proto.Message {
	if target == nil {
		x.Logger().Error("ask target is nil", "id", x.Self(), "msgName", proto.MessageName(msg), "msg", msg)
		return nil
	}
	//
	system := target.GetSystem()
	reqTimeout := system.getConfig().askTimeout
	//
	reply := newProcessorReplay[proto.Message](system, reqTimeout)
	//
	system.getSender().tellWithSender(target, msg, reply.self(), x._getRunningMsgId())
	//
	v, err := reply.Result()
	if err != nil {
		panic(errors.Join(err, fmt.Errorf("ask err, sender:%v,target:%v,err:%v", x.Self(), target, err)))
	}
	return v
}

func (x *BaseActor) ScheduleSelfOnce(delay time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.GetSystem().GetScheduler().ScheduleOnce(x.Self(), delay, msg)
}

func (x *BaseActor) ScheduleSelfRepeated(delay time.Duration, interval time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.GetSystem().GetScheduler().ScheduleRepeated(x.Self(), delay, interval, msg)
}
