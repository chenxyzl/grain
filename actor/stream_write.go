package actor

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc"
	"io"
)

//send msg to remote rpcServer
//1.init rpcClient
//2.

var _ IActor = (*streamWriteActor)(nil)

type streamWriteActor struct {
	BaseActor
	router      *ActorRef
	address     string
	dialOptions []grpc.DialOption
	callOptions []grpc.CallOption
	//
	conn   *grpc.ClientConn
	remote Remoting_ListenClient
}

func newStreamWriterActor(router *ActorRef, address string, dialOptions []grpc.DialOption, callOptions []grpc.CallOption) IActor {
	return &streamWriteActor{
		router:      router,
		address:     address,
		dialOptions: dialOptions,
		callOptions: callOptions,
	}
}

func (x *streamWriteActor) Started() {
	conn, err := grpc.Dial(x.address, x.dialOptions...)
	if err != nil {
		panic(errors.Join(fmt.Errorf("connect to grpc server err, addr:%v", x.address), err))
	}
	stream, err := NewRemotingClient(conn).Listen(context.Background(), x.callOptions...)
	if err != nil {
		_ = x.conn.Close()
		panic(errors.Join(fmt.Errorf("listen to grpc server err, addr:%v", x.address), err))
	}
	x.conn = conn
	x.remote = stream
	go func() {
		for {
			unknownMsg, err := stream.Recv()
			switch {
			case errors.Is(err, io.EOF):
				x.Logger().Info("remote stream closed", "address", x.address)
			case err != nil:
				x.Logger().Error("remote stream lost connection", "address", x.address, "error", err)
			default: // DisconnectRequest
				x.Logger().Warn("remote stream got a msg form remote, but this stream only for write", "address", x.address, "msg", unknownMsg)
			}
			x.system.send(x.Self(), messageDef.poison, x.system.getNextSnId())
		}
	}()
}

func (x *streamWriteActor) PreStop() {
	if x.remote != nil {
		err := x.remote.CloseSend()
		if err != nil {
			x.Logger().Error("close grpc send stream err", "error", err)
		}
		x.remote = nil
	}
	if x.conn != nil {
		err := x.conn.Close()
		if err != nil {
			x.Logger().Error("close grpc conn err", "error", err)
		}
		x.conn = nil
	}
	x.Logger().Info("stop stream write actor end")
}

func (x *streamWriteActor) Receive(ctx IContext) {
	switch msg := ctx.Message().(type) {
	case *Envelope:
		x.sendToRemote(msg)
	default:
		x.Logger().Error("receive unknown msg", "msg.type", msg.ProtoReflect().Descriptor().FullName(), "msg.content", msg)
	}
}

func (x *streamWriteActor) sendToRemote(msg *Envelope) {
	err := x.remote.Send(msg)
	if err != nil {
		_ = x.conn.Close()
		x.system.Poison(x.Self())
		x.Logger().Error("send envelope err,", "err", err)
	}
}
