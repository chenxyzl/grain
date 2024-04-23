package actor

import "log/slog"

type IActor interface {
	//inner api, for inherit auth
	_init(system *System, self *ActorRef, this IActor) //for bind self
	_self() *ActorRef
	_system() *System
	_logger() *slog.Logger

	//Started after self Instance
	Started() error
	//PreStop when receive poison, before stop self
	PreStop() error
	//Receive message
	Receive(ctx IContext)
}
