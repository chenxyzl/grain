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

var _ IProcess = (*streamWriteActor)(nil)

type streamWriteActor struct {
	system      *System
	router      *ActorRef
	address     string
	dialOptions []grpc.DialOption
	callOptions []grpc.CallOption
	//
	logger *slog.Logger
	self   *ActorRef
	//
	conn   *grpc.ClientConn
	remote Remoting_ListenClient
}

func newStreamWriterActor(system *System, router *ActorRef, address string, dialOptions []grpc.DialOption, callOptions []grpc.CallOption) IProcess {
	return &streamWriteActor{
		system:      system,
		router:      router,
		address:     address,
		dialOptions: dialOptions,
		callOptions: callOptions,
		logger:      slog.With("actor", "streamWriteActor", "address", address),
		self:        NewActorRef(system.clusterProvider.SelfAddr(), "streamwrite/"+address),
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
			x.system.Send(x.Self(), Msg.Poison)
		}
	}()
	return nil
}

func (x *streamWriteActor) Logger() *slog.Logger {
	return x.logger
}

func (x *streamWriteActor) Self() *ActorRef {
	return x.self
}

func (x *streamWriteActor) Start() error {
	return x.init()
}

func (x *streamWriteActor) Stop() error {
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
	return nil
}

func (x *streamWriteActor) receive(ctx IContext) {
	//TODO implement me
	panic("implement me")
}

func (x *streamWriteActor) Receive(ctx IContext) {
	
}
