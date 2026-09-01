package remote

import (
	"io"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// errStream mimics a stream whose peer is gone: grpc caches the terminal error and
// returns it, together with a nil message, on every subsequent Recv.
type errStream struct {
	grpc.ServerStream
	err   error
	recvN atomic.Int32
}

func (f *errStream) Send(*Envelope) error { return nil }
func (f *errStream) Recv() (*Envelope, error) {
	f.recvN.Add(1)
	return nil, f.err
}

// TestListenReturnsOnEveryRecvError pins the fix for a spin-and-crash bug.
//
// The error switch used to have a `case status.Code(err) > 0` arm that logged
// without returning. codes.Canceled is 1, so every code >= 2 — notably
// codes.Unavailable (14), which is what grpc reports when a peer process dies or its
// TCP connection drops — fell through to recvEnvelope(msg) with msg == nil. Measured
// before the fix: ~460k iterations/sec, ~460k nil envelopes delivered, and
// system.RecvEnvelope dereferenced the nil envelope, panicking on a grpc handler
// goroutine that nothing recovers — i.e. one dead peer crashed the whole process.
func TestListenReturnsOnEveryRecvError(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"EOF", io.EOF},
		{"Canceled", status.Error(codes.Canceled, "cancelled")},
		{"Unavailable", status.Error(codes.Unavailable, "transport is closing")},
		{"Unknown", status.Error(codes.Unknown, "boom")},
		{"Internal", status.Error(codes.Internal, "internal")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var delivered atomic.Int32
			var sawNil atomic.Bool
			svc := NewRpcServer(func(e *Envelope) {
				delivered.Add(1)
				if e == nil {
					sawNil.Store(true)
				}
			}, ":0")
			svc.logger = slog.Default()

			stream := &errStream{err: c.err}
			done := make(chan struct{})
			go func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("Listen panicked: %v", r)
					}
					close(done)
				}()
				_ = svc.Listen(stream)
			}()

			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatalf("Listen never returned: it is spinning on a dead stream "+
					"(%d Recv calls, %d envelopes delivered, nil delivered=%v)",
					stream.recvN.Load(), delivered.Load(), sawNil.Load())
			}

			// One Recv to observe the error, then out. No envelope may be delivered.
			if n := stream.recvN.Load(); n != 1 {
				t.Errorf("Recv called %d times, want exactly 1", n)
			}
			if n := delivered.Load(); n != 0 {
				t.Errorf("%d envelopes delivered from a failed Recv, want 0", n)
			}
			if sawNil.Load() {
				t.Error("a nil envelope was handed to the receiver")
			}
		})
	}
}

// okThenEOF delivers n good envelopes, then EOF.
type okThenEOF struct {
	grpc.ServerStream
	left int
}

func (f *okThenEOF) Send(*Envelope) error { return nil }
func (f *okThenEOF) Recv() (*Envelope, error) {
	if f.left == 0 {
		return nil, io.EOF
	}
	f.left--
	return &Envelope{MsgName: "x"}, nil
}

// TestListenDeliversUntilEOF guards against over-correcting: the happy path must
// still deliver every envelope and then exit cleanly.
func TestListenDeliversUntilEOF(t *testing.T) {
	var delivered atomic.Int32
	svc := NewRpcServer(func(e *Envelope) { delivered.Add(1) }, ":0")
	svc.logger = slog.Default()

	if err := svc.Listen(&okThenEOF{left: 5}); err != nil {
		t.Fatalf("clean EOF should not be an error, got %v", err)
	}
	if n := delivered.Load(); n != 5 {
		t.Errorf("delivered %d envelopes, want 5", n)
	}
}

// The listen address used to be a hardcoded ":0". These pin what the configurable
// version does — in particular that the ADVERTISED address (what peers dial) is derived
// from it and is never the wildcard the listener reports.
func TestStartAdvertisedAddr(t *testing.T) {
	t.Run("explicit host is advertised as given", func(t *testing.T) {
		svc := NewRpcServer(func(*Envelope) {}, "127.0.0.1:0")
		if err := svc.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		defer func() { _ = svc.Stop() }()

		host, port, err := net.SplitHostPort(svc.Addr())
		if err != nil {
			t.Fatalf("Addr() %q is not host:port: %v", svc.Addr(), err)
		}
		if host != "127.0.0.1" {
			t.Errorf("advertised host = %q, want the one that was bound (127.0.0.1)", host)
		}
		// port 0 must be resolved to the kernel-assigned one, or peers dial nothing
		if port == "0" || port == "" {
			t.Errorf("advertised port = %q, want the kernel-assigned port", port)
		}
	})

	t.Run("wildcard is replaced by a dialable host", func(t *testing.T) {
		svc := NewRpcServer(func(*Envelope) {}, ":0")
		if err := svc.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		defer func() { _ = svc.Stop() }()

		host, port, err := net.SplitHostPort(svc.Addr())
		if err != nil {
			t.Fatalf("Addr() %q is not host:port: %v", svc.Addr(), err)
		}
		// "[::]" / "0.0.0.0" / "" are what the listener reports and what no peer can
		// connect to.
		for _, bad := range []string{"", "::", "0.0.0.0"} {
			if host == bad {
				t.Errorf("advertised host = %q, which no peer can dial", host)
			}
		}
		if port == "0" || port == "" {
			t.Errorf("advertised port = %q, want the kernel-assigned port", port)
		}
	})

	t.Run("empty listenAddr defaults to :0", func(t *testing.T) {
		svc := NewRpcServer(func(*Envelope) {}, "")
		if svc.listenAddr != ":0" {
			t.Errorf("listenAddr = %q, want the \":0\" default", svc.listenAddr)
		}
	})
}

// A bad listen address must name itself in the error: it now comes from user config, so
// "address already in use" alone would not say which option to fix.
func TestStartBadListenAddrNamesIt(t *testing.T) {
	svc := NewRpcServer(func(*Envelope) {}, "256.256.256.256:1")
	err := svc.Start()
	if err == nil {
		_ = svc.Stop()
		t.Fatal("Start must fail on an unbindable address")
	}
	if !strings.Contains(err.Error(), "256.256.256.256:1") {
		t.Errorf("error should name the address that failed, got %q", err)
	}
}

// TestStartThenImmediateStop pins a crash that has nothing to do with the caller: Start
// spawned a goroutine that read the x.gs FIELD and called Serve on it, while Stop set
// that same field to nil. A Stop landing in the window before the goroutine got
// scheduled made it call Serve on a nil *grpc.Server — a panic raised inside grpc, on a
// goroutine nothing recovers, so the process died. (It was also a plain data race on
// x.gs and on Start's err.)
//
// The mirror case is a Stop landing just AFTER Serve begins: Serve then returns
// grpc.ErrServerStopped, which the goroutine treated as fatal and panicked on.
//
// A panic on either path takes the whole test binary down, so reaching the end of this
// loop IS the assertion.
func TestStartThenImmediateStop(t *testing.T) {
	for range 50 {
		svc := NewRpcServer(func(*Envelope) {}, "127.0.0.1:0")
		if err := svc.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if err := svc.Stop(); err != nil {
			t.Fatalf("Stop: %v", err)
		}
	}
	// let any Serve goroutine still in flight reach its error handling before the test
	// process moves on
	time.Sleep(200 * time.Millisecond)
}
