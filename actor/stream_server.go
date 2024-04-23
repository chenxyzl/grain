package actor

import (
	"context"
	"errors"
	"github.com/chenxyzl/grain/utils/share"
	"google.golang.org/grpc"
	"io"
	"log/slog"
	"net"
	"time"
)

type RPCService struct {
	system *System
	//
	logger *slog.Logger
	addr   string
	gs     *grpc.Server
}

func NewRpcServer(system *System) *RPCService {
	return &RPCService{system: system}
}

func (x *RPCService) Listen(server Remoting_ListenServer) error {
	defer share.Recover()
	//save to
	for {
		msg, err := server.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				slog.Info("Connection Closed")
				return nil
			} else {
				slog.Error("Read Failed, Close Connection", "err", err)
				return err
			}
		}
		x.system.sendToLocal(msg)
	}
}

func (x *RPCService) mustEmbedUnimplementedRemotingServer() {
	x.Logger().Info("mustEmbedUnimplementedRemotingServer")
}

func (x *RPCService) Start() error {
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		return err
	}
	x.addr = lis.Addr().String()
	x.logger = slog.With("rpcService", x.addr)
	x.gs = grpc.NewServer()
	RegisterRemotingServer(x.gs, x)
	go func() {
		if err = x.gs.Serve(lis); err != nil && err != io.EOF {
			panic(err)
		}
	}()
	return nil
}

func (x *RPCService) Stop() error {
	if x.gs != nil {
		c := make(chan bool, 1)
		go func() {
			x.gs.GracefulStop()
			c <- true
		}()

		select {
		case <-c:
			x.Logger().Info("Stopped Proto.Actor server")
		case <-time.After(time.Second * 10):
			x.gs.Stop()
			x.Logger().Info("Stopped Proto.Actor server", "err", "timeout")
		}
	}
	return nil
}

func (x *RPCService) SelfAddr() string {
	return x.addr
}

func (x *RPCService) Logger() *slog.Logger {
	return x.logger
}
