package actor

import (
	"github.com/chenxyzl/grain/utils/al/safemap"
	share "github.com/chenxyzl/grain/utils/helper"
	"log/slog"
)

var _ IActor = (*BaseActor)(nil)

type BaseActor struct {
	inner    IActor
	self     *ActorRef
	parent   *ActorRef
	children *safemap.SafeMap[string, *ActorRef]
	system   *System
	logger   *slog.Logger
}

func (x *BaseActor) init() error {
	//TODO implement me
	panic("implement me")
}

func (x *BaseActor) afterStop() error {
	//TODO implement me
	panic("implement me")
}

func (x *BaseActor) invoke(envelope *MessageEnvelope) {
	//TODO implement me
	panic("implement me")
}

func (x *BaseActor) Self() *ActorRef {
	return x.self
}

func (x *BaseActor) Logger() *slog.Logger {
	return x.logger
}

func (x *BaseActor) Awake() error {
	//TODO implement me
	panic("implement me")
}

func (x *BaseActor) Start() error {
	//TODO implement me
	panic("implement me")
}

func (x *BaseActor) Stop() error {
	//TODO implement me
	panic("implement me")
}

func (x *BaseActor) receive(ctx IContext) {
	share.Recover(x.logger)
	if ctx.Message().ProtoReflect().Descriptor().FullName() == Msg.PoisonName {
		x.Receive(ctx)
	} else {
		x.Stop()
	}
}

func (x *BaseActor) Receive(ctx IContext) {
	//TODO implement me
	panic("implement me")
}

func (x *BaseActor) GetId() string {
	return x.self.GetId()
}
