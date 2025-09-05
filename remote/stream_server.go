package remote

import (
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/chenxyzl/grain/ghelper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// iRpcReceiver ...
type iRpcReceiver func(envelope *Envelope)

type RpcService struct {
	iRpcReceiver iRpcReceiver
	//
	logger *slog.Logger
	addr   string
	gs     *grpc.Server
}

func NewRpcServer(iRpcReceiver iRpcReceiver) *RpcService {
	return &RpcService{iRpcReceiver: iRpcReceiver}
}

func (x *RpcService) Listen(server Remoting_ListenServer) error {
	//save to
	for {
		msg, err := server.Recv()
		switch {
		case err == io.EOF:
			x.Logger().Info("connection closed 1")
			return nil
		case status.Code(err) == codes.Canceled:
			x.Logger().Info("connection closed 2")
			return nil
		case status.Code(err) > 0:
			x.Logger().Info("connection closed 3", "cod", status.Code(err))
		case err != nil:
			x.Logger().Error("read failed, close connection", "err", err)
			return err
		default:
			//do something left
		}
		x.recvEnvelope(msg)
	}
}

func (x *RpcService) mustEmbedUnimplementedRemotingServer() {
	x.Logger().Info("mustEmbedUnimplementedRemotingServer")
}

func (x *RpcService) Start() error {
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

func (x *RpcService) Stop() error {
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

func (x *RpcService) Addr() string {
	return x.addr
}

func (x *RpcService) Logger() *slog.Logger {
	return x.logger
}

func (x *RpcService) recvEnvelope(envelope *Envelope) {
	defer func() {
		if err := recover(); err != nil {
			x.Logger().Error("panic recover", "err", err, "stack", ghelper.StackTrace())
		}
	}()
	x.iRpcReceiver(envelope)
}
