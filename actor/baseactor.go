package actor

import "log/slog"

var _ IActor = (*BaseActor)(nil)

type BaseActor struct {
	IActor
	self   *ActorRef
	system *System
	logger *slog.Logger
}

func (x *BaseActor) Started() error       { return nil }
func (x *BaseActor) PreStop() error       { return nil }
func (x *BaseActor) Receive(ctx IContext) {}

func (x *BaseActor) _init(system *System, self *ActorRef, this IActor) {
	x.system = system
	x.self = self
	x.IActor = this
	x.logger = slog.With("actor", x.self)
}

func (x *BaseActor) _self() *ActorRef { return x.self }
func (x *BaseActor) Self() *ActorRef  { return x.self }

func (x *BaseActor) _logger() *slog.Logger { return x.logger }
func (x *BaseActor) Logger() *slog.Logger  { return x.logger }

func (x *BaseActor) _system() *System { return x.system }
func (x *BaseActor) System() *System  { return x.system }
