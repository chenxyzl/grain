package actor

import (
	"github.com/chenxyzl/grain/actor/iface"
	"github.com/chenxyzl/grain/utils/al/safemap"
	share "github.com/chenxyzl/grain/utils/helper"
	"log/slog"
)

var _ iface.ActorRef = (*BaseActor)(nil)
var _ IActor = (*BaseActor)(nil)

type BaseActor struct {
	self     *Address
	parent   iface.ActorRef
	children *safemap.SafeMap[string, iface.ActorRef]
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

func (x *BaseActor) Self() iface.ActorRef {
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
	x.Receive(ctx)
}

func (x *BaseActor) Receive(ctx IContext) {
	//TODO implement me
	panic("implement me")
}

func (x *BaseActor) GetId() string {
	return x.self.GetId()
}
