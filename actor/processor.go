package actor

type processor struct {
	//mailbox
	//actorRef
	//
}

var _ iProcess = (*processor)(nil)

func newProcessor(system *System) *processor {
	return &processor{}
}

func (p processor) self() *ActorRef {
	//TODO implement me
	panic("implement me")
}

func (p processor) start() error {
	//TODO implement me
	panic("implement me")
}

func (p processor) stop() error {
	//TODO implement me
	panic("implement me")
}

func (p processor) receive(ctx IContext) {
	//watch start
	//watch stop
	//watch poison

	//TODO implement me
	panic("implement me")
}
