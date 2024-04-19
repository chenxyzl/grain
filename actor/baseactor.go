package actor

import (
	"log/slog"
)

var _ IActor = (*BaseActor)(nil)

type BaseActor struct {
	IActor
	self   *ActorRef
	system *System
	logger *slog.Logger
	//parent   *ActorRef
	//children *safemap.SafeMap[string, *ActorRef]
}

func (x *BaseActor) init(self IActor) {
	x.IActor = self
	x.logger = slog.With("actor", self)
}
