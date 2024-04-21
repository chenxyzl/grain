package actor

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc"
	"io"
	"log/slog"
)

//send msg to remote rpcServer
//1.init rpcClient
//2.

var _ IActor = (*streamWriteActor)(nil)

type streamWriteActor struct {
	system      *System
	router      *ActorRef
	address     string
	dialOptions []grpc.DialOption
	callOptions []grpc.CallOption
	//
	_logger *slog.Logger
	_self   *ActorRef
	//
	conn   *grpc.ClientConn
	remote Remoting_ListenClient
}

func newStreamWriterActor(system *System, router *ActorRef, address string, dialOptions []grpc.DialOption, callOptions []grpc.CallOption) iProcess {
	return &streamWriteActor{
		system:      system,
		router:      router,
		address:     address,
		dialOptions: dialOptions,
		callOptions: callOptions,
		_logger:     slog.With("actor", "streamWriteActor", "address", address),
		_self:       NewActorRef(system.clusterProvider.SelfAddr(), "stream_write/"+address),
	}
}

func (x *streamWriteActor) init() error {
	conn, err := grpc.Dial(x.address, x.dialOptions...)
	if err != nil {
		return errors.Join(fmt.Errorf("connect to grpc server err, addr:%v", x.address), err)
	}
	stream, err := NewRemotingClient(conn).Listen(context.Background(), x.callOptions...)
	if err != nil {
		x.conn.Close()
		return errors.Join(fmt.Errorf("listen to grpc server err, addr:%v", x.address), err)
	}
	x.conn = conn
	x.remote = stream
	go func() {
		for {
			unknownMsg, err := stream.Recv()
			switch {
			case errors.Is(err, io.EOF):
				slog.Info("remote stream closed", "address", x.address)
			case err != nil:
				slog.Error("remote stream lost connection", "address", x.address, "error", err)
			default: // DisconnectRequest
				slog.Warn("remote stream got a msg form remote, but this stream only for write", "address", x.address, "msg", unknownMsg)
			}
			x.system.Send(x.self(), Msg.Poison)
		}
	}()
	return nil
}

func (x *streamWriteActor) logger() *slog.Logger {
	return x._logger
}

func (x *streamWriteActor) self() *ActorRef {
	return x._self
}

func (x *streamWriteActor) start() error {
	return x.init()
}

func (x *streamWriteActor) stop() error {
	if x.remote != nil {
		err := x.remote.CloseSend()
		if err != nil {
			x.logger().Error("close grpc send stream err", "error", err)
		}
		x.remote = nil
	}
	if x.conn != nil {
		err := x.conn.Close()
		if err != nil {
			x.logger().Error("close grpc conn err", "error", err)
		}
		x.conn = nil
	}
	x.logger().Info("stop stream write actor end")
	return nil
}

func (x *streamWriteActor) receive(ctx IContext) {
	//TODO implement me
	panic("implement me")
}
