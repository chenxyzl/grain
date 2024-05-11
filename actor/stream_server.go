package actor

import (
	"context"
	"errors"
	"github.com/chenxyzl/grain/utils/helper"
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
	defer helper.Recover()
	//save to
	for {
		msg, err := server.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				x.Logger().Info("Connection Closed")
				return nil
			} else {
				x.Logger().Error("Read Failed, Close Connection", "err", err)
				return err
			}
		}
		//
		x.ensureActorExist(msg.GetTarget())
		//
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

func (x *RPCService) ensureActorExist(ref *ActorRef) {
	//todo use agent or direct registry?

	//todo 1. check get?
	//todo 2. if not found, get from kind local provider?
	//todo 2.1 if kind not found at local provider, ignore and print log
	//todo 2.2 if found kind at local provider, new kind
	//todo 2.2.1 add to this registry, use return to instead self(because may already add, for double check)
	//todo return self
}
