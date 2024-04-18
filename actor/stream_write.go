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
		self:        NewActorRef(system.clusterProvider.UnregisterActor()),
	}
}

func (x *streamWriteActor) init() (Remoting_ListenClient, error) {
	conn, err := grpc.Dial(x.address, x.dialOptions...)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("connect to grpc server err, addr:%v", address), err)
	}
	stream, err := NewRemotingClient(conn).Listen(context.Background(), x.callOptions...)
	if err != nil {
		x.conn.Close()
		return nil, errors.Join(fmt.Errorf("listen to grpc server err, addr:%v", address), err)
	}
	x.conn = conn
	x.remote = stream
	go func() {
		for {
			unknownMsg, err := stream.Recv()
			switch {
			case errors.Is(err, io.EOF):
				slog.Info("remote stream closed", "address", address)
			case err != nil:
				slog.Error("remote stream lost connection", "address", address, "error", err)
			default: // DisconnectRequest
				slog.Warn("remote stream got a msg form remote, but this stream only for write", "address", address, "msg", unknownMsg)
			}
			_ = x.conn.Close()
			x.conn = nil
			_ = x.remote.CloseSend()
			x.remote = nil
		}
	}()
	return stream, nil
}

func (x *streamWriteActor) Logger() *slog.Logger {
	return x.logger
}

func (x *streamWriteActor) Self() *ActorRef {
	return x.self
}

func (x *streamWriteActor) Start() error {
	x.init()
}

func (x *streamWriteActor) Stop() error {
	//TODO implement me
	panic("implement me")
}

func (x *streamWriteActor) receive(ctx IContext) {
	//TODO implement me
	panic("implement me")
}

func (x *streamWriteActor) Receive(ctx IContext) {
	//TODO implement me
	panic("implement me")
}
