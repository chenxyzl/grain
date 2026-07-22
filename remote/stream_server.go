package remote

import (
	"fmt"
	"io"
	"log/slog"
	"net"

	"github.com/chenxyzl/grain/al/gonet"
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
	// advertise a reachable inner IP instead of the wildcard "[::]" the listener
	// reports, otherwise other cluster nodes cannot dial this one. Fall back to
	// loopback when no inner NIC exists (single-machine / container / CI), so the
	// node still starts (only reachable locally, which is fine for that case).
	host := "127.0.0.1"
	if innerIP := gonet.GetTopInnerIP(); innerIP != nil {
		host = innerIP.String()
	}
	_, port, err := net.SplitHostPort(lis.Addr().String())
	if err != nil {
		_ = lis.Close()
		return fmt.Errorf("parse listen addr err: %w", err)
	}
	x.addr = net.JoinHostPort(host, port)
	x.logger = slog.With("rpcService", x.addr)
	x.gs = grpc.NewServer()
	RegisterRemotingServer(x.gs, x)
	go func() {
		if err = x.gs.Serve(lis); err != nil && err != io.EOF {
			panic(err)
		}
	}()
	x.Logger().Info("RpcService Started ...")
	return nil
}

func (x *RpcService) Stop() error {
	if x.gs != nil {
		x.gs.Stop()
		x.gs = nil
		x.Logger().Info("RpcService Stopped ...")
		//c := make(chan bool, 1)
		//go func() {
		//	x.gs.GracefulStop()
		//	c <- true
		//}()
		//
		//select {
		//case <-c:
		//	x.Logger().Info("RpcService Stopped ...")
		//case <-time.After(time.Second * 10):
		//	x.gs.Stop()
		//	x.Logger().Info("RpcService Stopped Timeout", "err", "timeout")
		//}
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
