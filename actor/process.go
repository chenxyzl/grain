package actor

type iProcess interface {
	//
	self() *ActorRef
	//
	init()
	// to self process
	send(ctx Context)
	// for mailbox to
	invoke(ctx Context)
}
