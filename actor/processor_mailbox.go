package actor

import (
	"fmt"
	"github.com/chenxyzl/grain/actor/internal"
	share "github.com/chenxyzl/grain/utils/helper"
)

type processor struct {
	Opts
	system   *System
	mailBox  *MailBox
	restarts int32
	receiver IActor
}

var _ iProcess = (*processor)(nil)

func newProcessor(system *System, opts Opts) *processor {
	p := &processor{
		Opts:     opts,
		system:   system,
		restarts: 0,
	}
	p.mailBox = NewMailBox(opts.MailboxSize, p)
	return p
}

func (p *processor) self() *ActorRef {
	return p.Self
}

func (p *processor) start() error {
	//create actor
	p.receiver = p.Producer()
	//send start to  actor
	p.mailBox.send(newContext(p.self(), p.self(), Message.start, p.Context))
	return nil
}

func (p *processor) stop() error {
	//send stop to actor
	p.mailBox.send(newContext(p.self(), p.self(), Message.stop, p.Context))
	//stop mailbox
	p.mailBox.stop()
	//remove from registry
	p.system.registry.Remove(p.self())
	//
	return nil
}

func (p *processor) send(ctx IContext) {
	p.mailBox.send(ctx)
}

func (p *processor) invoke(ctx IContext) {
	defer share.RecoverInfo(fmt.Sprintf("actor receive panic, id:%v ", p.self()), p.system.Logger())
	//todo restart ?
	//todo actor life?

	switch ctx.Message().(type) {
	case *internal.Start:
		err := p.receiver.Started()
		if err != nil {
			panic(err)
		}
	case *internal.Stop:
		err := p.receiver.PreStop()
		if err != nil {
			panic(err)
		}
	case *internal.Poison:
		//todo deal all mailbox msg, add send stop to mailbox
	default:
		p.receiver.Receive(ctx)
	}
}
