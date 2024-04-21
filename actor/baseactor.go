package actor

import "log/slog"

var _ IActor = (*BaseActor)(nil)

type BaseActor struct {
	this   IActor
	self   *ActorRef
	system *System
	logger *slog.Logger
}

func (x *BaseActor) Started() error { return nil }

func (x *BaseActor) PreStop() error { return nil }

func (x *BaseActor) Receive(ctx IContext) {}

func (x *BaseActor) init(system *System, self *ActorRef, this IActor) {
	x.system = system
	x.self = self
	x.logger = slog.With()
}

func (x *BaseActor) Self() *ActorRef {
	return x.self
}

func (x *BaseActor) Logger() *slog.Logger {
	return x.logger
}

func (x *BaseActor) System() *System {
	return x.system
}
