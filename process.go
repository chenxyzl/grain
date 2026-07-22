package grain

type iProcessProvider func() iProcess

type iProcess interface {
	self() ActorRef
	opts() *tOpts
	init()
	send(ctx Context)
	//poison requests the process to stop. It is non-blocking: it never enqueues
	//into a (possibly full) mailbox, so it is safe to call while holding locks.
	poison()
}
