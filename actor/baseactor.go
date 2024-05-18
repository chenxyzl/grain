package actor

import (
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"log/slog"
	"time"
)

//var _ IActor = (*BaseActor)(nil)

const defaultRegisterTimes = 3

type BaseActor struct {
	//impl         IActor
	self         *ActorRef
	system       *System
	logger       *slog.Logger
	runningMsgId uint64
}

//func (x *BaseActor) Started()             {}
//func (x *BaseActor) PreStop()             {}
//func (x *BaseActor) Receive(ctx IContext) {}

func (x *BaseActor) _init(system *System, self *ActorRef, this IActor) {
	x.system = system
	x.self = self
	//x.impl = this
	x.logger = slog.With("actor", x.self) //warning: slog.With performance too slow
}
func (x *BaseActor) _getRunningMsgId() uint64             { return x.runningMsgId }
func (x *BaseActor) _setRunningMsgId(runningMsgId uint64) { x.runningMsgId = runningMsgId }
func (x *BaseActor) _cleanRunningMsgId()                  { x.runningMsgId = 0 }

func (x *BaseActor) Self() *ActorRef { return x.self }

func (x *BaseActor) Logger() *slog.Logger { return x.logger }

func (x *BaseActor) System() *System { return x.system }

func (x *BaseActor) Send(target *ActorRef, msg proto.Message) {
	if target == nil {
		x.Logger().Error("send target is nil", "id", x.Self(), "msgName", proto.MessageName(msg), "msg", msg)
		return
	}
	//todo check reply
	//x.system.sendWithoutSender(target, msg, x.runningMsgId)
	x.system.sendWithoutSender(target, msg)
}

// Request
// wanted BaseActor.Request[T proto.Message](target *ActorRef, req proto.Message) T
// but golang not support
func (x *BaseActor) Request(target *ActorRef, msg proto.Message) proto.Message {
	if target == nil {
		x.Logger().Error("request target is nil", "id", x.Self(), "msgName", proto.MessageName(msg), "msg", msg)
		return nil
	}

	v, err := request[proto.Message](x.system, target, msg, x.runningMsgId)
	if err != nil {
		panic(errors.Join(err, fmt.Errorf("requset err, sender:%v,target:%v,request:%v", x.Self(), target, err)))
	}
	return v
}

func (x *BaseActor) ScheduleSelfOnce(delay time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.system.timerSchedule.sendOnce(x.Self(), delay, msg)
}

func (x *BaseActor) ScheduleSelfRepeated(delay time.Duration, interval time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.system.timerSchedule.sendRepeatedly(x.Self(), delay, interval, msg)
}
