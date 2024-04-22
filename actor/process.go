package actor

type iProcess interface {
	//
	self() *ActorRef
	//
	start() error //
	//
	stop() error
	// send to mail box
	send(ctx IContext)
	// for mailbox to
	invoke(ctx IContext)
}
