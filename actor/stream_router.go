package actor

import (
	"github.com/chenxyzl/grain/utils/al/safemap"
	"io"
	"log/slog"
)

//todo select a remoteStreamWrite

type streamRouterActor struct {
	system *System
	self   *ActorRef
	//
	logger  *slog.Logger
	streams *safemap.SafeMap[string, *ActorRef]
}

func (x *streamRouterActor) Logger() *slog.Logger {
	return x.logger
}

func (x *streamRouterActor) Start() error {
	return nil
}

func (x *streamRouterActor) Stop() error {
	return nil
}

func (x *streamRouterActor) receive(ctx IContext) {
	//to box ?
	//dispatcher ?
}

func (x *streamRouterActor) Receive(ctx IContext) {
	switch msg := ctx.Message().(type) {
	case *Request:
		x.dispatchMsg(msg)
	default:
		slog.Error("unknown message")
	}
}

func newStreamRouter(self *ActorRef, system *System) IProcess {
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

func (x *streamRouterActor) dispatchMsg(msg *Request) {
	targetAddress := msg.GetTarget().GetAddress()
	//1. is self?
	if targetAddress == x.Self().GetAddress() {
		x.system.sendToLocal(msg)
		return
	}

	stremActorRef, ok := x.streams[targetAddress]
	if ok {
		x.system.sendToLocal()
	}

	//todo need dispatcher to everyone self remote stream?
	//todo need check address in cluster?
	stream, err := x.getRemoteStream(msg.GetTarget().GetAddress())
	if err != nil {
		slog.Error("get stream err", "address", msg.GetTarget().GetAddress(), "err", err)
		return
	}
	err = stream.Send(msg)
	if err != nil {
		if err == io.EOF {
			x.streams.Delete(msg.GetTarget().GetAddress())
			//todo need close stream conn?
		}
		slog.Error("send msg err", "address", msg.GetTarget().GetAddress(), "msg", msg, "err", err)
	}
}
