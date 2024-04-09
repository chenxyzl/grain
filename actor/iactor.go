package actor

import (
	"github.com/chenxyzl/grain/actor/iface"
	"log/slog"
)

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

	Self() iface.ActorRef
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
	receive(ctx Context)
	//Receive message
	Receive(ctx Context)
}
