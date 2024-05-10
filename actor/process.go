package actor

type iProcess interface {
	//
	self() *ActorRef
	//
	start() //
	//
	stop(withRegistry bool)
	// to self process
	send(ctx IContext)
	// for mailbox to
	invoke(ctx IContext)
}
