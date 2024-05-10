package actor

type iProcess interface {
	//
	self() *ActorRef
	//
	start() error //
	//
	stop(withRegistry bool)
	// to self process
	send(ctx IContext)
	// for mailbox to
	invoke(ctx IContext)
}
