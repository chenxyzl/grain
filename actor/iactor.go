package actor

import (
	"log/slog"
)

var _ IProcess = (IActor)(nil)

type IActor interface {
	//inner api
	//
	//
	bind(system *System, this IActor, self *ActorRef) //for bind self

	//base api
	//
	//

	Self() *ActorRef
	Logger() *slog.Logger
	System() *System

	//life api
	//
	//

	//Awake before start
	Awake() error
	//Start after awake
	Start() error
	//Stop will destroy
	Stop() error

	//receive wrapper
	receive(ctx IContext)
	//Receive message
	Receive(ctx IContext)
}
