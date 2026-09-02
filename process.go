package grain

type iProcessProvider func() iProcess

type iProcess interface {
	self() ActorRef
	opts() *tOpts
	init()
	send(ctx Context)
	//poison requests a stop. Non-blocking: never enqueues, so it is safe to call while holding locks.
	poison()
}
