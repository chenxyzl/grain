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
	//listenAddr is what Start() binds; addr is what peers are told to dial. They are
	//deliberately not the same string — see Start().
	listenAddr string
	addr       string
	gs         *grpc.Server
}

// NewRpcServer builds the node's inbound grpc server. listenAddr is the host:port to
// bind; "" means ":0" — every interface, kernel-assigned port — which is what this used
// to hardcode.
func NewRpcServer(iRpcReceiver iRpcReceiver, listenAddr string) *RpcService {
	if listenAddr == "" {
		listenAddr = ":0"
	}
	return &RpcService{iRpcReceiver: iRpcReceiver, listenAddr: listenAddr}
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
	// gs is captured in a LOCAL, and so is the goroutine's err. Reading x.gs (and
	// assigning the outer err) from inside the goroutine was an unsynchronized access to
	// a field Stop() writes: a Start immediately followed by Stop had the goroutine read
	// x.gs *after* Stop set it to nil and call Serve on a nil *grpc.Server, which panics
	// inside grpc on a goroutine nothing recovers — the whole process died. Pinned by
	// TestStartThenImmediateStop.
	gs := grpc.NewServer()
	x.gs = gs
	RegisterRemotingServer(gs, x)
	go func() {
		// ErrServerStopped is the OTHER half of the same race: Stop() landing before
		// Serve() makes Serve return it immediately, and panicking on a deliberate
		// shutdown would be just as fatal.
		if err := gs.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) && err != io.EOF {
			panic(err)
		}
	}()
	x.Logger().Info("RpcService Started ...", "listen", x.listenAddr)
	return nil
}

// advertiseHost picks the host other cluster nodes are told to dial.
//
// A bound wildcard cannot be advertised: the listener reports "[::]", which no peer can
// connect to, so a reachable inner IP is substituted (falling back to loopback when the
// machine has no inner NIC — single-machine / container / CI, where the node still
// starts and is reachable locally, which is fine for that case).
//
// A host the caller named explicitly is advertised as given: having asked to bind one
// NIC, they do not then want peers pointed at a different one that happens to sort
// higher in GetTopInnerIP.
//
// Neither branch helps a node behind NAT or a container port mapping, where the
// reachable address is not one this process can observe at all. That needs a separate
// advertise-address option, which does not exist yet.
func (x *RpcService) advertiseHost() string {
	switch host, _, err := net.SplitHostPort(x.listenAddr); {
	case err != nil:
		// unreachable in practice: net.Listen already accepted this string.
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
