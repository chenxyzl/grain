package actor

import (
	"log/slog"
)

type IProcess interface {
	Logger() *slog.Logger
	Self() *ActorRef
	Start() error //
	Stop() error
	receive(ctx IContext) // add to mail box
	Receive(ctx IContext) // actor receive msg
}

type process struct {
}
