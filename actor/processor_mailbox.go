package actor

type processor struct {
	Opts
	mailBox  *MailBox
	restarts int32
}

var _ iProcess = (*processor)(nil)

func newProcessor(system *System, opts Opts) *processor {
	return &processor{
		Opts:     opts,
		mailBox:  NewMailBox(opts.MailboxSize),
		restarts: 0,
	}
}

func (p *processor) self() *ActorRef {
	return p.Self
}

func (p *processor) start() error {
	//todo send Msg.start

	//TODO implement me
	panic("implement me")
}

func (p *processor) stop() error {
	//todo send Msg.poison
	//TODO implement me
	panic("implement me")
}

func (p *processor) receive(ctx IContext) {
	//todo dispatcher
}
