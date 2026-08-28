package remote

import (
	"errors"
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
	for {
		msg, err := server.Recv()
		// EVERY Recv error is terminal for this stream: grpc caches it and returns the
		// same one forever, so the loop must exit. It also returns a nil message
		// alongside the error, which must never reach the receiver. Falling through
		// here used to spin at ~460k iterations/sec feeding nil envelopes to the
		// receiver — one dead peer was enough to burn a core and crash the process.
		if err != nil {
			switch {
			case errors.Is(err, io.EOF):
				// peer half-closed the stream: the normal way a write stream shuts down.
				x.Logger().Info("listen stream closed by peer")
				return nil
			case status.Code(err) == codes.Canceled:
				// peer cancelled (its context died, e.g. graceful shutdown).
				x.Logger().Info("listen stream cancelled by peer")
				return nil
			default:
				// Unavailable (peer process died / TCP dropped), Unknown, etc.
				x.Logger().Warn("listen stream closed with error",
					"code", status.Code(err), "err", err)
				return err
			}
		}
		if msg == nil {
			// Defensive: a nil envelope with a nil error should be impossible, but the
			// receiver dereferences it, so never pass one on.
			x.Logger().Error("listen got a nil envelope without an error, ignoring")
			continue
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
