package grain

type iProcessProvider func() iProcess

type iProcess interface {
	self() ActorRef
	opts() *tOpts
	init()
	send(ctx Context)
}
