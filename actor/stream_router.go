package actor

import (
	"github.com/chenxyzl/grain/actor/internal"
	"github.com/chenxyzl/grain/utils/al/safemap"
)

var _ IActor = (*streamRouterActor)(nil)

type streamRouterActor struct {
	BaseActor
	//
	streams *safemap.SafeMap[string, *ActorRef]
}

func (x *streamRouterActor) Receive(ctx IContext) {
	switch msg := ctx.Message().(type) {
	case *Envelope:
		x.dispatchMsg(msg)
	case *internal.StreamClosed:
		x.streamClosed(ctx.Sender())
	default:
		x.Logger().Error("unknown message")
	}
}

func newStreamRouter(self *ActorRef, system *System) IActor {
	return &streamRouterActor{
		streams: safemap.New[string, *ActorRef](),
	}
}

func (x *streamRouterActor) dispatchMsg(msg *Envelope) {
	targetAddress := msg.GetTarget().GetAddress()
	remoteStream, ok := x.streams.Get(targetAddress)
	//if not found, spawn it
	if !ok {
		remoteStream = x.system.SpawnNamed(func() IActor {
			return newStreamWriterActor(x.Self(), targetAddress, x.system.GetConfig().DialOptions, x.system.GetConfig().CallOptions)
		}, msg.GetTarget().GetIdentifier())
		//save
		x.streams.Set(targetAddress, remoteStream)
		x.Logger().Info("remote stream created", "address", remoteStream)
		return
	}
	x.system.Send(remoteStream, msg)
}

func (x *streamRouterActor) streamClosed(streamActor *ActorRef) {
	x.streams.Delete(streamActor.Address)
	x.Logger().Info("remote stream closed", "address", streamActor.Address)
}
