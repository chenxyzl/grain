package grain

type iProcessProvider func() iProcess

type iProcess interface {
	//
	self() ActorRef
	//
	init()
	// to self process
	send(ctx Context)
}
