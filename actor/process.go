package actor

type iProcess interface {
	//
	self() *ActorRef
	//
	start() //
	//
	stop(ignoreRegistry bool)
	// to self process
	send(ctx IContext)
	// for mailbox to
	invoke(ctx IContext)
}
