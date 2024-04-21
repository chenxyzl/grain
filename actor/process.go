package actor

type iProcess interface {
	//
	self() *ActorRef
	//
	start() error //
	//
	stop() error
	//
	receive(ctx IContext) // add to mail box
}
