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
	//listenAddr is what Start() binds; addr is what peers are told to dial — deliberately
	//not the same string, see advertiseHost().
	listenAddr string
	addr       string
	gs         *grpc.Server
}

// NewRpcServer builds the node's inbound grpc server. listenAddr is the host:port to bind;
// "" means ":0" — every interface, kernel-assigned port.
func NewRpcServer(iRpcReceiver iRpcReceiver, listenAddr string) *RpcService {
	if listenAddr == "" {
		listenAddr = ":0"
	}
	return &RpcService{iRpcReceiver: iRpcReceiver, listenAddr: listenAddr}
}

func (x *RpcService) Listen(server Remoting_ListenServer) error {
	for {
		msg, err := server.Recv()
		// Every Recv error is terminal — grpc caches it and returns it forever, so the loop must
		// exit or it spins on a dead peer. Test err != nil first: status.Code(nil) is codes.OK.
		if err != nil {
			switch {
			case errors.Is(err, io.EOF):
				// peer half-closed: the normal way a write stream shuts down
				x.Logger().Info("listen stream closed by peer")
				return nil
			case status.Code(err) == codes.Canceled:
				// peer's context died, e.g. graceful shutdown
				x.Logger().Info("listen stream cancelled by peer")
				return nil
			default:
				// Unavailable (peer died / TCP dropped), Unknown, etc.
				x.Logger().Warn("listen stream closed with error",
					"code", status.Code(err), "err", err)
				return err
			}
		}
		if msg == nil {
			// should be impossible with a nil error, but the receiver dereferences it
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
	lis, err := net.Listen("tcp", x.listenAddr)
	if err != nil {
		return fmt.Errorf("grpc listen on %q err: %w", x.listenAddr, err)
	}
	_, port, err := net.SplitHostPort(lis.Addr().String())
	if err != nil {
		_ = lis.Close()
		return fmt.Errorf("parse listen addr err: %w", err)
	}
	x.addr = net.JoinHostPort(x.advertiseHost(), port)
	x.logger = slog.With("rpcService", x.addr)
	// Server and err MUST stay in locals: Stop() nils x.gs concurrently, so reading the field
	// in the goroutine can hand Serve a nil *grpc.Server — a panic nothing recovers.
	gs := grpc.NewServer()
	x.gs = gs
	RegisterRemotingServer(gs, x)
	go func() {
		// ErrServerStopped just means Stop() landed before Serve(); not a failure.
		if err := gs.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) && err != io.EOF {
			panic(err)
		}
	}()
	x.Logger().Info("RpcService Started ...", "listen", x.listenAddr)
	return nil
}

// advertiseHost picks the host peers are told to dial. A wildcard bind reports "[::]", which
// no peer can dial, so a reachable inner IP is substituted (loopback if there is none); an
// explicit host is advertised as given. Neither helps behind NAT — no advertise option yet.
func (x *RpcService) advertiseHost() string {
	switch host, _, err := net.SplitHostPort(x.listenAddr); {
	case err != nil:
		// unreachable: net.Listen already accepted this string
	case host != "" && host != "0.0.0.0" && host != "::":
		return host
	}
	if innerIP := gonet.GetTopInnerIP(); innerIP != nil {
		return innerIP.String()
	}
	return "127.0.0.1"
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
