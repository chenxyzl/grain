package grain

type iProcess interface {
	//
	self() ActorRef
	//
	init()
	// to self process
	send(ctx Context)
}
