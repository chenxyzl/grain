package actor

import (
	"log/slog"
)

type IActor interface {
	//inner api
	//
	//
	init(system *System, this IActor) //for bind self

	//base api
	//
	//

	Self() *ActorRef
	Logger() *slog.Logger
	System() *System

	//Started after Instance
	Started() error
	//PreStop when receive poison, before stop self
	PreStop() error

	//Receive message
	Receive(ctx IContext)
}
