package actor

type iProcess interface {
	//
	self() *ActorRef
	//
	init()
	//
	start() //
	//
	stop()
	// to self process
	send(ctx IContext)
	// for mailbox to
	invoke(ctx IContext)
}
