package actor

type iProcess interface {
	//
	self() *ActorRef
	//
	init()
	// to self process
	send(ctx IContext)
	// for mailbox to
	invoke(ctx IContext)
}
