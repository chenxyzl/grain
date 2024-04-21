package actor

import (
	"github.com/chenxyzl/grain/utils/al/safemap"
	"log/slog"
)

var _ IActor = (*streamRouterActor)(nil)

type streamRouterActor struct {
	system *System
	self   *ActorRef
	//
	_logger *slog.Logger
	streams *safemap.SafeMap[string, *ActorRef]
}

func (x *streamRouterActor) logger() *slog.Logger {
	return x._logger
}

func (x *streamRouterActor) start() error {
	return nil
}

func (x *streamRouterActor) stop() error {
	return nil
}

func (x *streamRouterActor) receive(ctx IContext) {
	//to box ?
	//dispatcher ?
}

func (x *streamRouterActor) Receive(ctx IContext) {
	switch msg := ctx.Message().(type) {
	case *Envelope:
		x.dispatchMsg(msg)
	default:
		slog.Error("unknown message")
	}
}

func newStreamRouter(self *ActorRef, system *System) IActor {
	return &streamRouterActor{
		system:  system,
		self:    self,
		logger:  slog.With("streamRouterActor", system.clusterProvider.SelfAddr()),
		streams: safemap.New[string, *ActorRef](),
	}
}

func (x *streamRouterActor) Self() *ActorRef {
	return x.self
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
		return
	}
	x.system.Send(remoteStream, msg)
}
