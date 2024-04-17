package actor

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/chenxyzl/grain/utils/al/safemap"
	"google.golang.org/grpc"
	"io"
	"log/slog"
)

//todo select a remoteStreamWrite

type streamRouter struct {
	self        *ActorRef
	dialOptions []grpc.DialOption
	callOptions []grpc.CallOption
	tlsConfig   *tls.Config
	streams     *safemap.SafeMap[string, Remoting_ListenClient]
}

func (x *streamRouter) Receive(ctx IContext) {
	switch msg := ctx.Message().(type) {
	case *Request:
		x.dispatchMsg(msg)
	default:
		slog.Error("unknown message")
	}
}

func newStreamRouter(self *ActorRef, dialOptions []grpc.DialOption, callOptions []grpc.CallOption, tlsConfig *tls.Config) Receiver {
	return &streamRouter{
		self:        self,
		dialOptions: dialOptions,
		callOptions: callOptions,
		tlsConfig:   tlsConfig,
		streams:     safemap.New[string, Remoting_ListenClient](),
	}
}

func (x *streamRouter) Self() *ActorRef {
	return x.self
}

func (x *streamRouter) getRemoteStream(address string) (Remoting_ListenClient, error) {
	stream, ok := x.streams.Get(address)
	if ok {
		return stream, nil
	}
	conn, err := grpc.Dial(address, x.dialOptions...)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("connect to grpc server err, addr:%v", address), err)
	}
	stream, err = NewRemotingClient(conn).Listen(context.Background(), x.callOptions...)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("listen to grpc server err, addr:%v", address), err)
	}
	x.streams.Set(address, stream)
	go func() {
		for {
			unknownMsg, err := stream.Recv()
			switch {
			case errors.Is(err, io.EOF):
				slog.Info("remote stream closed", slog.String("address", address))
			case err != nil:
				slog.Error("remote stream lost connection", slog.String("address", address), slog.Any("error", err))
			default: // DisconnectRequest
				slog.Warn("remote stream got a msg form remote, but this stream only for write", slog.String("address", address), slog.Any("msg", unknownMsg))
			}
			x.streams.Delete(address)
		}
	}()
	return stream, nil
}

func (x *streamRouter) dispatchMsg(msg *Request) {
	//1. is self?
	if msg.GetTarget().GetAddress() == x.Self().GetAddress() {
		//todo send to local
		return
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
