package actor

import (
	"google.golang.org/protobuf/proto"
	"log/slog"
)

var _ IProcess = (IActor)(nil)

type IActor interface {
	//inner api
	//
	//

	init() error
	afterStop() error

	messageInvoker

	//base api
	//
	//

	Self() *ActorRef
	Logger() *slog.Logger

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

func (I IActor) Invoke(params []proto.Message) {
	//TODO implement me
	panic("implement me")
}
