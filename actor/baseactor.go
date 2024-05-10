package actor

import (
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"log/slog"
)

var _ IActor = (*BaseActor)(nil)

type BaseActor struct {
	IActor
	self         *ActorRef
	system       *System
	logger       *slog.Logger
	runningMsgId uint64
}

func (x *BaseActor) Started()             {}
func (x *BaseActor) PreStop()             {}
func (x *BaseActor) Receive(ctx IContext) {}

func (x *BaseActor) _init(system *System, self *ActorRef, this IActor) {
	x.system = system
	x.self = self
	x.IActor = this
	x.logger = slog.With("actor", x.self) //warning: slog.With performance too slow
}
func (x *BaseActor) _getRunningMsgId() uint64             { return x.runningMsgId }
func (x *BaseActor) _setRunningMsgId(runningMsgId uint64) { x.runningMsgId = runningMsgId }
func (x *BaseActor) _cleanRunningMsgId()                  { x.runningMsgId = 0 }

func (x *BaseActor) Self() *ActorRef { return x.self }

func (x *BaseActor) Logger() *slog.Logger { return x.logger }

func (x *BaseActor) System() *System { return x.system }

func (x *BaseActor) Send(target *ActorRef, msg proto.Message) {
	x.system.send(target, msg, x.runningMsgId)
}

// Request
// wanted BaseActor.Request[T proto.Message](target *ActorRef, req proto.Message) T
// but golang not support
func (x *BaseActor) Request(target *ActorRef, msg proto.Message) proto.Message {
	v, err := request[proto.Message](x.system, target, msg, x.runningMsgId)
	if err != nil {
		panic(errors.Join(err, fmt.Errorf("requset err, sender:%v,target:%v,request:%v", x.Self(), target, err)))
	}
	return v
}
