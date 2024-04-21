package actor

type processor struct {
	//mailbox
	//actorRef
	//
}

var _ iProcess = (*processor)(nil)

func newProcessor(system *System, opts Opts) *processor {
	return &processor{}
}

func (p processor) self() *ActorRef {
	//TODO implement me
	panic("implement me")
}

func (p processor) start() error {
	//todo send msg.Start

	//TODO implement me
	panic("implement me")
}

func (p processor) stop() error {
	//todo send msg.Poison
	//TODO implement me
	panic("implement me")
}

func (p processor) receive(ctx IContext) {

}
