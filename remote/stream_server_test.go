package remote

import (
	"io"
	"log/slog"
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
			})
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
	svc := NewRpcServer(func(e *Envelope) { delivered.Add(1) })
	svc.logger = slog.Default()

	if err := svc.Listen(&okThenEOF{left: 5}); err != nil {
		t.Fatalf("clean EOF should not be an error, got %v", err)
	}
	if n := delivered.Load(); n != 5 {
		t.Errorf("delivered %d envelopes, want 5", n)
	}
}
